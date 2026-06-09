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
	createRefCalled      int
	createRefBody        map[string]string
	putContentsBody      map[string]string
	createPRBody         map[string]string
	createPRResponseURL  string
	failOnCreatePRStatus int

	// getRefTransientFailures: while > 0, the get-ref endpoint answers
	// with getRefTransientStatus (default 503) and decrements, so a
	// test can exercise the idempotent-GET retry path before the call
	// finally succeeds.
	getRefTransientFailures int
	getRefTransientStatus   int

	// createRefAlreadyExists makes the create-ref endpoint answer with a
	// 422 "Reference already exists" body, simulating a branch a prior
	// openPR attempt left behind. createBranch must treat that as success
	// (idempotent whole-pipeline retry, issue #24).
	createRefAlreadyExists bool

	// getRefBlockUntilCtx makes the get-ref endpoint hang until the
	// request's context is canceled (simulating a wedged GitHub call).
	// Used by the issue #25 shutdown-drain-deadline test to prove a
	// stuck remote can't pin process exit past shutdownDrainTimeout.
	getRefBlockUntilCtx bool
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
		f.mu.Unlock()
		if block {
			// Hang until the client cancels (the request ctx fires when
			// the worker's per-call deadline elapses). Never respond.
			<-r.Context().Done()
			return
		}
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
		f.createRefCalled++
		f.createRefBody = readJSONStringMap(f.t, r.Body)
		if f.createRefAlreadyExists {
			// Shape mirrors GitHub's real 422 for a duplicate ref.
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"message": "Reference already exists",
			})
			return
		}
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
// date-only added_at line matching the approval date passed in. This
// is the single line that lets a moderator's "approve" click flow
// through to the homepage "Recently indexed" strip with the right
// date — without it, the next deploy of the merged PR would fail
// the Phase 4 required-field check at boot and the whole bundle
// would refuse to load.
func TestRenderOrgBlock_AddedAt(t *testing.T) {
	sub := sampleSubmission()
	// CreatedAt is May 28 (submission time); approval is later.
	// The plan explicitly forbids reusing sub.CreatedAt, so pick a
	// distinct date and assert THAT one — proves the field is sourced
	// from the caller's clock, not the submission row.
	addedAt := time.Date(2026, 6, 1, 9, 15, 0, 0, time.UTC)
	out, err := RenderOrgBlock(sub, "brooklyn-greenways", addedAt)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Unquoted ISO date — matches the seed convention (see Phase 3
	// backfill diff) and parses as toml.LocalDate, which is what the
	// loader's required-field check expects.
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
// affordance: public submissions no longer carry tags (the `tags` field
// was dropped from SubmissionPayload), so sub.Payload.Tags is nil. The
// rendered block must still emit a `tags = []` line — an explicit empty
// array the editor fills in during PR review — rather than omitting the
// key. region_slugs, when present, is rendered as-is.
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
// issue #27 truncated-read marker path in apiError.
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

// TestWorker_RetriesTransientGET pins the issue #24 idempotent-GET
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

// TestWorker_CreateBranch_AlreadyExistsIsIdempotent pins issue #24's
// whole-pipeline idempotency: when the deterministic submission branch
// already exists (a prior attempt got that far then failed), the 422
// "Reference already exists" is treated as success and the pipeline
// proceeds to put-contents + create-PR.
func TestWorker_CreateBranch_AlreadyExistsIsIdempotent(t *testing.T) {
	gh := newFakeGitHub(t, "# orgs.toml\n")
	gh.createRefAlreadyExists = true
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

// TestWorker_ShutdownDrain_BoundsWedgedCall pins issue #25: a job
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
	// it through drainAndExit (the issue #25 code path).
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
