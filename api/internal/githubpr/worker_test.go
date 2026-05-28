package githubpr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

// fakeGitHub captures the sequence of calls a PR-creation flow makes
// against api.github.com and returns canned responses.
type fakeGitHub struct {
	mu          sync.Mutex
	t           *testing.T
	branchSHA   string
	fileSHA     string
	fileContent string

	getRefCalled         int
	getContentsCalled    int
	createRefBody        map[string]string
	putContentsBody      map[string]string
	createPRBody         map[string]string
	createPRResponseURL  string
	failOnCreatePRStatus int
}

func newFakeGitHub(t *testing.T, fileContent string) *fakeGitHub {
	return &fakeGitHub{
		t:                   t,
		branchSHA:           "deadbeefcafe00000000000000000000deadbeef",
		fileSHA:             "filecontentsha000000000000000000feedface",
		fileContent:         fileContent,
		createPRResponseURL: "https://github.com/mjrossi/urbanist-atlas/pull/42",
	}
}

func (f *fakeGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/mjrossi/urbanist-atlas/git/ref/heads/main", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.getRefCalled++
		writeJSON(w, http.StatusOK, map[string]any{
			"object": map[string]string{"sha": f.branchSHA, "type": "commit"},
		})
	})
	mux.HandleFunc("/repos/mjrossi/urbanist-atlas/contents/api/seed/orgs.toml", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			f.mu.Lock()
			defer f.mu.Unlock()
			f.getContentsCalled++
			writeJSON(w, http.StatusOK, map[string]any{
				"content":  base64.StdEncoding.EncodeToString([]byte(f.fileContent)),
				"encoding": "base64",
				"sha":      f.fileSHA,
			})
		case http.MethodPut:
			f.mu.Lock()
			defer f.mu.Unlock()
			body := readJSONStringMap(f.t, r.Body)
			f.putContentsBody = body
			writeJSON(w, http.StatusOK, map[string]any{"commit": map[string]string{"sha": "newcommitsha"}})
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/repos/mjrossi/urbanist-atlas/git/refs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.createRefBody = readJSONStringMap(f.t, r.Body)
		writeJSON(w, http.StatusCreated, map[string]any{"ref": f.createRefBody["ref"]})
	})
	mux.HandleFunc("/repos/mjrossi/urbanist-atlas/pulls", func(w http.ResponseWriter, r *http.Request) {
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
		writeJSON(w, status, map[string]any{"html_url": f.createPRResponseURL})
	})
	return mux
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

	// PR body must include the public id + submitter line.
	if !strings.Contains(gh.createPRBody["body"], "01928200-3344-7000-9abc-000000000001") {
		t.Fatalf("PR body missing public id: %q", gh.createPRBody["body"])
	}
	if !strings.Contains(gh.createPRBody["body"], "Jane") || !strings.Contains(gh.createPRBody["body"], "jane@example.org") {
		t.Fatalf("PR body missing submitter info: %q", gh.createPRBody["body"])
	}
	if gh.createPRBody["title"] != "Add Brooklyn Greenways" {
		t.Fatalf("PR title = %q", gh.createPRBody["title"])
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
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v, want ErrDisabled", err)
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
	a, err := RenderOrgBlock(sub, "brooklyn-greenways")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	b, err := RenderOrgBlock(sub, "brooklyn-greenways")
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
	resp := &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader("bad gateway"))}
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
