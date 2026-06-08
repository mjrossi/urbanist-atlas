package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/coverage"
	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// newCoverageTestServer wires the full router with coverage capture on
// (sample rate 1.0, deterministic RNG) so the empty-result → recorder →
// store → admin-read path can be exercised end-to-end.
func newCoverageTestServer(t *testing.T) (*httptest.Server, *coverage.Recorder) {
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

	rec := coverage.New(subs, 1.0, 100, slog.New(slog.DiscardHandler))
	rec.SetRNG(func() float64 { return 0.0 }) // always sample

	handler := New(Config{
		Store:        store,
		Logger:       slog.New(slog.DiscardHandler),
		APIVersion:   "v1",
		Submissions:  subs,
		AdminToken:   testAdminToken,
		Metrics:      NewMetrics(),
		Coverage:     rec,
		CoverageGaps: subs,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestCoverageGaps_EmptySearchCapturedAndListed(t *testing.T) {
	srv, rec := newCoverageTestServer(t)

	// An empty-result search is the coverage signal; the handler fires
	// the (sampled) capture before responding.
	resp, err := http.Get(srv.URL + "/api/v1/regions/search?q=zzznomatch")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	resp.Body.Close()
	rec.Wait() // flush the fire-and-forget write

	// Unauthenticated read is rejected.
	resp, err = http.Get(srv.URL + "/api/v1/admin/coverage-gaps")
	if err != nil {
		t.Fatalf("get (no bearer): %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no-bearer status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Authenticated read returns the captured gap.
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/coverage-gaps", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get (bearer): %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var gaps []struct {
		Kind    string  `json:"kind"`
		Country *string `json:"country"`
		Input   string  `json:"input"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gaps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(gaps) != 1 {
		t.Fatalf("len = %d, want 1", len(gaps))
	}
	if gaps[0].Kind != "search" || gaps[0].Input != "zzznomatch" {
		t.Errorf("gap = %+v, want search/zzznomatch", gaps[0])
	}
	if gaps[0].Country != nil {
		t.Errorf("search gap country = %v, want omitted (nil)", *gaps[0].Country)
	}
}

func TestCoverageGaps_BadLimitReturns400(t *testing.T) {
	srv, _ := newCoverageTestServer(t)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/admin/coverage-gaps?limit=0", nil)
	req.Header.Set("Authorization", "Bearer "+testAdminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
