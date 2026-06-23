package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// newMetricsTestServer mirrors newTestServer (lookup_test.go) but keeps
// a reference to the *Metrics so the test can scrape the registry
// directly. The dev fixtures back the store: 11217 is a known US hit,
// 00000 a known miss.
func newMetricsTestServer(t *testing.T) (*httptest.Server, *Metrics) {
	t.Helper()
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	m := NewMetrics()
	handler := New(Config{
		Store:      store,
		Logger:     slog.New(slog.DiscardHandler),
		APIVersion: "v1",
		Metrics:    m,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, m
}

func TestMetrics_LookupCounters(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	// One known hit, one known miss, one APO/FPO/DPO military ZIP (which
	// is tracked under its own "military" result, not "miss").
	for _, postal := range []string{"11217", "00000", "09000"} {
		resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=" + postal + "&country=US")
		if err != nil {
			t.Fatalf("GET %s: %v", postal, err)
		}
		resp.Body.Close()
	}

	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("US", "hit")); got != 1 {
		t.Errorf("atlas_lookup_total{country=US,result=hit} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("US", "miss")); got != 1 {
		t.Errorf("atlas_lookup_total{country=US,result=miss} = %v, want 1", got)
	}
	// The military ZIP must increment "military", not "miss".
	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("US", "military")); got != 1 {
		t.Errorf("atlas_lookup_total{country=US,result=military} = %v, want 1", got)
	}
}

func TestMetricCountry(t *testing.T) {
	cases := map[string]string{
		"US":  "US",
		"CA":  "CA",
		"ZZ":  "other",
		"":    "other",
		"FOO": "other",
		"us":  "other", // already upper-cased at the handler boundary
	}
	for in, want := range cases {
		if got := metricCountry(in); got != want {
			t.Errorf("metricCountry(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestMetrics_UnknownCountryBucketed guards the cardinality bound: a
// miss for an unrecognized country must land under country="other"
// rather than minting a new label series per arbitrary input.
func TestMetrics_UnknownCountryBucketed(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	// Two distinct unknown country codes, both misses.
	for _, country := range []string{"ZZ", "XX"} {
		resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=00000&country=" + country)
		if err != nil {
			t.Fatalf("GET %s: %v", country, err)
		}
		resp.Body.Close()
	}

	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("other", "miss")); got != 2 {
		t.Errorf("atlas_lookup_total{country=other,result=miss} = %v, want 2", got)
	}
	// The raw inputs must not have created their own series.
	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("ZZ", "miss")); got != 0 {
		t.Errorf("atlas_lookup_total{country=ZZ,result=miss} = %v, want 0 (should bucket to other)", got)
	}
}

// TestMetrics_SubmissionCounters exercises the createSubmissionHandler
// counter paths through the real route table: one accepted submission
// (status=created) and one that fails field validation
// (status=rejected_validation).
func TestMetrics_SubmissionCounters(t *testing.T) {
	rig := newSubmissionsTestServer(t)

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	resp.Body.Close()

	bad := goodSubmissionBody()
	delete(bad["payload"].(map[string]any), "name") // required field
	resp = postJSON(t, rig.srv.URL+"/api/v1/submissions", bad, nil)
	resp.Body.Close()

	if got := testutil.ToFloat64(rig.metrics.submissions.WithLabelValues("created")); got != 1 {
		t.Errorf("atlas_submissions_total{status=created} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(rig.metrics.submissions.WithLabelValues("rejected_validation")); got != 1 {
		t.Errorf("atlas_submissions_total{status=rejected_validation} = %v, want 1", got)
	}
}

func TestMetrics_HTTPRequestsUseRoutePattern(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	// The route label must be the chi route pattern, not the raw path.
	if got := testutil.ToFloat64(m.httpRequests.WithLabelValues("/api/v1/lookup", "GET", "200")); got < 1 {
		t.Errorf("atlas_http_requests_total{route=/api/v1/lookup,method=GET,status=200} = %v, want >= 1", got)
	}
}

func TestMetrics_HandlerExposition(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	// Populate at least one atlas_ series before scraping.
	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("metrics content-type = %q, want to contain text/plain", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "atlas_lookup_total") {
		t.Fatalf("metrics body missing atlas_lookup_total; got:\n%s", body)
	}
}

func TestLookupTier(t *testing.T) {
	cases := []struct {
		local, regional, statewide int
		want                       string
	}{
		{1, 0, 0, "local"},
		{1, 2, 3, "local"}, // local wins whenever present
		{0, 1, 0, "regional"},
		{0, 1, 5, "regional"},
		{0, 0, 1, "statewide"},
		{0, 0, 0, "empty"},
	}
	for _, c := range cases {
		if got := lookupTier(c.local, c.regional, c.statewide); got != c.want {
			t.Errorf("lookupTier(%d,%d,%d) = %q, want %q", c.local, c.regional, c.statewide, got, c.want)
		}
	}
}

func TestMetricSubmissionField(t *testing.T) {
	known := []string{
		"name", "short_desc", "website_url", "contact_url",
		"region_slugs", "submitter_name", "submitter_email", "submitter_note",
	}
	for _, f := range known {
		if got := metricSubmissionField(f); got != f {
			t.Errorf("metricSubmissionField(%q) = %q, want %q (known field must pass through)", f, got, f)
		}
	}
	for _, f := range []string{"", "tags", "id", "anything-else"} {
		if got := metricSubmissionField(f); got != "other" {
			t.Errorf("metricSubmissionField(%q) = %q, want other", f, got)
		}
	}
}

func TestAdminOutcome(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{atlas.ErrSubmissionNotFound, "not_found"},
		{atlas.ErrSubmissionNotPending, "conflict"},
		{errors.New("boom"), "error"},
	}
	for _, c := range cases {
		if got := adminOutcome(c.err); got != c.want {
			t.Errorf("adminOutcome(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

// TestMetrics_LookupResultTiers pins the partition invariant: one hit
// increments exactly one tier bucket, so the four buckets sum to the
// hit count (the miss does not touch lookup_results_total).
func TestMetrics_LookupResultTiers(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	for _, postal := range []string{"11217", "00000"} { // one hit, one miss
		resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=" + postal + "&country=US")
		if err != nil {
			t.Fatalf("GET %s: %v", postal, err)
		}
		resp.Body.Close()
	}

	var sum float64
	for _, tier := range []string{"local", "regional", "statewide", "empty"} {
		sum += testutil.ToFloat64(m.lookupResults.WithLabelValues("US", tier))
	}
	if sum != 1 {
		t.Errorf("sum(atlas_lookup_results_total{country=US}) = %v, want 1", sum)
	}
}

func TestMetrics_RegionAndOrgViews(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	for _, p := range []string{
		"/api/v1/regions/brooklyn-ny",  // found
		"/api/v1/regions/no-such-slug", // not found
		"/api/v1/orgs/walk-sf",         // found
		"/api/v1/orgs/no-such-org",     // not found
	} {
		resp, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatalf("GET %s: %v", p, err)
		}
		resp.Body.Close()
	}

	checks := []struct {
		name string
		vec  *prometheus.CounterVec
		val  string
	}{
		{"region_views_total{found=true}", m.regionViews, "true"},
		{"region_views_total{found=false}", m.regionViews, "false"},
		{"org_views_total{found=true}", m.orgViews, "true"},
		{"org_views_total{found=false}", m.orgViews, "false"},
	}
	for _, c := range checks {
		if got := testutil.ToFloat64(c.vec.WithLabelValues(c.val)); got != 1 {
			t.Errorf("atlas_%s = %v, want 1", c.name, got)
		}
	}
}

func TestMetrics_RegionSearch(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	// "brooklyn" matches a fixture region; "zzznomatch" matches nothing.
	for _, q := range []string{"brooklyn", "zzznomatch"} {
		resp, err := http.Get(srv.URL + "/api/v1/regions/search?q=" + q)
		if err != nil {
			t.Fatalf("GET search %q: %v", q, err)
		}
		resp.Body.Close()
	}

	if got := testutil.ToFloat64(m.regionSearch.WithLabelValues("nonempty")); got != 1 {
		t.Errorf("atlas_region_search_total{result=nonempty} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.regionSearch.WithLabelValues("empty")); got != 1 {
		t.Errorf("atlas_region_search_total{result=empty} = %v, want 1", got)
	}
}

func TestMetrics_SubmissionValidationFields(t *testing.T) {
	rig := newSubmissionsTestServer(t)

	bad := goodSubmissionBody()
	delete(bad["payload"].(map[string]any), "name")              // → field=name
	bad["payload"].(map[string]any)["website_url"] = "not-a-url" // → field=website_url

	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", bad, nil)
	resp.Body.Close()

	if got := testutil.ToFloat64(rig.metrics.submissionValidationFailures.WithLabelValues("name")); got != 1 {
		t.Errorf("atlas_submission_validation_failures_total{field=name} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(rig.metrics.submissionValidationFailures.WithLabelValues("website_url")); got != 1 {
		t.Errorf("atlas_submission_validation_failures_total{field=website_url} = %v, want 1", got)
	}
}

func TestMetrics_AdminActions(t *testing.T) {
	rig := newSubmissionsTestServer(t)
	auth := map[string]string{"Authorization": "Bearer " + testAdminToken}

	approveID := createPendingSubmissionID(t, rig)
	rejectID := createPendingSubmissionID(t, rig)

	resp := postJSON(t, rig.srv.URL+"/api/v1/admin/submissions/"+approveID+"/approve", nil, auth)
	resp.Body.Close()
	resp = postJSON(t, rig.srv.URL+"/api/v1/admin/submissions/"+rejectID+"/reject",
		map[string]any{"reason": "duplicate"}, auth)
	resp.Body.Close()

	if got := testutil.ToFloat64(rig.metrics.adminActions.WithLabelValues("approve", "ok")); got != 1 {
		t.Errorf("atlas_admin_actions_total{action=approve,outcome=ok} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(rig.metrics.adminActions.WithLabelValues("reject", "ok")); got != 1 {
		t.Errorf("atlas_admin_actions_total{action=reject,outcome=ok} = %v, want 1", got)
	}
}

// TestMetrics_StorePingFailure drives readyHandler with a failing pinger
// and a real registry, asserting the readiness counter moved. fakePinger
// is defined in health_test.go (same package).
func TestMetrics_StorePingFailure(t *testing.T) {
	m := NewMetrics()
	h := readyHandler(fakePinger{err: errors.New("dial tcp: connection refused")},
		slog.New(slog.DiscardHandler), m)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := testutil.ToFloat64(m.storePingFailures); got != 1 {
		t.Errorf("atlas_store_ping_failures_total = %v, want 1", got)
	}
}

// createPendingSubmissionID posts a valid submission and returns its public id.
func createPendingSubmissionID(t *testing.T, rig *submissionsTestRig) string {
	t.Helper()
	resp := postJSON(t, rig.srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create submission: want 201, got %d", resp.StatusCode)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created submission: %v", err)
	}
	return created.ID
}
