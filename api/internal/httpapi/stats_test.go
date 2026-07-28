package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestStats_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Errorf("Content-Type: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), "https://urbanistatlas.com"; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}

	var stats oapi.Stats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.TotalOrgCount <= 0 {
		t.Errorf("total_org_count: want > 0, got %d", stats.TotalOrgCount)
	}
	if stats.TotalRegionCount < stats.BrowseRegionCount {
		t.Errorf("total_region_count (%d) < browse_region_count (%d): the browse set is a subset of the graph",
			stats.TotalRegionCount, stats.BrowseRegionCount)
	}
	if stats.ByCountry == nil {
		t.Errorf("by_country: want [] on the wire, got null")
	}
	for i, c := range stats.ByCountry {
		if c.Country == "" {
			t.Errorf("by_country[%d].country: want a country code, got empty", i)
		}
		if c.OrgCount > stats.TotalOrgCount {
			t.Errorf("by_country[%d].org_count (%d) > total_org_count (%d)", i, c.OrgCount, stats.TotalOrgCount)
		}
	}
}

// TestStats_TotalExceedsBrowseSubsetSum is the regression guard at the
// HTTP layer. The production frontend computed its org total by summing
// direct_org_count over /api/v1/regions, which silently omits every org
// attached solely to a non-browseable region (state, province, borough,
// multi-state). /stats must not agree with that sum on a dataset that
// contains such an org — if it ever does, the endpoint has been
// reimplemented in terms of the browse subset and the bug is back.
func TestStats_TotalExceedsBrowseSubsetSum(t *testing.T) {
	srv := newTestServer(t)

	var stats oapi.Stats
	statsResp, err := http.Get(srv.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET /stats: %v", err)
	}
	defer statsResp.Body.Close()
	if err := json.NewDecoder(statsResp.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}

	var env oapi.RegionSummariesEnvelope
	regionsResp, err := http.Get(srv.URL + "/api/v1/regions")
	if err != nil {
		t.Fatalf("GET /regions: %v", err)
	}
	defer regionsResp.Body.Close()
	if err := json.NewDecoder(regionsResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode regions: %v", err)
	}

	if got, want := stats.BrowseRegionCount, int32(len(env.Data)); got != want {
		t.Errorf("browse_region_count = %d, want %d (must equal len(/regions data))", got, want)
	}

	var directSum int32
	for _, rs := range env.Data {
		directSum += rs.DirectOrgCount
	}
	// Strict inequality is the whole guard: the dev fixtures attach
	// tri-state-transportation-campaign solely to the ny state region,
	// which is invisible to the browse subset, so a source count must
	// exceed the sum. Equality means the handler has been rewired onto
	// the browse subset and the bug is back.
	if stats.TotalOrgCount <= directSum {
		t.Errorf("total_org_count = %d must strictly exceed sum(direct_org_count) = %d — the fixtures contain an org attached only to a non-browseable region",
			stats.TotalOrgCount, directSum)
	}
}

// TestStats_EmptyStore_ByCountryIsEmptyArray pins the adapter's
// []-not-null guarantee where it can actually fail. The fixture-backed
// happy-path test always produces country rows, so its nil check is
// trivially satisfied; only an empty store exercises the make() in
// toOAPIStats. Asserting on the raw body is deliberate — decoding into
// oapi.Stats maps both null and [] to a nil slice.
func TestStats_EmptyStore_ByCountryIsEmptyArray(t *testing.T) {
	store := atlas.NewMemStore()
	handler := New(Config{
		Store:      store,
		Logger:     slog.New(slog.DiscardHandler),
		APIVersion: "v1",
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, ok := body["by_country"]
	if !ok {
		t.Fatal("by_country: key missing from response body")
	}
	if got := string(raw); got != "[]" {
		t.Errorf("by_country on the wire: want [], got %s", got)
	}
}

// TestStats_401_MissingClientSecret pins that /stats sits INSIDE the
// Phase 1 client-secret gate. Only /healthz and /api/v1/openapi.yaml
// are exempt; a dataset-size endpoint is not a discovery endpoint.
func TestStats_401_MissingClientSecret(t *testing.T) {
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	handler := New(Config{
		Store:        store,
		Logger:       slog.New(slog.DiscardHandler),
		APIVersion:   "v1",
		ClientSecret: "the-secret",
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/stats")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", resp.StatusCode)
	}
	var prob oapi.ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if prob.Type != problemUnauthorized {
		t.Errorf("type: want %q, got %q", problemUnauthorized, prob.Type)
	}
}
