package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// newTestServer builds an httptest.Server backed by the full router,
// memory-store, and the dev fixtures, so handler tests run end-to-end
// through the same middleware stack the production server uses.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	handler := New(Config{
		Store:      store,
		Logger:     slog.New(slog.DiscardHandler),
		APIVersion: "v1",
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func TestLookup_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	// Tighten the Content-Type assertion to a full equality check so a
	// future regression that silently drops the charset is caught at
	// this level (writeJSON emits "application/json; charset=utf-8").
	if got, want := resp.Header.Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Errorf("Content-Type: want %q, got %q", want, got)
	}

	var got oapi.LookupResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Query.PostalCode != "11217" || got.Query.Country != "US" {
		t.Errorf("query: %+v", got.Query)
	}
	// resolved_place_label is derived from the dev-fixture ancestry chain
	// for Brooklyn 11217: leaf "Brooklyn" + most-specific local ancestor
	// (Kings County, NY) + most-specific regional ancestor (New York
	// Metro). The em-dash separator is U+2014, matching placeLabel() in
	// pkg/atlas/lookup.go.
	if want := "Brooklyn, Kings County, NY — New York Metro"; got.ResolvedPlaceLabel != want {
		t.Errorf("resolved_place_label: want %q, got %q", want, got.ResolvedPlaceLabel)
	}
	if len(got.Local) == 0 {
		t.Errorf("want at least one local org, got 0")
	}
	if len(got.Regional) == 0 {
		t.Errorf("want at least one regional org, got 0")
	}
	// At least one regional org from the fixtures has multiple regions.
	for _, o := range got.Regional {
		if o.Slug == "tri-state-transportation-campaign" && len(o.Regions) == 0 {
			t.Error("regional org has no regions populated; full set is required by the wire contract")
		}
	}
	// ODbL attribution headers must accompany every /api/v1/** success
	// (CLAUDE.md "Launch strategy — Ongoing — ODbL attribution"). The
	// canonical middleware coverage lives in odbl_test.go; this one-line
	// assertion pins that /lookup specifically participates.
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got := resp.Header.Get("X-Data-Attribution"); got == "" {
		t.Errorf("X-Data-Attribution: want non-empty, got empty")
	}
}

// TestLookup_BadRequests is the table-driven sweep over the handler's
// query-string validation. It supersedes the per-case 400 tests so the
// validation contract lives in one place.
func TestLookup_BadRequests(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name       string
		query      string
		wantDetail string
	}{
		{
			name:       "empty postal_code",
			query:      "postal_code=&country=US",
			wantDetail: "The postal_code query parameter is required.",
		},
		{
			name:       "empty country",
			query:      "postal_code=11217&country=",
			wantDetail: "The country query parameter is required.",
		},
		{
			// postal_code is checked first; both-empty is still a
			// "postal_code is required" 400. Pinning this so a reorder
			// of the validation checks is a visible change.
			name:       "both empty",
			query:      "postal_code=&country=",
			wantDetail: "The postal_code query parameter is required.",
		},
		{
			// Whitespace-only postal codes are trimmed at the boundary
			// (strings.TrimSpace), so they reduce to the empty case.
			name:       "whitespace-only postal_code",
			query:      "postal_code=%20%20&country=US",
			wantDetail: "The postal_code query parameter is required.",
		},
		{
			// Length-cap the raw input before normalization. 17 chars
			// is one over maxPostalCodeLen; chosen ASCII so the byte
			// count matches the rune count.
			name:       "postal_code over length cap",
			query:      "postal_code=" + strings.Repeat("A", maxPostalCodeLen+1) + "&country=US",
			wantDetail: "The postal_code query parameter is longer than the maximum allowed length.",
		},
		{
			// 5 chars is one over maxCountryLen.
			name:       "country over length cap",
			query:      "postal_code=11217&country=" + strings.Repeat("A", maxCountryLen+1),
			wantDetail: "The country query parameter is longer than the maximum allowed length.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/api/v1/lookup?" + tc.query)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status: want 400, got %d", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
				t.Errorf("Content-Type: want application/problem+json prefix, got %q", ct)
			}

			var prob oapi.ProblemDetails
			if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if prob.Type != problemValidation {
				t.Errorf("type: want %q, got %q", problemValidation, prob.Type)
			}
			if prob.Status != int32(http.StatusBadRequest) {
				t.Errorf("status: want 400, got %d", prob.Status)
			}
			if prob.Detail == nil || *prob.Detail != tc.wantDetail {
				t.Errorf("detail: want %q, got %v", tc.wantDetail, prob.Detail)
			}
		})
	}
}

// TestLookup_RequestIDPropagatesIntoProblemBody pins the middleware →
// problem-body integration. A client-supplied X-Request-ID must reach
// the problem document's request_id extension verbatim AND be echoed
// in the response header so operators can correlate logs end-to-end.
func TestLookup_RequestIDPropagatesIntoProblemBody(t *testing.T) {
	srv := newTestServer(t)

	const wantRID = "my-rid-123"
	// Trigger a 400 by omitting postal_code; the path through
	// writeProblem is the same as for any error response.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/lookup?country=US", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("X-Request-ID", wantRID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: want 400, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Request-ID"); got != wantRID {
		t.Errorf("X-Request-ID header: want %q, got %q", wantRID, got)
	}

	var prob oapi.ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if prob.RequestId == nil || *prob.RequestId != wantRID {
		t.Errorf("problem.request_id: want %q, got %v", wantRID, prob.RequestId)
	}
}

func TestLookup_PostalCodeNotFound_ReturnsProblemJSON(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=00000&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type: want application/problem+json, got %q", ct)
	}

	var prob oapi.ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob.Type != problemNotFound {
		t.Errorf("type: want %q, got %q", problemNotFound, prob.Type)
	}
	if prob.Status != int32(http.StatusNotFound) {
		t.Errorf("status: want 404, got %d", prob.Status)
	}
	if prob.Title != "Postal Code Not Found" {
		t.Errorf("title: want %q, got %q", "Postal Code Not Found", prob.Title)
	}
}

func TestLookup_MilitaryZIP_ReturnsTailoredProblem(t *testing.T) {
	srv := newTestServer(t)

	// One ZIP per reserved range: AE (090–098), AA (340), AP (962–966).
	for _, zip := range []string{"09000", "34001", "96201"} {
		t.Run(zip, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=" + zip + "&country=US")
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("status: want 404, got %d", resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type: want application/problem+json, got %q", ct)
			}

			var prob oapi.ProblemDetails
			if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if prob.Type != problemMilitaryZIP {
				t.Errorf("type: want %q, got %q", problemMilitaryZIP, prob.Type)
			}
			if prob.Title != "Military or Diplomatic ZIP Code" {
				t.Errorf("title: want %q, got %q", "Military or Diplomatic ZIP Code", prob.Title)
			}
		})
	}
}

func TestLookup_UnknownCountry_ReturnsNotFound(t *testing.T) {
	// Per the slice #4.6 loader-engineering work, the handler no longer
	// gates on a known-country whitelist (Country is opaque per
	// pkg/atlas/atlas.go). An unknown country with an unknown postal
	// code falls through to atlas.Lookup → ErrPostalCodeNotFound → 404.
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=ZZ")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}

	var prob oapi.ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if prob.Type != problemNotFound {
		t.Errorf("type: want %q, got %q", problemNotFound, prob.Type)
	}
}

// The handler canonicalizes the postal code at the boundary so logs,
// problem-detail responses, and the echoed query in the success payload
// all see the same form. The dev fixtures hold "M5V" (Toronto, CA) under
// the canonical FSA; sending the full lowercase postal "m5v 3a8" must
// resolve and echo back as "M5V" in query.postal_code.
func TestLookup_NormalizesPostalCodeAtBoundary(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=m5v%203a8&country=ca")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 200, got %d (%s)", resp.StatusCode, body)
	}
	var got oapi.LookupResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Query.PostalCode != "M5V" {
		t.Errorf("query.postal_code: want %q (canonical FSA), got %q", "M5V", got.Query.PostalCode)
	}
	if got.Query.Country != "CA" {
		t.Errorf("query.country: want %q, got %q", "CA", got.Query.Country)
	}
}

// TestLookup_NationalTierOrg_ExcludedFromDefaultLookup pins the
// editorial sibling-attachment contract: when a national region is
// NOT an ancestor of the leaf chain (as MUBi sat relative to
// lisboa-municipio in the PT validation fixture), its orgs must
// not leak into either /lookup bucket.
//
// This is the API-level pin. The structural national-tier filter
// itself lives in MemStore.AncestorRegions / DescendantRegions
// (see pkg/atlas/store.go); the per-store contract is exercised
// generically by pkg/atlas/storetest. This test pins the wire
// behavior so a future change to the lookup pipeline (e.g. a
// "popular nearby regions" pass) can't silently start including
// non-ancestor national orgs.
func TestLookup_NationalTierOrg_ExcludedFromDefaultLookup(t *testing.T) {
	s := atlas.NewMemStore()

	// Local ancestor (added before its child so AddRegion can resolve
	// the slug→id edge).
	s.AddRegion(atlas.Region{
		ID: 2, Kind: "us:county", Name: "Kings County, NY", Slug: "kings-county-ny",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeLocal, SortPriority: 30,
	})
	// Leaf city, with the county as its only parent.
	s.AddRegion(atlas.Region{
		ID: 1, Kind: "us:city", Name: "Brooklyn", Slug: "brooklyn-ny",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeLocal, SortPriority: 10,
		ParentSlugs: []string{"kings-county-ny"},
	})
	// National region as a SIBLING — no parent edges to brooklyn or
	// kings-county-ny. The MUBi-style attachment for v1 PT seed data.
	s.AddRegion(atlas.Region{
		ID: 99, Kind: "pt:nacional", Name: "Portugal (national)", Slug: "pt-nacional",
		Country: atlas.Country("PT"), ScopeTier: atlas.ScopeNational, SortPriority: 90,
	})

	// Postal 11217 → brooklyn-ny (leaf).
	s.AddPostalCode(atlas.CountryUS, "11217", 1)

	// Plain local org attached to brooklyn-ny — must surface.
	s.AddOrg(atlas.Org{
		ID: 1, Slug: "brooklyn-spoke", Name: "Brooklyn Spoke",
		ShortDesc:  "Local advocacy.",
		WebsiteURL: "https://brooklynspoke.com",
	}, []int64{1})
	// National-only org — must NOT surface for a US postal lookup, in
	// either local or regional.
	s.AddOrg(atlas.Org{
		ID: 2, Slug: "mubi-nacional", Name: "MUBi Nacional",
		ShortDesc:  "Portuguese national cycling advocacy.",
		WebsiteURL: "https://mubi.pt",
	}, []int64{99})

	handler := New(Config{
		Store:      s,
		Logger:     slog.New(slog.DiscardHandler),
		APIVersion: "v1",
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: want 200, got %d (%s)", resp.StatusCode, body)
	}
	var got oapi.LookupResult
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Brooklyn Spoke is the local org under test.
	var foundLocal bool
	for _, o := range got.Local {
		if o.Slug == "brooklyn-spoke" {
			foundLocal = true
		}
		if o.Slug == "mubi-nacional" {
			t.Errorf("national-only org leaked into local bucket: %s", o.Slug)
		}
	}
	if !foundLocal {
		t.Errorf("brooklyn-spoke missing from local bucket; got %v", oapiLookupOrgSlugs(got.Local))
	}
	for _, o := range got.Regional {
		if o.Slug == "mubi-nacional" {
			t.Errorf("national-only org leaked into regional bucket: %s", o.Slug)
		}
	}
}

func oapiLookupOrgSlugs(orgs []oapi.LookupOrg) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}
