package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// newUsageTestServer wires the full router with usage recording on, so
// the read-handler -> recorder -> store -> admin-read path can be
// exercised end-to-end. Mirrors newCoverageTestServer. tweak, if
// non-nil, adjusts the Config before the router is built.
func newUsageTestServer(t *testing.T, tweak func(*Config)) (*httptest.Server, *usage.Recorder) {
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

	// A long interval: tests drive Flush explicitly so nothing races.
	rec := usage.New(subs, time.Hour, 400, slog.New(slog.DiscardHandler))

	cfg := Config{
		Store:       store,
		Logger:      slog.New(slog.DiscardHandler),
		APIVersion:  "v1",
		Submissions: subs,
		AdminToken:  testAdminToken,
		Metrics:     NewMetrics(),
		Usage:       rec,
		UsageCounts: subs,
	}
	if tweak != nil {
		tweak(&cfg)
	}
	srv := httptest.NewServer(New(cfg))
	t.Cleanup(srv.Close)
	return srv, rec
}

// adminGet issues an authorized admin GET and returns the response.
func adminGet(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin get: %v", err)
	}
	return resp
}

// getUsage performs an authorized admin usage read and decodes it into
// the GENERATED wire type, so the test pins the published contract
// rather than the internal struct's tags.
func getUsage(t *testing.T, srv *httptest.Server, query string) []oapi.UsageCount {
	t.Helper()
	resp := adminGet(t, srv, "/api/v1/admin/usage?"+query)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out []oapi.UsageCount
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// assertUsageStatus drives one admin usage read and checks the status.
func assertUsageStatus(t *testing.T, srv *httptest.Server, query string, want int) {
	t.Helper()
	resp := adminGet(t, srv, "/api/v1/admin/usage?"+query)
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Errorf("GET ?%s status = %d, want %d", query, resp.StatusCode, want)
	}
}

func TestUsage_RegionViewRecordedAndListed(t *testing.T) {
	srv, rec := newUsageTestServer(t, nil)

	// Capture the day BEFORE the request: computing it afterwards would
	// flake if the two straddled UTC midnight.
	today := time.Now().UTC().Format("2006-01-02")

	// Drive a region detail fetch through the real handler.
	resp, err := http.Get(srv.URL + "/api/v1/regions/brooklyn-ny")
	if err != nil {
		t.Fatalf("region fetch: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("region fetch status = %d, want 200", resp.StatusCode)
	}
	if err := rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Default grouping: summed over the range, so no day on the row.
	got := getUsage(t, srv, "from="+today+"&to="+today+"&kind=region_view")
	if len(got) != 1 {
		t.Fatalf("want 1 region_view bucket, got %+v", got)
	}
	if got[0].Key != "brooklyn-ny" || got[0].Count != 1 {
		t.Errorf("bucket = %+v, want brooklyn-ny/1", got[0])
	}
	if got[0].Day != nil {
		t.Errorf("day = %v, want omitted on a range-aggregated row", got[0].Day)
	}

	// group_by=day carries the stored day.
	byDay := getUsage(t, srv, "from="+today+"&to="+today+"&kind=region_view&group_by=day")
	if len(byDay) != 1 {
		t.Fatalf("want 1 per-day bucket, got %+v", byDay)
	}
	if byDay[0].Day == nil {
		t.Fatalf("group_by=day must carry a day, got %+v", byDay[0])
	}
	if got := byDay[0].Day.Format("2006-01-02"); got != today {
		t.Errorf("day = %q, want %q", got, today)
	}
}

func TestUsage_LookupRecordsResolvedRegionNotPostalCode(t *testing.T) {
	// The privacy contract: the bucket key is the resolved region slug.
	// A raw postal code must never appear in the rollup table.
	srv, rec := newUsageTestServer(t, nil)
	today := time.Now().UTC().Format("2006-01-02")

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200", resp.StatusCode)
	}
	if err := rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	all := getUsage(t, srv, "from="+today+"&to="+today)
	for _, c := range all {
		if c.Key == "11217" {
			t.Fatalf("raw postal code leaked into usage_daily: %+v", c)
		}
	}

	// The resolved-region bucket must be the smallest curated anchor
	// (Brooklyn), not the postal code and not a broader ancestor.
	var sawResolvedRegion, sawHit bool
	for _, c := range all {
		if string(c.Kind) == usage.KindLookup && c.Key == "brooklyn-ny" {
			sawResolvedRegion = true
		}
		if string(c.Kind) == usage.KindLookupResult && c.Key == "hit" {
			sawHit = true
		}
	}
	if !sawResolvedRegion {
		t.Errorf("expected a lookup bucket keyed brooklyn-ny, got %+v", all)
	}
	if !sawHit {
		t.Errorf("expected a lookup_result=hit bucket, got %+v", all)
	}
}

func TestUsage_UnresolvedSlugsAreNotRecorded(t *testing.T) {
	// A 404 slug is raw path input. Bucketing it would let any caller
	// mint unbounded rows in a 400-day table on the submission volume,
	// and an unresolved slug is not content popularity anyway. The
	// hit/miss signal lives in Prometheus instead.
	srv, rec := newUsageTestServer(t, nil)
	today := time.Now().UTC().Format("2006-01-02")

	for _, path := range []string{
		"/api/v1/regions/definitely-not-a-region",
		"/api/v1/orgs/definitely-not-an-org",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, resp.StatusCode)
		}
	}
	if err := rec.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	all := getUsage(t, srv, "from="+today+"&to="+today)
	if len(all) != 0 {
		t.Errorf("404s must record nothing, got %+v", all)
	}
}

func TestUsage_AdminReachableWithoutClientSecret(t *testing.T) {
	// The admin subtree is gated by the bearer token alone. It must NOT
	// also require X-Atlas-Client: that header exists for the browser and
	// ships in the public SPA bundle, and requiring it would break every
	// server-to-server caller — including the usage-digest workflow,
	// which would then fail silently behind continue-on-error.
	const clientSecret = "phase-1-browser-secret"
	srv, _ := newUsageTestServer(t, func(c *Config) { c.ClientSecret = clientSecret })

	assertUsageStatus(t, srv, "from=2026-08-01&to=2026-08-31", http.StatusOK)

	// Same server, public route: the client gate is genuinely active, so
	// the assertion above is about admin placement, not a disabled gate.
	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("public route without X-Atlas-Client = %d, want 401", resp.StatusCode)
	}
}

func TestUsage_ParamValidation(t *testing.T) {
	srv, _ := newUsageTestServer(t, nil)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"missing to", "from=2026-08-01", http.StatusBadRequest},
		{"missing from", "to=2026-08-31", http.StatusBadRequest},
		{"malformed from", "from=August&to=2026-08-31", http.StatusBadRequest},
		{"non-padded date", "from=2026-8-1&to=2026-08-31", http.StatusBadRequest},
		// An inverted range would otherwise return an empty 200, which
		// reads as "no traffic" and would hide a digest date bug.
		{"inverted range", "from=2026-08-31&to=2026-08-01", http.StatusBadRequest},
		// A typo'd kind can never match a row (the CHECK constraint), so
		// passing it through would return an empty 200.
		{"unknown kind", "from=2026-08-01&to=2026-08-31&kind=regionview", http.StatusBadRequest},
		{"unknown group_by", "from=2026-08-01&to=2026-08-31&group_by=week", http.StatusBadRequest},
		{"limit over cap", "from=2026-08-01&to=2026-08-31&limit=1001", http.StatusBadRequest},
		{"limit zero", "from=2026-08-01&to=2026-08-31&limit=0", http.StatusBadRequest},
		{"single day range", "from=2026-08-01&to=2026-08-01", http.StatusOK},
		{"valid kind", "from=2026-08-01&to=2026-08-31&kind=lookup_tier", http.StatusOK},
		{"valid group_by", "from=2026-08-01&to=2026-08-31&group_by=day", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertUsageStatus(t, srv, tc.query, tc.want)
		})
	}
}

func TestUsage_RequiresBearerToken(t *testing.T) {
	srv, _ := newUsageTestServer(t, nil)

	resp, err := http.Get(srv.URL + "/api/v1/admin/usage?from=2026-08-01&to=2026-08-31")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestUsage_UnconfiguredAdminTokenReturns503(t *testing.T) {
	// An empty URBANIST_ADMIN_TOKEN disables admin endpoints rather than
	// exposing them; 503 (not 401) so a misconfigured deploy is
	// distinguishable from a bad token. Documented as AdminUnavailable.
	srv, _ := newUsageTestServer(t, func(c *Config) { c.AdminToken = "" })

	assertUsageStatus(t, srv, "from=2026-08-01&to=2026-08-31", http.StatusServiceUnavailable)
}
