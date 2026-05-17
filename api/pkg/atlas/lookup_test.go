package atlas

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// fixture builds a small, deterministic MemStore for lookup tests:
//
//   - Brooklyn (city, local) ─┐
//   - Kings County (county, local)
//   - NYC Metro (metro, regional)
//   - NY (state, regional)
//
// Orgs:
//   - TransAlt Brooklyn  → regions: Brooklyn, Kings  (local match expected)
//   - Brooklyn Spoke     → regions: Brooklyn        (local match expected)
//   - Riders Alliance    → regions: NYC Metro       (regional)
//   - StreetsPAC         → regions: NYC Metro       (regional)
//   - Tri-State          → regions: NY              (regional)
//   - Off-Topic SF       → regions: SF Bay Area     (no match for 11217)
//
// Postal code 11217 maps to city=Brooklyn, county=Kings, metro=NYC Metro, state=NY.
func fixture(t *testing.T) *MemStore {
	t.Helper()
	s := NewMemStore()

	s.AddRegion(Region{ID: 1, Kind: RegionCity, Name: "Brooklyn", Slug: "brooklyn", Country: CountryUS, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 2, Kind: RegionCounty, Name: "Kings County", Slug: "kings", Country: CountryUS, ScopeTier: ScopeLocal})
	s.AddRegion(Region{ID: 3, Kind: RegionMetro, Name: "NYC Metro", Slug: "nyc-metro", Country: CountryUS, ScopeTier: ScopeRegional})
	s.AddRegion(Region{ID: 4, Kind: RegionState, Name: "NY", Slug: "ny", Country: CountryUS, ScopeTier: ScopeRegional})
	s.AddRegion(Region{ID: 99, Kind: RegionMetro, Name: "SF Bay Area", Slug: "sf-bay", Country: CountryUS, ScopeTier: ScopeRegional})

	bk := s.regions[1]
	kings := s.regions[2]
	metro := s.regions[3]
	ny := s.regions[4]
	s.AddPostalCode(ResolvedPostalCode{Code: "11217", Country: CountryUS, City: &bk, County: &kings, Metro: &metro, State: &ny})

	s.AddOrg(Org{ID: 1, Slug: "transalt-bk", Name: "TransAlt Brooklyn"}, []int64{1, 2})
	s.AddOrg(Org{ID: 2, Slug: "bk-spoke", Name: "Brooklyn Spoke"}, []int64{1})
	s.AddOrg(Org{ID: 3, Slug: "riders", Name: "Riders Alliance"}, []int64{3})
	s.AddOrg(Org{ID: 4, Slug: "streetspac", Name: "StreetsPAC"}, []int64{3})
	s.AddOrg(Org{ID: 5, Slug: "tri-state", Name: "Tri-State Transportation Campaign"}, []int64{4})
	s.AddOrg(Org{ID: 6, Slug: "off-topic-sf", Name: "Off-Topic SF"}, []int64{99})

	return s
}

func TestLookup_BucketsAndOrders(t *testing.T) {
	s := fixture(t)

	got, err := Lookup(context.Background(), s, LookupQuery{PostalCode: "11217", Country: CountryUS})
	if err != nil {
		t.Fatalf("Lookup: unexpected error: %v", err)
	}

	// Local: TransAlt (city+county) sorts before Brooklyn Spoke (city
	// only) only if their kinds differ. Both have a city region, so the
	// tiebreaker is alphabetical Name — "Brooklyn Spoke" < "TransAlt
	// Brooklyn". Verifies the sort tiebreaker is wired.
	wantLocalNames := []string{"Brooklyn Spoke", "TransAlt Brooklyn"}
	gotLocalNames := names(got.Local)
	if diff := cmp.Diff(wantLocalNames, gotLocalNames); diff != "" {
		t.Errorf("local bucket names mismatch (-want +got):\n%s", diff)
	}

	// Regional: Riders Alliance & StreetsPAC are metro (rank 2);
	// Tri-State is state (rank 3). Metro before state, alphabetical
	// within metro.
	wantRegionalNames := []string{"Riders Alliance", "StreetsPAC", "Tri-State Transportation Campaign"}
	gotRegionalNames := names(got.Regional)
	if diff := cmp.Diff(wantRegionalNames, gotRegionalNames); diff != "" {
		t.Errorf("regional bucket names mismatch (-want +got):\n%s", diff)
	}

	// Off-topic SF org must not appear in either bucket.
	for _, o := range append(got.Local, got.Regional...) {
		if o.Slug == "off-topic-sf" {
			t.Errorf("off-topic org leaked into results: %+v", o)
		}
	}

	if got.ResolvedPlaceLabel != "Brooklyn, NY — NYC Metro" {
		t.Errorf("place label: want %q, got %q", "Brooklyn, NY — NYC Metro", got.ResolvedPlaceLabel)
	}
}

func TestLookup_PostalCodeNotFound(t *testing.T) {
	s := fixture(t)

	_, err := Lookup(context.Background(), s, LookupQuery{PostalCode: "00000", Country: CountryUS})
	if !errors.Is(err, ErrPostalCodeNotFound) {
		t.Fatalf("want ErrPostalCodeNotFound, got %v", err)
	}
}

func TestLookup_EmptyResultSet(t *testing.T) {
	// A postal code that resolves to regions no org serves.
	s := NewMemStore()
	s.AddRegion(Region{ID: 50, Kind: RegionCity, Name: "Nowhere", Country: CountryUS, ScopeTier: ScopeLocal})
	nowhere := Region{ID: 50, Kind: RegionCity, Name: "Nowhere", Country: CountryUS, ScopeTier: ScopeLocal}
	s.AddPostalCode(ResolvedPostalCode{Code: "00001", Country: CountryUS, City: &nowhere})

	got, err := Lookup(context.Background(), s, LookupQuery{PostalCode: "00001", Country: CountryUS})
	if err != nil {
		t.Fatalf("Lookup: unexpected error: %v", err)
	}
	if len(got.Local) != 0 || len(got.Regional) != 0 {
		t.Errorf("want empty buckets, got local=%d regional=%d", len(got.Local), len(got.Regional))
	}
}

func TestMemStore_CanadianPostalCodeNormalization(t *testing.T) {
	s := NewMemStore()
	tor := Region{ID: 20, Kind: RegionCity, Name: "Toronto", Country: CountryCA, ScopeTier: ScopeLocal}
	s.AddRegion(tor)
	s.AddPostalCode(ResolvedPostalCode{Code: "M5V 3A8", Country: CountryCA, City: &tor})

	// User inputs the full postal code with a space; we should match.
	rpc, err := s.ResolvePostalCode(context.Background(), CountryCA, "m5v 3a8")
	if err != nil {
		t.Fatalf("ResolvePostalCode: %v", err)
	}
	if rpc.Code != "M5V" {
		t.Errorf("want normalized code %q, got %q", "M5V", rpc.Code)
	}
}

func names(orgs []Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Name
	}
	return out
}
