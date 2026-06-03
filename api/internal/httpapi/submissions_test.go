package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

const testAdminToken = "test-admin-token"

// recordingEnqueuer captures what Enqueue was called with and lets the
// test choose what to return. Mirrors how the GitHub PR worker will be
// injected once Phase 3 lands.
type recordingEnqueuer struct {
	mu      sync.Mutex
	calls   []atlas.Submission
	failErr error
}

func (e *recordingEnqueuer) Enqueue(_ context.Context, sub atlas.Submission) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, sub)
	return e.failErr
}

func (e *recordingEnqueuer) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

type submissionsTestRig struct {
	srv     *httptest.Server
	subs    atlas.SubmissionStore
	store   atlas.Store
	enq     *recordingEnqueuer
	limiter *ipRateLimiter
	metrics *Metrics
}

func newSubmissionsTestServer(t *testing.T) *submissionsTestRig {
	t.Helper()

	subs, err := sqlite.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	t.Cleanup(func() { _ = subs.Close() })
	if err := subs.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)

	enq := &recordingEnqueuer{}
	limiter := newIPRateLimiter(2, time.Hour) // tight ceiling for the 429 test
	logger := slog.New(slog.DiscardHandler)
	metrics := NewMetrics()
	r := buildSubmissionRoutes(subs, store, enq, limiter, logger, metrics)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &submissionsTestRig{
		srv:     srv,
		subs:    subs,
		store:   store,
		enq:     enq,
		limiter: limiter,
		metrics: metrics,
	}
}

// buildSubmissionRoutes mirrors the chi route table the production
// router wires up, scoped to just the submission endpoints + the
// requestID middleware (so problem documents carry a request_id).
// This lets the test exercise the real handler-middleware stack
// (bearer auth, rate limit, problem responses) without spinning up
// every /api/v1/* sibling.
func buildSubmissionRoutes(subs atlas.SubmissionStore, store atlas.Store, enq PromotionEnqueuer, limiter *ipRateLimiter, logger *slog.Logger, m *Metrics) http.Handler {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/submissions", createSubmissionHandler(subs, store, limiter, logger, m))
		r.Route("/admin", func(r chi.Router) {
			r.Use(bearerAuthMiddleware(testAdminToken))
			r.Get("/submissions", listSubmissionsHandler(subs, logger))
			r.Post("/submissions/{id}/approve", approveSubmissionHandler(subs, enq, logger))
			r.Post("/submissions/{id}/reject", rejectSubmissionHandler(subs, logger))
		})
	})
	return r
}

func postJSON(t *testing.T, url string, body any, headers map[string]string) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func goodSubmissionBody() map[string]any {
	return map[string]any{
		"payload": map[string]any{
			"name":         "Brooklyn Greenways",
			"short_desc":   "Volunteers expanding the protected-lane network.",
			"website_url":  "https://example.org/brooklyn-greenways",
			"region_slugs": []string{"brooklyn-ny"},
		},
		"submitter_name":  "Jane",
		"submitter_email": "jane@example.org",
	}
}

func TestCreateSubmission_HappyPath(t *testing.T) {
	rig := newSubmissionsTestServer(t)

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", resp.StatusCode)
	}
	var got struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status != "pending" {
		t.Fatalf("status field: want pending, got %q", got.Status)
	}
	if len(got.ID) != 36 {
		t.Fatalf("id length: want 36, got %d (%q)", len(got.ID), got.ID)
	}
}

// TestCreateSubmission_RejectsTagsField pins the wire-compat behavior
// after `tags` was dropped from SubmissionPayload: the create handler
// decodes with DisallowUnknownFields, so a client still sending a `tags`
// key in the payload now gets a 400 rather than having it silently
// accepted. Editors assign tags during PR review instead.
func TestCreateSubmission_RejectsTagsField(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	body["payload"].(map[string]any)["tags"] = []string{"cycling"}

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400 for unknown `tags` field, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("content-type: want problem+json, got %q", got)
	}
}

func TestCreateSubmission_MissingField_Returns400Validation(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	delete(body["payload"].(map[string]any), "name")

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("content-type: want problem+json, got %q", got)
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := problem["type"]; got != "https://urbanistatlas.com/problems/validation" {
		t.Fatalf("problem type: %v", got)
	}
}

func TestCreateSubmission_UnknownRegionSlug_Returns400(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	body["payload"].(map[string]any)["region_slugs"] = []string{"no-such-region"}

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
	var problem map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&problem)
	// Bad region slugs now surface via the per-field `errors` map
	// rather than the top-level `detail`. Mirrors the wire shape the
	// SPA's Submit page consumes for per-input validator dispatch.
	errs, _ := problem["errors"].(map[string]any)
	if got, _ := errs["region_slugs"].(string); !strings.Contains(got, "no-such-region") {
		t.Fatalf("errors.region_slugs does not mention bad slug: %v", problem)
	}
}

// Empty region_slugs is accepted on the public-submission wire because
// most submitters don't know the canonical slug. Editors finalize the
// region in PR review from submitter_note context. The loader-side
// validation in ValidateOrgFields keeps the "at least one" rule for
// records already in the orgs.toml dataset.
func TestCreateSubmission_EmptyRegionSlugs_Returns201(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	body["payload"].(map[string]any)["region_slugs"] = []string{}

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", resp.StatusCode)
	}
}

func TestCreateSubmission_OmittedRegionSlugs_Returns201(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	delete(body["payload"].(map[string]any), "region_slugs")

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status: want 201, got %d", resp.StatusCode)
	}
}

func TestCreateSubmission_RateLimit_Returns429WithRetryAfter(t *testing.T) {
	rig := newSubmissionsTestServer(t)

	// Limiter is sized 2/hr in newSubmissionsTestServer; first two
	// requests should succeed, the third should 429.
	for i := range 2 {
		resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("warmup %d: status %d", i, resp.StatusCode)
		}
	}
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status: want 429, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header missing")
	} else if n, err := strconv.Atoi(got); err != nil || n < 1 {
		t.Fatalf("Retry-After not a positive int: %q", got)
	}
}

func TestAdminList_RequiresBearer(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	resp, err := http.Get(rig.srv.URL + "/api/v1/admin/submissions")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status without auth: want 401, got %d", resp.StatusCode)
	}
}

func TestAdminList_ReturnsPendingByDefault(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	// Seed two pending, one rejected.
	for range 2 {
		_ = postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	}
	// Reject one of them directly via the store so the list test
	// doesn't depend on the reject handler.
	all, _ := rig.subs.List(context.Background(), atlas.ListSubmissionsQuery{Status: atlas.SubmissionPending})
	if len(all) < 2 {
		t.Fatalf("expected at least 2 pending after seeding, got %d", len(all))
	}
	if _, err := rig.subs.Reject(context.Background(), all[0].PublicID, "test rejection"); err != nil {
		t.Fatalf("seed reject: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, rig.srv.URL+"/api/v1/admin/submissions", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, row := range got {
		if row["status"] != "pending" {
			t.Fatalf("expected only pending in default list, got %v", row["status"])
		}
	}
}

func TestAdminList_PaginatesWithCursor(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	rig.limiter = newIPRateLimiter(100, time.Hour) // unused — limiter is wired at handler-build time

	// Seed 5 pending submissions directly via the store so we sidestep
	// the per-IP limiter and get deterministic created_at ordering.
	for i := range 5 {
		payload := atlas.SubmissionPayload{
			Name:        "Org " + strconv.Itoa(i),
			ShortDesc:   "desc",
			WebsiteURL:  "https://example.org/" + strconv.Itoa(i),
			RegionSlugs: []string{"brooklyn-ny"},
		}
		if _, err := rig.subs.Create(context.Background(), atlas.NewSubmissionInput{Payload: payload}); err != nil {
			t.Fatalf("seed Create %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond) // distinct created_at per row
	}

	// First page: limit=2.
	req, _ := http.NewRequest(http.MethodGet, rig.srv.URL+"/api/v1/admin/submissions?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("page 1 status: %d", resp.StatusCode)
	}
	var page1 []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&page1); err != nil {
		t.Fatalf("decode page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 length: want 2, got %d", len(page1))
	}
	cursor := resp.Header.Get("X-Next-Cursor")
	if cursor == "" {
		t.Fatal("page 1: X-Next-Cursor header missing with 5 rows and limit=2")
	}

	// Second page: feed back the cursor.
	req2, _ := http.NewRequest(http.MethodGet, rig.srv.URL+"/api/v1/admin/submissions?limit=2&cursor="+cursor, nil)
	req2.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	defer resp2.Body.Close()
	var page2 []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&page2); err != nil {
		t.Fatalf("decode page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2 length: want 2, got %d", len(page2))
	}
	// Pages must not overlap.
	if page1[0]["id"] == page2[0]["id"] || page1[1]["id"] == page2[0]["id"] {
		t.Fatalf("pages overlap: page1 ids %v %v, page2[0] id %v", page1[0]["id"], page1[1]["id"], page2[0]["id"])
	}
}

func TestAdminList_RejectsBadCursor(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, rig.srv.URL+"/api/v1/admin/submissions?cursor=not-a-cursor", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
}

func TestAdminList_RejectsBadLimit(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	for _, raw := range []string{"0", "201", "-1", "abc"} {
		req, _ := http.NewRequest(http.MethodGet, rig.srv.URL+"/api/v1/admin/submissions?limit="+raw, nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("do %q: %v", raw, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("limit=%q: status want 400, got %d", raw, resp.StatusCode)
		}
	}
}

func TestCreateSubmission_RejectsNonHTTPWebsiteURL(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	body["payload"].(map[string]any)["website_url"] = "javascript:alert(1)"
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
}

func TestCreateSubmission_RejectsMalformedEmail(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	body := goodSubmissionBody()
	body["submitter_email"] = "not-an-email"
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", body, nil)
	defer resp.Body.Close()
	// oapi's openapi_types.Email is a string alias — Go's JSON
	// decoder accepts any string. Server-side validateSubmitterFields
	// is what catches this.
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
}

func TestApprove_EnqueuesAndReturnsWorkerDisabledWhenNil(t *testing.T) {
	rig := newSubmissionsTestServer(t)

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	var created map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&created)
	_ = resp.Body.Close()
	id := created["id"].(string)

	// Approve with the rig's recording enqueuer (returns nil → success).
	req, _ := http.NewRequest(http.MethodPost, rig.srv.URL+"/api/v1/admin/submissions/"+id+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	ar, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	defer ar.Body.Close()
	if ar.StatusCode != http.StatusOK {
		t.Fatalf("approve status: %d", ar.StatusCode)
	}
	if rig.enq.callCount() != 1 {
		t.Fatalf("enqueue called %d times, want 1", rig.enq.callCount())
	}
}

func TestApprove_PersistsWorkerDisabledError(t *testing.T) {
	// Build a rig with a nil enqueuer to simulate local dev (no GitHub token).
	subs, err := sqlite.Open("file::memory:?cache=shared&mode=memory")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = subs.Close() })
	if err := subs.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)

	limiter := newIPRateLimiter(10, time.Hour)
	logger := slog.New(slog.DiscardHandler)
	r := buildSubmissionRoutes(subs, store, nil /* no enqueuer */, limiter, logger, NewMetrics())
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	var created map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&created)
	_ = resp.Body.Close()
	id := created["id"].(string)

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/admin/submissions/"+id+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	ar, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	defer ar.Body.Close()
	if ar.StatusCode != http.StatusOK {
		t.Fatalf("approve status: %d", ar.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(ar.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errStr, _ := body["promotion_error"].(string); !strings.Contains(strings.ToLower(errStr), "worker disabled") {
		t.Fatalf("promotion_error should mention worker disabled, got %v", body["promotion_error"])
	}
	if body["status"] != "approved" {
		t.Fatalf("status should be approved even when worker disabled, got %v", body["status"])
	}
}

func TestApprove_AlreadyProcessed_Returns409(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	var created map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&created)
	_ = resp.Body.Close()
	id := created["id"].(string)

	for _, want := range []int{http.StatusOK, http.StatusConflict} {
		req, _ := http.NewRequest(http.MethodPost, rig.srv.URL+"/api/v1/admin/submissions/"+id+"/approve", nil)
		req.Header.Set("Authorization", "Bearer "+testAdminToken)
		ar, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("approve: %v", err)
		}
		_ = ar.Body.Close()
		if ar.StatusCode != want {
			t.Fatalf("approve status: want %d, got %d", want, ar.StatusCode)
		}
	}
}

func TestApprove_UnknownID_Returns404(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	// Well-formed UUID that just doesn't exist.
	req, _ := http.NewRequest(http.MethodPost, rig.srv.URL+"/api/v1/admin/submissions/0192f6c0-1c2c-7000-9000-000000000099/approve", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	ar, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	defer ar.Body.Close()
	if ar.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", ar.StatusCode)
	}
}

func TestApprove_BadUUID_Returns400(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	req, _ := http.NewRequest(http.MethodPost, rig.srv.URL+"/api/v1/admin/submissions/not-a-uuid/approve", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	ar, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	defer ar.Body.Close()
	if ar.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", ar.StatusCode)
	}
}

func TestReject_RequiresReason(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	var created map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&created)
	_ = resp.Body.Close()
	id := created["id"].(string)

	r := postJSON(t, rig.srv.URL+"/api/v1/admin/submissions/"+id+"/reject", map[string]any{"reason": ""}, map[string]string{
		"Authorization": "Bearer " + testAdminToken,
	})
	defer r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", r.StatusCode)
	}
}

func TestReject_HappyPath(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	var created map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&created)
	_ = resp.Body.Close()
	id := created["id"].(string)

	r := postJSON(t, rig.srv.URL+"/api/v1/admin/submissions/"+id+"/reject", map[string]any{"reason": "duplicate"}, map[string]string{
		"Authorization": "Bearer " + testAdminToken,
	})
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("status: %d", r.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["status"] != "rejected" {
		t.Fatalf("status: %v", body["status"])
	}
	if body["rejection_reason"] != "duplicate" {
		t.Fatalf("reason: %v", body["rejection_reason"])
	}
}
