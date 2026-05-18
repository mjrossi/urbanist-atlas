package atlas

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func nycFixture(t *testing.T) *MemStore {
	t.Helper()
	s := NewMemStore()
	addRegions(s,
		Region{ID: 1, Slug: "nyc-tristate", Kind: "us:multi-state", Name: "Tri-State Region", Country: "US", ScopeTier: ScopeRegional, SortPriority: 80},
		Region{ID: 2, Slug: "ny", Kind: "us:state", Name: "New York", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60},
		Region{ID: 3, Slug: "nj", Kind: "us:state", Name: "New Jersey", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60},
		Region{ID: 4, Slug: "nyc-metro", Kind: "us:metro", Name: "New York Metro", Country: "US", ScopeTier: ScopeRegional, SortPriority: 40, ParentSlugs: []string{"nyc-tristate"}},
		Region{ID: 5, Slug: "nyc", Kind: "us:city", Name: "New York City", Country: "US", ScopeTier: ScopeLocal, SortPriority: 15, ParentSlugs: []string{"nyc-metro", "ny"}},
		Region{ID: 6, Slug: "brooklyn", Kind: "us:borough", Name: "Brooklyn", Country: "US", ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"nyc"}},
		Region{ID: 7, Slug: "hoboken", Kind: "us:city", Name: "Hoboken", Country: "US", ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"nyc-metro", "nj"}},
	)
	s.AddPostalCode("US", "11217", 6)
	s.AddPostalCode("US", "07302", 7)

	s.AddOrg(Org{ID: 100, Slug: "brooklyn-spoke", Name: "Brooklyn Spoke", ShortDesc: "Park Slope cycling.", WebsiteURL: "https://example.org/bks"}, []int64{6})
	s.AddOrg(Org{ID: 101, Slug: "transalt", Name: "Transportation Alternatives", ShortDesc: "NYC streets.", WebsiteURL: "https://transalt.org"}, []int64{5})
	s.AddOrg(Org{ID: 102, Slug: "transitcenter", Name: "TransitCenter", ShortDesc: "NYC metro foundation.", WebsiteURL: "https://transitcenter.org"}, []int64{4})
	s.AddOrg(Org{ID: 103, Slug: "ny-lcv", Name: "NY LCV Transportation", ShortDesc: "State-wide.", WebsiteURL: "https://example.org/nylcv"}, []int64{2})
	s.AddOrg(Org{ID: 104, Slug: "tri-state", Name: "Tri-State Transportation Campaign", ShortDesc: "Tri-state policy.", WebsiteURL: "https://tstc.org"}, []int64{1})
	return s
}

func addRegions(s *MemStore, rs ...Region) {
	for _, r := range rs {
		s.AddRegion(r)
	}
}

func slugs(orgs []Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}

func TestLookup_NYC_Brooklyn(t *testing.T) {
	got, err := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "11217", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	wantLocal := []string{"brooklyn-spoke", "transalt"}
	wantRegional := []string{"transitcenter", "ny-lcv", "tri-state"}
	if diff := cmp.Diff(wantLocal, slugs(got.Local)); diff != "" {
		t.Errorf("Local (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantRegional, slugs(got.Regional)); diff != "" {
		t.Errorf("Regional (-want +got):\n%s", diff)
	}
}

func TestLookup_NYC_Hoboken_NoCrossStateLocalLeak(t *testing.T) {
	got, err := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "07302", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	for _, o := range got.Local {
		if o.Slug == "transalt" || o.Slug == "brooklyn-spoke" {
			t.Errorf("Local leak: %q should not appear for Hoboken", o.Slug)
		}
	}
	regionalSlugs := slugs(got.Regional)
	mustContain(t, regionalSlugs, "transitcenter")
	mustContain(t, regionalSlugs, "tri-state")
	mustNotContain(t, regionalSlugs, "ny-lcv")
}

func TestLookup_NotFound(t *testing.T) {
	_, err := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "00000", Country: "US"})
	if err != ErrPostalCodeNotFound {
		t.Errorf("err = %v, want ErrPostalCodeNotFound", err)
	}
}

func TestLookup_MatchedRegionSlugs(t *testing.T) {
	got, err := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "11217", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	for _, o := range append(got.Local, got.Regional...) {
		if len(o.MatchedRegionSlugs) == 0 {
			t.Errorf("org %q has empty MatchedRegionSlugs", o.Slug)
		}
	}
}

func TestLookup_ResolvedAncestry(t *testing.T) {
	got, err := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "11217", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	wantOrder := []string{"brooklyn", "nyc", "nyc-metro", "ny", "nyc-tristate"}
	gotOrder := make([]string, len(got.ResolvedAncestry))
	for i, r := range got.ResolvedAncestry {
		gotOrder[i] = r.Slug
	}
	if diff := cmp.Diff(wantOrder, gotOrder); diff != "" {
		t.Errorf("ResolvedAncestry (-want +got):\n%s", diff)
	}
}

func TestLookup_PlaceLabel_NYC(t *testing.T) {
	got, _ := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "11217", Country: "US"})
	want := "Brooklyn, New York City — New York Metro"
	if got.ResolvedPlaceLabel != want {
		t.Errorf("ResolvedPlaceLabel = %q, want %q", got.ResolvedPlaceLabel, want)
	}
}

func mustContain(t *testing.T, ss []string, want string) {
	t.Helper()
	for _, s := range ss {
		if s == want {
			return
		}
	}
	t.Errorf("missing %q in %v", want, ss)
}

func mustNotContain(t *testing.T, ss []string, bad string) {
	t.Helper()
	for _, s := range ss {
		if s == bad {
			t.Errorf("unexpected %q in %v", bad, ss)
			return
		}
	}
}

func TestLookup_OrgWithNoMatchedRegions(t *testing.T) {
	s := nycFixture(t)
	s.AddRegion(Region{ID: 999, Slug: "wyoming", Kind: "us:state", Name: "Wyoming", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60})
	s.AddOrg(Org{ID: 999, Slug: "wyoming-streets", Name: "Wyoming Streets", ShortDesc: "x", WebsiteURL: "https://example.org/wy"}, []int64{999})
	got, _ := Lookup(context.Background(), s, LookupQuery{PostalCode: "11217", Country: "US"})
	all := append([]Org{}, got.Local...)
	all = append(all, got.Regional...)
	for _, o := range all {
		if strings.Contains(o.Slug, "wyoming") {
			t.Errorf("wyoming-streets surfaced for 11217: %v", o.Slug)
		}
	}
}
