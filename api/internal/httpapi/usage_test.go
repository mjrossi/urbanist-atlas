package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// newUsageTestServer wires the full router with usage recording on, so
// the read-handler -> recorder -> store -> admin-read path can be
// exercised end-to-end. Mirrors newCoverageTestServer.
func newUsageTestServer(t *testing.T) (*httptest.Server, *usage.Recorder) {
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

	handler := New(Config{
		Store:       store,
		Logger:      slog.New(slog.DiscardHandler),
		APIVersion:  "v1",
		Submissions: subs,
		AdminToken:  testAdminToken,
		Metrics:     NewMetrics(),
		Usage:       rec,
		UsageCounts: subs,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, rec
}

// getUsage performs an authorized admin usage read and decodes it.
func getUsage(t *testing.T, srv *httptest.Server, query string) []atlas.UsageCount {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/usage?"+query, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get usage: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out []atlas.UsageCount
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestUsage_RegionViewRecordedAndListed(t *testing.T) {
	srv, rec := newUsageTestServer(t)

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

	today := time.Now().UTC().Format("2006-01-02")
	got := getUsage(t, srv, "from="+today+"&to="+today+"&kind=region_view")
	if len(got) != 1 {
		t.Fatalf("want 1 region_view bucket, got %+v", got)
	}
	if got[0].Key != "brooklyn-ny" || got[0].Count != 1 {
		t.Errorf("bucket = %+v, want brooklyn-ny/1", got[0])
	}
	if got[0].Day != today {
		t.Errorf("day = %q, want %q", got[0].Day, today)
	}
}

func TestUsage_LookupRecordsResolvedRegionNotPostalCode(t *testing.T) {
	// The privacy contract: the bucket key is the resolved region slug.
	// A raw postal code must never appear in the rollup table.
	srv, rec := newUsageTestServer(t)

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

	today := time.Now().UTC().Format("2006-01-02")
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
		if c.Kind == usage.KindLookup && c.Key == "brooklyn-ny" {
			sawResolvedRegion = true
		}
		if c.Kind == usage.KindLookupResult && c.Key == "hit" {
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

func TestUsage_RequiresFromAndTo(t *testing.T) {
	// Without a bounded range the handler would scan the whole table.
	srv, _ := newUsageTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/usage?from=2026-08-01", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUsage_RejectsMalformedDate(t *testing.T) {
	srv, _ := newUsageTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/usage?from=August&to=2026-08-31", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestUsage_RequiresBearerToken(t *testing.T) {
	srv, _ := newUsageTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/admin/usage?from=2026-08-01&to=2026-08-31")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}
