package githubpr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// fakeRef is a branch head (or commit snapshot) in fakeGitHub. The repo
// holds a single file, so a commit is fully described by that file's
// content and blob SHA.
type fakeRef struct {
	commit  string
	content string
	fileSHA string
}

// fakePull is a pull request in fakeGitHub.
type fakePull struct {
	head    string // branch name, without the owner prefix
	base    string
	state   string // "open" or "closed"
	htmlURL string
}

// fakeGitHub models just enough of the GitHub REST API for the PR
// pipeline: per-branch file state, 409 on a stale blob SHA, `?ref=` on
// reads, 422 on a duplicate ref, and 422 on a second open PR for the
// same head and base. It must reject what GitHub rejects, or the tests
// only mirror the worker's own assumptions.
type fakeGitHub struct {
	mu          sync.Mutex
	t           *testing.T
	branchSHA   string // main's initial commit
	fileSHA     string // main's initial blob
	fileContent string

	refs    map[string]fakeRef // branch name -> head
	commits map[string]fakeRef // commit SHA -> snapshot
	pulls   []fakePull
	seq     int

	getRefCalled         int
	getContentsCalled    int
	createRefCalled      int
	updateRefCalled      int
	createRefBody        map[string]string
	putContentsBody      map[string]string
	createPRBody         map[string]string
	failOnCreatePRStatus int
	listPullsCalled      int

	// getRefTransientFailures: while > 0, the get-ref endpoint answers
	// with getRefTransientStatus (default 503) and decrements.
	getRefTransientFailures int
	getRefTransientStatus   int

	// getRefBlockUntilCtx makes the get-ref endpoint hang until the
	// request's context is canceled, simulating a wedged GitHub call.
	getRefBlockUntilCtx bool
}

func newFakeGitHub(t *testing.T, fileContent string) *fakeGitHub {
	f := &fakeGitHub{
		t:           t,
		branchSHA:   "deadbeefcafe00000000000000000000deadbeef",
		fileSHA:     "filecontentsha000000000000000000feedface",
		fileContent: fileContent,
		refs:        map[string]fakeRef{},
		commits:     map[string]fakeRef{},
	}
	main := fakeRef{commit: f.branchSHA, content: fileContent, fileSHA: f.fileSHA}
	f.refs["main"] = main
	f.commits[main.commit] = main
	return f
}

// commitLocked records a new commit with content on branch. Caller holds f.mu.
func (f *fakeGitHub) commitLocked(branch, content string) fakeRef {
	f.seq++
	ref := fakeRef{
		commit:  fmt.Sprintf("commit-%d", f.seq),
		content: content,
		fileSHA: fmt.Sprintf("blob-%d", f.seq),
	}
	f.refs[branch] = ref
	f.commits[ref.commit] = ref
	return ref
}

// commit is commitLocked for test setup, e.g. moving main forward.
func (f *fakeGitHub) commit(branch, content string) fakeRef {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commitLocked(branch, content)
}

func (f *fakeGitHub) branch(name string) (fakeRef, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ref, ok := f.refs[name]
	return ref, ok
}

func (f *fakeGitHub) handler() http.Handler {
	const repo = "/repos/mjrossi/urbanist-atlas"
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+repo+"/git/ref/heads/{branch...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.getRefCalled++
		block := f.getRefBlockUntilCtx
		if f.getRefTransientFailures > 0 {
			f.getRefTransientFailures--
			status := f.getRefTransientStatus
			if status == 0 {
				status = http.StatusServiceUnavailable
			}
			f.mu.Unlock()
			http.Error(w, "transient", status)
			return
		}
		ref, ok := f.refs[r.PathValue("branch")]
		f.mu.Unlock()
		if block {
			<-r.Context().Done()
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not Found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"object": map[string]string{"sha": ref.commit, "type": "commit"},
		})
	})
	mux.HandleFunc("GET "+repo+"/contents/api/seed/orgs.toml", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.getContentsCalled++
		branch := r.URL.Query().Get("ref")
		if branch == "" {
			branch = "main"
		}
		ref, ok := f.refs[branch]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "No commit found for the ref " + branch})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"content":  base64.StdEncoding.EncodeToString([]byte(ref.content)),
			"encoding": "base64",
			"sha":      ref.fileSHA,
		})
	})
	mux.HandleFunc("PUT "+repo+"/contents/api/seed/orgs.toml", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		body := readJSONStringMap(f.t, r.Body)
		f.putContentsBody = body
		branch := body["branch"]
		if branch == "" {
			branch = "main"
		}
		ref, ok := f.refs[branch]
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]any{"message": "Branch " + branch + " not found"})
			return
		}
		if body["sha"] != ref.fileSHA {
			writeJSON(w, http.StatusConflict, map[string]any{
				"message": "api/seed/orgs.toml does not match " + body["sha"],
			})
			return
		}
		content, err := base64.StdEncoding.DecodeString(body["content"])
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "content is not valid Base64"})
			return
		}
		next := f.commitLocked(branch, string(content))
		writeJSON(w, http.StatusOK, map[string]any{"commit": map[string]string{"sha": next.commit}})
	})
	mux.HandleFunc("POST "+repo+"/git/refs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.createRefCalled++
		f.createRefBody = readJSONStringMap(f.t, r.Body)
		name := strings.TrimPrefix(f.createRefBody["ref"], "refs/heads/")
		if _, exists := f.refs[name]; exists {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "Reference already exists"})
			return
		}
		snap, ok := f.commits[f.createRefBody["sha"]]
		if !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "Object does not exist"})
			return
		}
		f.refs[name] = snap
		writeJSON(w, http.StatusCreated, map[string]any{"ref": f.createRefBody["ref"]})
	})
	mux.HandleFunc("PATCH "+repo+"/git/refs/heads/{branch...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.updateRefCalled++
		var body struct {
			SHA   string `json:"sha"`
			Force bool   `json:"force"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("decode update-ref body: %v", err)
		}
		name := r.PathValue("branch")
		if _, exists := f.refs[name]; !exists {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "Reference does not exist"})
			return
		}
		snap, ok := f.commits[body.SHA]
		if !ok {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "Object does not exist"})
			return
		}
		// The fake keeps no commit graph, so any non-forced move is
		// treated as a non-fast-forward.
		if !body.Force {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "Update is not a fast forward"})
			return
		}
		f.refs[name] = snap
		writeJSON(w, http.StatusOK, map[string]any{"ref": "refs/heads/" + name})
	})
	mux.HandleFunc("POST "+repo+"/pulls", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.createPRBody = readJSONStringMap(f.t, r.Body)
		status := f.failOnCreatePRStatus
		if status == 0 {
			status = http.StatusCreated
		}
		if status != http.StatusCreated {
			http.Error(w, "boom", status)
			return
		}
		head, base := f.createPRBody["head"], f.createPRBody["base"]
		for _, p := range f.pulls {
			if p.head == head && p.base == base && p.state == "open" {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"message": "Validation Failed",
					"errors": []map[string]string{{
						"resource": "PullRequest",
						"code":     "custom",
						"message":  "A pull request already exists for mjrossi:" + head + ".",
					}},
					"documentation_url": "https://docs.github.com/rest/pulls/pulls#create-a-pull-request",
				})
				return
			}
		}
		pr := fakePull{
			head:    head,
			base:    base,
			state:   "open",
			htmlURL: fmt.Sprintf("https://github.com/mjrossi/urbanist-atlas/pull/%d", 42+len(f.pulls)),
		}
		f.pulls = append(f.pulls, pr)
		writeJSON(w, status, map[string]any{"html_url": pr.htmlURL})
	})
	// List pulls filters on `head` (as owner:branch), `base` and `state`
	// (default "open").
	mux.HandleFunc("GET "+repo+"/pulls", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.listPullsCalled++
		q := r.URL.Query()
		state := q.Get("state")
		if state == "" {
			state = "open"
		}
		out := []map[string]any{}
		for _, p := range f.pulls {
			if h := q.Get("head"); h != "" && h != "mjrossi:"+p.head {
				continue
			}
			if b := q.Get("base"); b != "" && b != p.base {
				continue
			}
			if state != "all" && state != p.state {
				continue
			}
			out = append(out, map[string]any{
				"html_url": p.htmlURL,
				"state":    p.state,
				"head":     map[string]string{"ref": p.head, "label": "mjrossi:" + p.head},
				"base":     map[string]string{"ref": p.base},
			})
		}
		writeJSON(w, http.StatusOK, out)
	})
	return mux
}

// closePulls marks every PR as closed, as if a maintainer closed it.
func (f *fakeGitHub) closePulls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.pulls {
		f.pulls[i].state = "closed"
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func readJSONStringMap(t *testing.T, r io.Reader) map[string]string {
	t.Helper()
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode body %q: %v", raw, err)
	}
	return out
}

func sampleSubmission() atlas.Submission {
	return atlas.Submission{
		PublicID: "01928200-3344-7000-9abc-000000000001",
		Payload: atlas.SubmissionPayload{
			Name:        "Brooklyn Greenways",
			ShortDesc:   "Volunteers expanding the borough's protected-lane network.",
			WebsiteURL:  "https://example.org/brooklyn-greenways",
			Tags:        []string{"cycling", "grassroots"},
			RegionSlugs: []string{"brooklyn-ny"},
		},
		SubmitterName:  "Jane",
		SubmitterEmail: "jane@example.org",
		SubmitterNote:  "Coordinating with DOT on the Ashland project.",
		Status:         atlas.SubmissionApproved,
		CreatedAt:      time.Date(2026, 5, 28, 14, 30, 0, 0, time.UTC),
	}
}

// fakePersist records calls to PersistResult.
type fakePersist struct {
	mu    sync.Mutex
	calls []persistCall
}

type persistCall struct {
	PublicID string
	URL      string
	Err      string
}

func (p *fakePersist) record(_ context.Context, publicID, prURL, prErr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, persistCall{publicID, prURL, prErr})
	return nil
}

func (p *fakePersist) wait(t *testing.T, want int) []persistCall {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		p.mu.Lock()
		n := len(p.calls)
		p.mu.Unlock()
		if n >= want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d persist calls; got %d", want, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]persistCall, len(p.calls))
	copy(out, p.calls)
	return out
}

func TestWorker_HappyPath_OpensPR(t *testing.T) {
	gh := newFakeGitHub(t, "# existing orgs.toml\n\n[[org]]\nslug = \"transalt-brooklyn\"\nname = \"Transportation Alternatives\"\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	calls := persist.wait(t, 1)
	if calls[0].URL != "https://github.com/mjrossi/urbanist-atlas/pull/42" {
		t.Fatalf("PersistResult URL = %q", calls[0].URL)
	}
	if calls[0].Err != "" {
		t.Fatalf("PersistResult Err = %q, want empty on happy path", calls[0].Err)
	}

	// Branch name uses the keyset suffix (first 16 hex chars of the
	// dash-stripped UUIDv7) — long enough that two submissions in the
	// same millisecond still get distinct branches.
	if got := gh.createRefBody["ref"]; got != "refs/heads/submission/0192820033447000" {
		t.Fatalf("create-ref body ref = %q", got)
	}
	if gh.createRefBody["sha"] != gh.branchSHA {
		t.Fatalf("create-ref body sha = %q, want %q", gh.createRefBody["sha"], gh.branchSHA)
	}

	// PUT contents must reuse the existing file SHA and target the
	// new branch.
	if gh.putContentsBody["branch"] != "submission/0192820033447000" {
		t.Fatalf("put contents branch = %q", gh.putContentsBody["branch"])
	}
	if gh.putContentsBody["sha"] != gh.fileSHA {
		t.Fatalf("put contents sha = %q, want existing file sha", gh.putContentsBody["sha"])
	}
	decoded, err := base64.StdEncoding.DecodeString(gh.putContentsBody["content"])
	if err != nil {
		t.Fatalf("put contents body not base64: %v", err)
	}
	if !strings.Contains(string(decoded), "[[org]]") || !strings.Contains(string(decoded), "Brooklyn Greenways") {
		t.Fatalf("appended content missing expected entry: %s", decoded)
	}
	if !strings.HasPrefix(string(decoded), "# existing orgs.toml") {
		t.Fatalf("existing content not preserved at top of file: %s", decoded[:min(200, len(decoded))])
	}

	if !strings.Contains(gh.createPRBody["body"], "01928200-3344-7000-9abc-000000000001") {
		t.Fatalf("PR body missing public id: %q", gh.createPRBody["body"])
	}
	if gh.createPRBody["title"] != "Add `Brooklyn Greenways`" {
		t.Fatalf("PR title = %q", gh.createPRBody["title"])
	}
}

// TestBuildPRBody_OmitsSubmitterPII: the PR lands on a public repo, and
// the submit form tells submitters their email is for editor follow-up
// only. Name, email and note must never reach the body.
func TestBuildPRBody_OmitsSubmitterPII(t *testing.T) {
	sub := sampleSubmission()
	sub.SubmitterName = "Jane Q. Submitter"
	sub.SubmitterEmail = "jane.private@example.org"
	sub.SubmitterNote = "Call me on 555-0100 about the Ashland project."
	body := buildPRBody(sub)

	for _, secret := range []string{"Jane", "jane.private@example.org", "555-0100", "Ashland"} {
		if strings.Contains(body, secret) {
			t.Errorf("PR body leaks submitter data %q:\n%s", secret, body)
		}
	}
	for _, want := range []string{
		"`01928200-3344-7000-9abc-000000000001`",
		"2026-05-28T14:30:00Z",
		"`brooklyn-ny`",
		"/api/v1/admin/submissions",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("PR body missing %q:\n%s", want, body)
		}
	}
}

// outsideCodeSpans returns s with every `...` code span removed. GitHub
// renders neither @mentions nor Markdown inside a code span, so
// whatever remains is what could ping someone.
func outsideCodeSpans(s string) string {
	var b strings.Builder
	inCode := false
	for _, r := range s {
		if r == '`' {
			inCode = !inCode
			continue
		}
		if !inCode {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// TestPRText_NeutralisesMentionsAndMarkdown: the org name is submitter
// text, so it must not be able to @mention a user or inject Markdown
// into the PR title, body or commit message.
func TestPRText_NeutralisesMentionsAndMarkdown(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	w := New(Config{BaseURL: server.URL, Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	sub := sampleSubmission()
	sub.Payload.Name = "Riders `@someone` **now**\n# heading @other"
	if _, err := w.ProcessNow(t.Context(), sub); err != nil {
		t.Fatalf("ProcessNow: %v", err)
	}

	gh.mu.Lock()
	defer gh.mu.Unlock()
	texts := map[string]string{
		"title":          gh.createPRBody["title"],
		"body":           gh.createPRBody["body"],
		"commit message": gh.putContentsBody["message"],
	}
	for field, text := range texts {
		if strings.Count(text, "`")%2 != 0 {
			t.Errorf("%s has an unbalanced backtick, so a code span can be escaped: %q", field, text)
		}
		outside := outsideCodeSpans(text)
		for _, bad := range []string{"@someone", "@other", "**", "\n#"} {
			if strings.Contains(outside, bad) {
				t.Errorf("%s exposes %q outside a code span: %q", field, bad, text)
			}
		}
	}
	if want := "Add `Riders @someone **now** # heading @other`"; texts["title"] != want {
		t.Errorf("title = %q, want %q", texts["title"], want)
	}
}

func TestShortID_UsesRandomTail(t *testing.T) {
	// Same millisecond timestamp prefix, different random tails.
	a := "01928200-3344-7000-9abc-000000000001"
	b := "01928200-3344-7000-9abc-000000000002"
	if shortID(a) == shortID(b) {
		t.Fatalf("shortID collides for same-timestamp IDs: %q", shortID(a))
	}
	if got := shortID(a); got != "00000001" {
		t.Fatalf("shortID(%q) = %q, want last 8 hex digits", a, got)
	}
}

func TestWorker_FailingPR_PersistsErrorAndDoesNotRetry(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.failOnCreatePRStatus = http.StatusUnprocessableEntity
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	calls := persist.wait(t, 1)
	if calls[0].Err == "" {
		t.Fatal("expected PersistResult Err to be set")
	}
	if !strings.Contains(calls[0].Err, "422") {
		t.Fatalf("error string should mention status: %q", calls[0].Err)
	}
	if calls[0].URL != "" {
		t.Fatalf("expected empty URL on failure, got %q", calls[0].URL)
	}
}

func TestEnqueue_DisabledWithoutToken(t *testing.T) {
	w := New(Config{Logger: slog.New(slog.DiscardHandler)})
	err := w.Enqueue(context.Background(), sampleSubmission())
	if !errors.Is(err, atlas.ErrPromotionDisabled) {
		t.Fatalf("err = %v, want atlas.ErrPromotionDisabled", err)
	}
}

func TestEnqueue_FullBuffer(t *testing.T) {
	w := New(Config{Token: "x", BufferSize: 1, Logger: slog.New(slog.DiscardHandler)})
	// Don't call Run, so jobs accumulate.
	if err := w.Enqueue(context.Background(), sampleSubmission()); err != nil {
		t.Fatalf("first Enqueue: %v", err)
	}
	if err := w.Enqueue(context.Background(), sampleSubmission()); !errors.Is(err, ErrBufferFull) {
		t.Fatalf("second Enqueue err = %v, want ErrBufferFull", err)
	}
}

// TestWorker_Stop_IsIdempotent pins that a second Stop call is a
// no-op rather than a panic (close-of-closed-channel). serve.go
// currently calls Stop exactly once, but a future shutdown path
// that fires twice (e.g., double SIGTERM) shouldn't crash the
// process before the deferred cleanup runs.
func TestWorker_Stop_IsIdempotent(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	w := New(Config{
		BaseURL: server.URL,
		Token:   "fake-token",
		Logger:  slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if _, err := w.Stop(stopCtx); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second call must not panic. Returns nil because Run has
	// already exited and w.done is closed.
	dropped, err := w.Stop(stopCtx)
	if err != nil {
		t.Fatalf("second Stop returned err: %v", err)
	}
	if len(dropped) != 0 {
		t.Fatalf("second Stop reported dropped IDs: %v", dropped)
	}
}

// TestWorker_Stop_DrainsBuffer pins the SIGTERM-shutdown contract:
// Stop closes the jobs channel, Run drains whatever's already
// buffered (processing each one), and Stop returns nil once Run
// exits.
func TestWorker_Stop_DrainsBuffer(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	persist.wait(t, 1)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	dropped, err := w.Stop(stopCtx)
	if err != nil {
		t.Fatalf("Stop returned err: %v (dropped=%v)", err, dropped)
	}
	if len(dropped) != 0 {
		t.Fatalf("Stop dropped IDs on clean drain: %v", dropped)
	}
}

func TestRenderOrgBlock_Deterministic(t *testing.T) {
	sub := sampleSubmission()
	addedAt := time.Date(2026, 5, 28, 14, 30, 0, 0, time.UTC)
	a, err := RenderOrgBlock(sub, "brooklyn-greenways", addedAt)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	b, err := RenderOrgBlock(sub, "brooklyn-greenways", addedAt)
	if err != nil {
		t.Fatalf("render again: %v", err)
	}
	if a != b {
		t.Fatalf("non-deterministic output:\n%s\n---\n%s", a, b)
	}
	// go-toml/v2 picks single-quoted literal strings when no escape
	// is needed, double-quoted basic strings otherwise. Both are
	// valid TOML and the seed loader accepts both — assert on
	// structural content, not quote style.
	if !strings.Contains(a, "slug = ") || !strings.Contains(a, "brooklyn-greenways") {
		t.Fatalf("rendered output missing slug=brooklyn-greenways: %s", a)
	}
	if !strings.Contains(a, "name = ") || !strings.Contains(a, "Brooklyn Greenways") {
		t.Fatalf("rendered output missing name=Brooklyn Greenways: %s", a)
	}
	if !strings.HasPrefix(strings.TrimSpace(a), "[[org]]") {
		t.Fatalf("rendered output does not start with [[org]] header: %s", a)
	}
}

// TestRenderOrgBlock_AddedAt asserts the rendered block carries a
// date-only added_at matching the approval date passed in. Without it
// the merged PR's bundle would fail the loader's required-field check
// at boot.
func TestRenderOrgBlock_AddedAt(t *testing.T) {
	sub := sampleSubmission()
	// CreatedAt is May 28 (submission time); approval is later.
	// Pick a date distinct from CreatedAt and assert THAT one: the field
	// comes from the caller's clock, not the submission row.
	addedAt := time.Date(2026, 6, 1, 9, 15, 0, 0, time.UTC)
	out, err := RenderOrgBlock(sub, "brooklyn-greenways", addedAt)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Unquoted ISO date, the seed convention; it parses as the
	// toml.LocalDate the loader's required-field check expects.
	const want = "added_at = 2026-06-01"
	if !strings.Contains(out, want) {
		t.Fatalf("rendered block missing %q line; got:\n%s", want, out)
	}
	// And NOT the submission date — guard against a future refactor
	// that accidentally reroutes the field through sub.CreatedAt.
	if strings.Contains(out, "added_at = 2026-05-28") {
		t.Fatalf("rendered block used sub.CreatedAt (2026-05-28) instead of approval date:\n%s", out)
	}
}

// TestRenderOrgBlock_EmptyTagsRendersPlaceholder pins the editor
// affordance: public submissions carry no tags, so sub.Payload.Tags is
// nil, yet the block must still emit an explicit `tags = []` for the
// editor to fill in during PR review. region_slugs renders as-is.
func TestRenderOrgBlock_EmptyTagsRendersPlaceholder(t *testing.T) {
	sub := atlas.Submission{
		PublicID: "01928200-3344-7000-9abc-000000000099",
		Payload: atlas.SubmissionPayload{
			Name:        "Queens Bus Riders",
			ShortDesc:   "Riders organizing for faster, more frequent bus service.",
			WebsiteURL:  "https://example.org/queens-bus-riders",
			Tags:        nil, // public submissions don't carry tags
			RegionSlugs: []string{"queens"},
		},
		Status:    atlas.SubmissionApproved,
		CreatedAt: time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC),
	}
	addedAt := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	out, err := RenderOrgBlock(sub, "queens-bus-riders", addedAt)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, "tags = []") {
		t.Fatalf("rendered block missing empty `tags = []` placeholder; got:\n%s", out)
	}
	if !strings.Contains(out, "queens") {
		t.Fatalf("rendered block missing region_slugs entry `queens`; got:\n%s", out)
	}
}

func TestDeriveSlug(t *testing.T) {
	cases := map[string]string{
		"Brooklyn Greenways":   "brooklyn-greenways",
		"  Transit  Alliance ": "transit-alliance",
		"NYC DOT":              "nyc-dot",
		"!!!":                  "",
		"Café Reform":          "caf-reform", // current behavior: non-ASCII dropped, double hyphen collapsed
	}
	for in, want := range cases {
		if got := DeriveSlug(in); got != want {
			t.Errorf("DeriveSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApiError_IncludesStatusAndBody(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("bad gateway"))}
	err := apiError("test op", resp)
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "bad gateway") {
		t.Fatalf("apiError lost detail: %v", err)
	}
	// satisfies the "ensure short body" guard so a future change to
	// the LimitReader doesn't accidentally drop characters under
	// 4 KiB.
	if !strings.HasPrefix(err.Error(), "test op: github returned 502") {
		t.Fatalf("apiError prefix wrong: %v", err)
	}
}

// errReader returns some bytes then a non-EOF error, exercising the
// truncated-read marker path in apiError.
type errReader struct {
	data []byte
	pos  int
}

func (e *errReader) Read(p []byte) (int, error) {
	if e.pos < len(e.data) {
		n := copy(p, e.data[e.pos:])
		e.pos += n
		return n, nil
	}
	return 0, errors.New("simulated mid-body read failure")
}

func TestApiError_MarksTruncatedRead(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       io.NopCloser(&errReader{data: []byte("partial body")}),
	}
	err := apiError("flaky op", resp)
	// Status must survive — it's the load-bearing signal.
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("apiError dropped status on read error: %v", err)
	}
	// Whatever bytes arrived before the error must still be present.
	if !strings.Contains(err.Error(), "partial body") {
		t.Fatalf("apiError dropped partial body: %v", err)
	}
	// And the read failure must be visible, not silently swallowed.
	if !strings.Contains(err.Error(), "body read truncated") {
		t.Fatalf("apiError did not mark the truncated read: %v", err)
	}
}

// TestWorker_RetriesTransientGET pins the idempotent-GET
// retry: the first two get-ref calls return 503, the third succeeds,
// and the pipeline opens the PR without the moderator seeing an error.
// Proves doIdempotentRequest retries a retryable status on a GET.
func TestWorker_RetriesTransientGET(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.getRefTransientFailures = 2 // two 503s, then succeed
	gh.getRefTransientStatus = http.StatusServiceUnavailable
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	calls := persist.wait(t, 1)
	if calls[0].Err != "" {
		t.Fatalf("expected success after retries, got err %q", calls[0].Err)
	}
	if calls[0].URL == "" {
		t.Fatal("expected PR URL after retries")
	}
	gh.mu.Lock()
	got := gh.getRefCalled
	gh.mu.Unlock()
	if got != 3 {
		t.Fatalf("get-ref called %d times, want 3 (2 retried 503 + 1 success)", got)
	}
}

// TestWorker_NonRetryableGET pins that a non-retryable GET status (404)
// fails fast — no retry storm against caller-state errors.
func TestWorker_NonRetryableGET(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.getRefTransientFailures = 5 // would exceed retryMaxAttempts if retried
	gh.getRefTransientStatus = http.StatusNotFound
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	calls := persist.wait(t, 1)
	if calls[0].Err == "" {
		t.Fatal("expected error on non-retryable 404")
	}
	gh.mu.Lock()
	got := gh.getRefCalled
	gh.mu.Unlock()
	if got != 1 {
		t.Fatalf("get-ref called %d times, want 1 (404 not retried)", got)
	}
}

// TestWorker_RetriesExhaustGET pins the retry-exhaustion boundary: a
// retryable status (503) that never recovers stops after exactly
// retryMaxAttempts calls and surfaces a terminal error (recorded via
// PersistResult), rather than retrying unboundedly. TestWorker_RetriesTransientGET
// covers recover-before-exhaustion; this covers the give-up edge.
func TestWorker_RetriesExhaustGET(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.getRefTransientFailures = 99 // never recovers
	gh.getRefTransientStatus = http.StatusServiceUnavailable
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	calls := persist.wait(t, 1)
	if calls[0].Err == "" {
		t.Fatal("expected a terminal error after retries exhaust, got success")
	}
	gh.mu.Lock()
	got := gh.getRefCalled
	gh.mu.Unlock()
	if got != retryMaxAttempts {
		t.Fatalf("get-ref called %d times, want %d (1 initial + %d retries, then give up)",
			got, retryMaxAttempts, retryMaxAttempts-1)
	}
}

// TestWorker_CreateBranch_AlreadyExistsIsIdempotent: when the
// deterministic submission branch already exists with a different
// orgs.toml blob than main, the pipeline must still put the file and
// open the PR rather than 409 on a stale blob SHA.
func TestWorker_CreateBranch_AlreadyExistsIsIdempotent(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.commit("submission/0192820033447000", "# orgs.toml\n# leftover from a prior attempt\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go w.Run(ctx)

	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	calls := persist.wait(t, 1)
	if calls[0].Err != "" {
		t.Fatalf("422 already-exists should be tolerated, got err %q", calls[0].Err)
	}
	if calls[0].URL == "" {
		t.Fatal("expected PR URL despite pre-existing branch")
	}
	// The pipeline must still have PUT the file and opened the PR.
	gh.mu.Lock()
	defer gh.mu.Unlock()
	if gh.putContentsBody == nil {
		t.Fatal("put-contents not called after tolerated 422")
	}
	if gh.createPRBody == nil {
		t.Fatal("create-PR not called after tolerated 422")
	}
}

// TestWorker_ShutdownDrain_BoundsWedgedCall pins that a job
// drained on the hard-cancel path (drainAndExit) runs under
// shutdownDrainTimeout, so a wedged GitHub call can't pin process exit
// for the full openPRTimeout (30s). The fake's get-ref hangs forever;
// without the bounded drain ctx, Run would block ~30s. We shrink
// shutdownDrainTimeout to keep the test fast and assert Run returns
// well under openPRTimeout.
func TestWorker_ShutdownDrain_BoundsWedgedCall(t *testing.T) {
	prev := shutdownDrainTimeout
	shutdownDrainTimeout = 200 * time.Millisecond
	t.Cleanup(func() { shutdownDrainTimeout = prev })

	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.getRefBlockUntilCtx = true // every get-ref hangs until ctx fires
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	persist := &fakePersist{}
	w := New(Config{
		BaseURL:       server.URL,
		Token:         "fake-token",
		PersistResult: persist.record,
		Logger:        slog.New(slog.DiscardHandler),
	})
	ctx, cancel := context.WithCancel(context.Background())

	// Job A will be picked up by Run and wedge in getBranchSHA. Job B
	// sits in the buffer so it's still there when ctx cancels, forcing
	// it through drainAndExit.
	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue A: %v", err)
	}
	if err := w.Enqueue(ctx, sampleSubmission()); err != nil {
		t.Fatalf("Enqueue B: %v", err)
	}

	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	// Wait until Run is actually wedged inside job A's get-ref before
	// canceling, so cancellation lands while a call is in flight and
	// job B is still buffered.
	deadline := time.Now().Add(2 * time.Second)
	for {
		gh.mu.Lock()
		started := gh.getRefCalled > 0
		gh.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("get-ref never reached; worker did not start processing")
		}
		time.Sleep(5 * time.Millisecond)
	}

	cancel()

	// Both jobs (A in-flight under the now-canceled parent ctx, B under
	// the bounded drain ctx) must resolve and Run must return. Budget:
	// comfortably above 2*shutdownDrainTimeout but far below
	// openPRTimeout — that gap is the regression guard.
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("Run did not return after cancel; wedged call pinned shutdown "+
			"(shutdownDrainTimeout=%v, openPRTimeout=%v)", shutdownDrainTimeout, openPRTimeout)
	}

	// Both jobs should have recorded a (failed) promotion result — the
	// detached persist write lands even though the GitHub call was cut
	// off. This proves the drain still records outcomes under the bound.
	calls := persist.wait(t, 2)
	for _, c := range calls {
		if c.Err == "" {
			t.Errorf("expected promotion_error on wedged-then-canceled job, got success url=%q", c.URL)
		}
	}
}

func TestRetryableStatus(t *testing.T) {
	retryable := []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable}
	for _, s := range retryable {
		if !retryableStatus(s) {
			t.Errorf("retryableStatus(%d) = false, want true", s)
		}
	}
	notRetryable := []int{
		http.StatusOK, http.StatusCreated, http.StatusNotFound,
		http.StatusUnprocessableEntity, http.StatusConflict, http.StatusForbidden,
		http.StatusInternalServerError, // 500 is ambiguous; we don't blind-retry it
	}
	for _, s := range notRetryable {
		if retryableStatus(s) {
			t.Errorf("retryableStatus(%d) = true, want false", s)
		}
	}
}

func TestIsSecondaryRateLimit(t *testing.T) {
	mk := func(status int, header http.Header, body string) (*http.Response, []byte) {
		if header == nil {
			header = http.Header{}
		}
		return &http.Response{StatusCode: status, Header: header}, []byte(body)
	}

	// Non-403 is never a secondary rate limit.
	if resp, body := mk(http.StatusServiceUnavailable, nil, "rate limit"); isSecondaryRateLimit(resp, body) {
		t.Error("503 should not classify as secondary rate limit")
	}
	// Plain authorization 403 (no signal) is terminal.
	if resp, body := mk(http.StatusForbidden, nil, "Bad credentials"); isSecondaryRateLimit(resp, body) {
		t.Error("403 without rate-limit signal should be terminal")
	}
	// Retry-After header -> retryable.
	if resp, body := mk(http.StatusForbidden, http.Header{"Retry-After": {"30"}}, ""); !isSecondaryRateLimit(resp, body) {
		t.Error("403 with Retry-After should be retryable")
	}
	// X-RateLimit-Remaining: 0 -> retryable.
	if resp, body := mk(http.StatusForbidden, http.Header{"X-Ratelimit-Remaining": {"0"}}, ""); !isSecondaryRateLimit(resp, body) {
		t.Error("403 with X-RateLimit-Remaining:0 should be retryable")
	}
	// Body mentions a rate limit (case-insensitive) -> retryable.
	if resp, body := mk(http.StatusForbidden, nil, "You have exceeded a secondary RATE LIMIT"); !isSecondaryRateLimit(resp, body) {
		t.Error("403 with rate-limit body should be retryable")
	}
}

// TestBackoffSleep_HonorsCancellation pins that a wedged backoff aborts
// promptly when ctx is canceled (the graceful-shutdown guarantee) rather
// than sleeping out the full duration.
func TestBackoffSleep_HonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	start := time.Now()
	err := backoffSleep(ctx, 10*time.Second)
	if err == nil {
		t.Fatal("backoffSleep returned nil on canceled ctx, want ctx.Err()")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("backoffSleep slept %v despite cancellation; want near-immediate return", elapsed)
	}
}

// TestBackoffSleep_CompletesNormally pins the non-canceled path returns
// nil after the timer fires.
func TestBackoffSleep_CompletesNormally(t *testing.T) {
	if err := backoffSleep(context.Background(), 5*time.Millisecond); err != nil {
		t.Fatalf("backoffSleep returned %v on clean completion, want nil", err)
	}
}

// TestWorker_RetryAfterPartialRun is the `submissions retry-pr` path: a
// first attempt committed to the submission branch and then failed to
// open the PR, and main moved on before the retry. The retry must
// succeed, and the branch must hold main's current file plus the new
// block exactly once.
func TestWorker_RetryAfterPartialRun(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.failOnCreatePRStatus = http.StatusBadGateway
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	w := New(Config{BaseURL: server.URL, Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	sub := sampleSubmission()
	if _, err := w.ProcessNow(t.Context(), sub); err == nil {
		t.Fatal("first attempt: want create-PR failure, got nil")
	}
	const branch = "submission/0192820033447000"
	if ref, ok := gh.branch(branch); !ok || !strings.Contains(ref.content, "Brooklyn Greenways") {
		t.Fatalf("first attempt should have committed the block to %s; ref=%+v ok=%v", branch, ref, ok)
	}

	const movedMain = "# orgs.toml\n\n[[org]]\nslug = \"merged-meanwhile\"\n"
	gh.commit("main", movedMain)
	gh.mu.Lock()
	gh.failOnCreatePRStatus = 0
	gh.mu.Unlock()

	prURL, err := w.ProcessNow(t.Context(), sub)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if prURL == "" {
		t.Fatal("retry returned an empty PR URL")
	}
	ref, _ := gh.branch(branch)
	if !strings.HasPrefix(ref.content, movedMain) {
		t.Errorf("branch file does not start with main's current content:\n%s", ref.content)
	}
	if n := strings.Count(ref.content, "Brooklyn Greenways"); n != 1 {
		t.Errorf("block appears %d times on the branch, want 1:\n%s", n, ref.content)
	}
}

// TestWorker_RetryAfterPROpened: the first attempt opened the PR but the
// job died before PersistResult recorded the URL. The operator's retry
// must resolve create-PR's duplicate 422 to the PR that already exists.
func TestWorker_RetryAfterPROpened(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	w := New(Config{BaseURL: server.URL, Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	sub := sampleSubmission()
	first, err := w.ProcessNow(t.Context(), sub)
	if err != nil {
		t.Fatalf("first attempt: %v", err)
	}

	second, err := w.ProcessNow(t.Context(), sub)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if second != first {
		t.Errorf("retry URL = %q, want the existing PR %q", second, first)
	}
	gh.mu.Lock()
	defer gh.mu.Unlock()
	if n := len(gh.pulls); n != 1 {
		t.Errorf("fake holds %d PRs, want 1", n)
	}
	if ref := gh.refs["submission/0192820033447000"]; strings.Count(ref.content, "Brooklyn Greenways") != 1 {
		t.Errorf("branch should hold the block exactly once:\n%s", ref.content)
	}
}

// TestWorker_RetryAfterPRClosed: a closed PR does not block a new one,
// so the retry opens a fresh PR rather than reporting the closed one.
func TestWorker_RetryAfterPRClosed(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	w := New(Config{BaseURL: server.URL, Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	sub := sampleSubmission()
	first, err := w.ProcessNow(t.Context(), sub)
	if err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	gh.closePulls()

	second, err := w.ProcessNow(t.Context(), sub)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if second == first {
		t.Errorf("retry returned the closed PR %q, want a new one", first)
	}
}

// TestWorker_CreatePR422WithoutOpenPR_KeepsError: a 422 the list lookup
// can't explain (no open PR on the head) stays a promotion error.
func TestWorker_CreatePR422WithoutOpenPR_KeepsError(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.failOnCreatePRStatus = http.StatusUnprocessableEntity
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)

	w := New(Config{BaseURL: server.URL, Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	_, err := w.ProcessNow(t.Context(), sampleSubmission())
	if err == nil || !strings.Contains(err.Error(), "create PR: github returned 422: boom") {
		t.Fatalf("err = %v, want the original create-PR 422", err)
	}
	gh.mu.Lock()
	defer gh.mu.Unlock()
	if gh.listPullsCalled != 1 {
		t.Errorf("list-PRs called %d times, want 1", gh.listPullsCalled)
	}
}

// TestWorker_EnqueueAfterStop_ReturnsErrStopped: once Stop has closed
// the channel, a late approval (an HTTP handler still running after a
// timed-out server Shutdown) must get an error it can persist as
// promotion_error, not a send-on-closed-channel panic.
func TestWorker_EnqueueAfterStop_ReturnsErrStopped(t *testing.T) {
	w := New(Config{Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	go w.Run(context.Background())
	if _, err := w.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := w.Enqueue(context.Background(), sampleSubmission()); !errors.Is(err, ErrStopped) {
		t.Fatalf("Enqueue after Stop = %v, want ErrStopped", err)
	}
}

// TestWorker_StopTimeout_ReportsInflightJob: when Stop's deadline fires
// while a job is mid-flight, that job's ID must be in droppedIDs, since
// the process is about to exit and its outcome will never be persisted.
func TestWorker_StopTimeout_ReportsInflightJob(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.getRefBlockUntilCtx = true
	server := httptest.NewServer(gh.handler())
	t.Cleanup(server.Close)
	// Runs before server.Close (cleanups are LIFO) so the wedged
	// request is severed and Close doesn't wait on it.
	t.Cleanup(server.CloseClientConnections)

	w := New(Config{BaseURL: server.URL, Token: "fake-token", Logger: slog.New(slog.DiscardHandler)})
	go w.Run(context.Background())
	sub := sampleSubmission()
	if err := w.Enqueue(t.Context(), sub); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		gh.mu.Lock()
		started := gh.getRefCalled > 0
		gh.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker never reached get-ref")
		}
		time.Sleep(5 * time.Millisecond)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	dropped, err := w.Stop(stopCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop err = %v, want DeadlineExceeded", err)
	}
	if len(dropped) != 1 || dropped[0] != sub.PublicID {
		t.Fatalf("dropped = %v, want [%s]", dropped, sub.PublicID)
	}
}
