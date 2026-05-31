package atlas

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func nycFixture(t *testing.T) *MemStore {
	t.Helper()
	s := NewMemStore()
	// Post-#7.5.2 shape: `nyc` is a regional intermediate region (only
	// parent is `nyc-metro`); the borough leaf carries the state edge
	// (`ny`) directly. Citywide orgs (TransAlt) attach to `nyc` and
	// surface as Regional results for borough lookups.
	addRegions(s,
		Region{ID: 1, Slug: "nyc-tristate", Kind: "us:multi-state", Name: "Tri-State Region", Country: "US", ScopeTier: ScopeRegional, SortPriority: 80},
		Region{ID: 2, Slug: "ny", Kind: "us:state", Name: "New York", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60},
		Region{ID: 3, Slug: "nj", Kind: "us:state", Name: "New Jersey", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60},
		Region{ID: 4, Slug: "nyc-metro", Kind: "us:metro", Name: "New York Metro", Country: "US", ScopeTier: ScopeRegional, SortPriority: 40, ParentSlugs: []string{"nyc-tristate"}},
		Region{ID: 5, Slug: "nyc", Kind: "us:city", Name: "New York City", Country: "US", ScopeTier: ScopeRegional, SortPriority: 15, ParentSlugs: []string{"nyc-metro"}},
		Region{ID: 6, Slug: "brooklyn", Kind: "us:borough", Name: "Brooklyn", Country: "US", ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"nyc", "ny"}},
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
	// After the #7.5.2 borough split, citywide NYC orgs (TransAlt)
	// attach to the regional `nyc` node and bucket as Regional.
	// Borough-only orgs (Brooklyn Spoke) stay in Local. State-attached
	// orgs (NY LCV → `ny`, a us:state) bucket as Statewide. The
	// nyc-tristate coalition (us:multi-state) is NOT state-equivalent,
	// so tri-state stays Regional. Each bucket is ordered by
	// best-matched sort_priority asc: nyc(15) → nyc-metro(40) for
	// Regional; nyc-tristate(80) sorts last in Regional.
	wantLocal := []string{"brooklyn-spoke"}
	wantRegional := []string{"transalt", "transitcenter", "tri-state"}
	wantStatewide := []string{"ny-lcv"}
	if diff := cmp.Diff(wantLocal, slugs(got.Local)); diff != "" {
		t.Errorf("Local (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantRegional, slugs(got.Regional)); diff != "" {
		t.Errorf("Regional (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantStatewide, slugs(got.Statewide)); diff != "" {
		t.Errorf("Statewide (-want +got):\n%s", diff)
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
	// Lookup wraps ResolveLeafRegion's error with context; the sentinel
	// is reachable via errors.Is. The HTTP handler does the same check.
	if !errors.Is(err, ErrPostalCodeNotFound) {
		t.Errorf("err = %v, want errors.Is(err, ErrPostalCodeNotFound)", err)
	}
}

func TestLookup_MatchedRegionSlugs(t *testing.T) {
	got, err := Lookup(context.Background(), nycFixture(t), LookupQuery{PostalCode: "11217", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	all := append([]Org{}, got.Local...)
	all = append(all, got.Regional...)
	all = append(all, got.Statewide...)
	for _, o := range all {
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
	// BFS depth order: brooklyn (0) → {nyc, ny} (1) → nyc-metro (2,
	// from nyc) → nyc-tristate (3, from nyc-metro). Within depth 1 the
	// tiebreak is sort_priority asc: nyc(15) before ny(60). ny has no
	// further parents and stops at depth 1.
	wantOrder := []string{"brooklyn", "nyc", "ny", "nyc-metro", "nyc-tristate"}
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
	all = append(all, got.Statewide...)
	for _, o := range all {
		if strings.Contains(o.Slug, "wyoming") {
			t.Errorf("wyoming-streets surfaced for 11217: %v", o.Slug)
		}
	}
}

// TestLookup_Michigan_StateVsMetro is the regression for the reported
// behavior: a metro ZIP (Detroit) must split metro-attached orgs into
// Regional and state-attached orgs into Statewide, and a non-metro ZIP
// that anchors directly on the state must surface only Statewide.
func TestLookup_Michigan_StateVsMetro(t *testing.T) {
	s := NewMemStore()
	addRegions(s,
		Region{ID: 1, Slug: "mi", Kind: "us:state", Name: "Michigan", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60},
		Region{ID: 2, Slug: "detroit-mi-metro", Kind: "us:metro", Name: "Detroit Metro", Country: "US", ScopeTier: ScopeRegional, SortPriority: 40, ParentSlugs: []string{"mi"}},
	)
	// 48125 anchors on the metro; 48624 anchors directly on the state.
	s.AddPostalCode("US", "48125", 2)
	s.AddPostalCode("US", "48624", 1)
	s.AddOrg(Org{ID: 10, Slug: "detroit-greenways", Name: "Detroit Greenways Coalition", ShortDesc: "Metro trails.", WebsiteURL: "https://example.org/dgc"}, []int64{2})
	s.AddOrg(Org{ID: 11, Slug: "league-of-michigan-bicyclists", Name: "League of Michigan Bicyclists", ShortDesc: "Statewide cycling.", WebsiteURL: "https://example.org/lmb"}, []int64{1})

	metro, err := Lookup(context.Background(), s, LookupQuery{PostalCode: "48125", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup 48125: %v", err)
	}
	if diff := cmp.Diff([]string{"detroit-greenways"}, slugs(metro.Regional)); diff != "" {
		t.Errorf("48125 Regional (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"league-of-michigan-bicyclists"}, slugs(metro.Statewide)); diff != "" {
		t.Errorf("48125 Statewide (-want +got):\n%s", diff)
	}

	state, err := Lookup(context.Background(), s, LookupQuery{PostalCode: "48624", Country: "US"})
	if err != nil {
		t.Fatalf("Lookup 48624: %v", err)
	}
	if len(state.Regional) != 0 {
		t.Errorf("48624 Regional = %v, want empty", slugs(state.Regional))
	}
	if diff := cmp.Diff([]string{"league-of-michigan-bicyclists"}, slugs(state.Statewide)); diff != "" {
		t.Errorf("48624 Statewide (-want +got):\n%s", diff)
	}
}
