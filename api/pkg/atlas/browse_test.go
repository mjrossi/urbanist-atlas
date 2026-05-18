package atlas

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// newBrowseFixture builds a small, deterministic MemStore for browse
// tests. It deliberately mixes:
//   - two metros (us:metro, ca:cma) so ordering can be observed,
//   - a non-metro region (us:state) so we can confirm it doesn't leak
//     into ListMetros,
//   - an org tagged only to a descendant of a metro (NYC city), so the
//     metro org count must include that org via the downward DAG walk,
//   - a national-tier region + org so ListRecent's national filter can
//     be exercised.
//
// Timestamps are explicit so newest-first ordering is testable.
func newBrowseFixture() *MemStore {
	s := NewMemStore()
	// NYC chain: brooklyn -> kings-county -> nyc-metro -> ny
	s.AddRegion(Region{ID: 4, Kind: "us:state", Name: "NY", Slug: "ny", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 70})
	s.AddRegion(Region{ID: 3, Kind: "us:metro", Name: "New York Metro", Slug: "nyc-metro", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 50, ParentSlugs: []string{"ny"}})
	s.AddRegion(Region{ID: 2, Kind: "us:county", Name: "Kings County, NY", Slug: "kings-county-ny", Country: CountryUS, ScopeTier: ScopeLocal, SortPriority: 30, ParentSlugs: []string{"nyc-metro", "ny"}})
	s.AddRegion(Region{ID: 1, Kind: "us:city", Name: "Brooklyn", Slug: "brooklyn-ny", Country: CountryUS, ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"kings-county-ny"}})

	// Toronto chain: toronto -> toronto-cma -> ontario
	s.AddRegion(Region{ID: 22, Kind: "ca:province", Name: "Ontario", Slug: "ontario", Country: CountryCA, ScopeTier: ScopeRegional, SortPriority: 70})
	s.AddRegion(Region{ID: 21, Kind: "ca:cma", Name: "Toronto CMA", Slug: "toronto-cma", Country: CountryCA, ScopeTier: ScopeRegional, SortPriority: 50, ParentSlugs: []string{"ontario"}})
	s.AddRegion(Region{ID: 20, Kind: "ca:city", Name: "Toronto", Slug: "toronto-on", Country: CountryCA, ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"toronto-cma"}})

	// National-tier region (modeled after PT's pt-nacional from slice #4.6).
	s.AddRegion(Region{ID: 99, Kind: "pt:nacional", Name: "Portugal (national)", Slug: "pt-nacional", Country: Country("PT"), ScopeTier: ScopeNational, SortPriority: 100})

	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	// Newest at offset 10 days. Orgs are added in arbitrary order so the
	// store's sort actually has to work.
	s.AddOrg(Org{ID: 1, Slug: "ny-state-org", Name: "NY State Org", ShortDesc: "x", WebsiteURL: "https://x", Tags: []Tag{"transit"}, CreatedAt: t0.AddDate(0, 0, 3)}, []int64{4})
	s.AddOrg(Org{ID: 2, Slug: "transalt-brooklyn", Name: "TransAlt Brooklyn", ShortDesc: "x", WebsiteURL: "https://x", Tags: []Tag{"safe-streets"}, CreatedAt: t0.AddDate(0, 0, 5)}, []int64{1})
	s.AddOrg(Org{ID: 3, Slug: "riders-alliance", Name: "Riders Alliance", ShortDesc: "x", WebsiteURL: "https://x", Tags: []Tag{"transit"}, CreatedAt: t0.AddDate(0, 0, 7)}, []int64{3})
	s.AddOrg(Org{ID: 4, Slug: "ttcriders", Name: "TTCriders", ShortDesc: "x", WebsiteURL: "https://x", Tags: []Tag{"transit"}, CreatedAt: t0.AddDate(0, 0, 9)}, []int64{20})
	// MUBi-like: ONLY a national-tier region attachment. Must NOT appear
	// in ListRecent. We give it the most-recent timestamp so a forgotten
	// filter would surface it at the top.
	s.AddOrg(Org{ID: 5, Slug: "mubi-nacional", Name: "MUBi Nacional", ShortDesc: "x", WebsiteURL: "https://x", Tags: []Tag{"national"}, CreatedAt: t0.AddDate(0, 0, 10)}, []int64{99})

	return s
}

func TestMemStore_ListMetros_OrderedByOrgCountThenName(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListMetros(context.Background())
	if err != nil {
		t.Fatalf("ListMetros: %v", err)
	}
	// Expect 2 entries: nyc-metro (count=2 — riders-alliance directly +
	// transalt-brooklyn via Brooklyn descendant) and toronto-cma (count=1).
	// Order: nyc-metro (2), toronto-cma (1).
	if len(got) != 2 {
		t.Fatalf("len: want 2, got %d (%+v)", len(got), got)
	}
	if got[0].Region.Slug != "nyc-metro" || got[0].OrgCount != 2 {
		t.Errorf("[0]: want nyc-metro count=2, got slug=%s count=%d", got[0].Region.Slug, got[0].OrgCount)
	}
	if got[1].Region.Slug != "toronto-cma" || got[1].OrgCount != 1 {
		t.Errorf("[1]: want toronto-cma count=1, got slug=%s count=%d", got[1].Region.Slug, got[1].OrgCount)
	}
}

func TestMemStore_ListMetros_ExcludesNonMetroKindsAndEmptyMetros(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListMetros(context.Background())
	if err != nil {
		t.Fatalf("ListMetros: %v", err)
	}
	for _, m := range got {
		if !IsMetroKind(m.Region.Kind) {
			t.Errorf("non-metro kind leaked into result: %q (%s)", m.Region.Kind, m.Region.Slug)
		}
		if m.OrgCount == 0 {
			t.Errorf("zero-org metro leaked into result: %s", m.Region.Slug)
		}
		if m.Region.ScopeTier == ScopeNational {
			t.Errorf("national-tier region leaked into result: %s", m.Region.Slug)
		}
	}
}

func TestMemStore_ListMetros_OrgCount_AlphabeticalTiebreak(t *testing.T) {
	// Two metros with the same org count: order should be alphabetical
	// by name.
	s := NewMemStore()
	s.AddRegion(Region{ID: 1, Kind: "us:metro", Name: "Atlanta", Slug: "atlanta", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 50})
	s.AddRegion(Region{ID: 2, Kind: "us:metro", Name: "Boston", Slug: "boston", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 50})
	s.AddOrg(Org{ID: 1, Slug: "a", Name: "A"}, []int64{1})
	s.AddOrg(Org{ID: 2, Slug: "b", Name: "B"}, []int64{2})

	got, err := s.ListMetros(context.Background())
	if err != nil {
		t.Fatalf("ListMetros: %v", err)
	}
	gotNames := make([]string, len(got))
	for i, m := range got {
		gotNames[i] = m.Region.Name
	}
	want := []string{"Atlanta", "Boston"}
	if diff := cmp.Diff(want, gotNames); diff != "" {
		t.Errorf("alphabetical tiebreak (-want +got):\n%s", diff)
	}
}

func TestMemStore_GetMetro_HappyPath(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetMetro(context.Background(), "nyc-metro")
	if err != nil {
		t.Fatalf("GetMetro: %v", err)
	}
	if got == nil {
		t.Fatal("nil result for known metro slug")
	}
	if got.Region.Slug != "nyc-metro" {
		t.Errorf("region slug: want nyc-metro, got %s", got.Region.Slug)
	}
	gotSlugs := make([]string, 0, len(got.Orgs))
	for _, o := range got.Orgs {
		gotSlugs = append(gotSlugs, o.Slug)
	}
	sort.Strings(gotSlugs)
	want := []string{"riders-alliance", "transalt-brooklyn"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("orgs (-want +got):\n%s", diff)
	}
}

func TestMemStore_GetMetro_OrderNewestFirst(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetMetro(context.Background(), "nyc-metro")
	if err != nil {
		t.Fatalf("GetMetro: %v", err)
	}
	if got == nil {
		t.Fatal("nil result")
	}
	// riders-alliance was created on day 7, transalt-brooklyn on day 5.
	// Newest-first must put riders-alliance ahead of transalt-brooklyn.
	if len(got.Orgs) < 2 {
		t.Fatalf("want >= 2 orgs, got %d", len(got.Orgs))
	}
	if got.Orgs[0].Slug != "riders-alliance" {
		t.Errorf("[0] want riders-alliance (newest), got %s", got.Orgs[0].Slug)
	}
	if got.Orgs[1].Slug != "transalt-brooklyn" {
		t.Errorf("[1] want transalt-brooklyn, got %s", got.Orgs[1].Slug)
	}
}

func TestMemStore_GetMetro_NotFound(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetMetro(context.Background(), "does-not-exist")
	if err != nil {
		t.Errorf("err: want nil, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil, got %+v", got)
	}
}

func TestMemStore_GetMetro_NonMetroSlug_ReturnsNil(t *testing.T) {
	s := newBrowseFixture()
	// "ny" is a us:state — exists as a region but is NOT a metro kind.
	got, err := s.GetMetro(context.Background(), "ny")
	if err != nil {
		t.Errorf("err: want nil, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil for non-metro slug, got %+v", got)
	}
}

func TestMemStore_ListRecent_NewestFirstCappedAtTen(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListRecent(context.Background())
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	// Fixture has 5 orgs total; MUBi is national-only and must be
	// excluded → 4 orgs expected.
	if len(got) != 4 {
		t.Fatalf("len: want 4 (5 orgs minus national-only), got %d (%v)", len(got), orgSlugSlice(got))
	}
	if len(got) > 10 {
		t.Errorf("cap: want <= 10, got %d", len(got))
	}
	// Newest-first: ttcriders (day 9), riders-alliance (day 7),
	// transalt-brooklyn (day 5), ny-state-org (day 3).
	want := []string{"ttcriders", "riders-alliance", "transalt-brooklyn", "ny-state-org"}
	if diff := cmp.Diff(want, orgSlugSlice(got)); diff != "" {
		t.Errorf("order (-want +got):\n%s", diff)
	}
}

func TestMemStore_ListRecent_ExcludesNationalOnlyOrgs(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListRecent(context.Background())
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	for _, o := range got {
		if o.Slug == "mubi-nacional" {
			t.Errorf("national-only org leaked into ListRecent: %+v", o)
		}
	}
}

// TestMemStore_ListRecent_CapAtTen seeds 12 plain non-national orgs and
// asserts the cap.
func TestMemStore_ListRecent_CapAtTen(t *testing.T) {
	s := NewMemStore()
	s.AddRegion(Region{ID: 1, Kind: "us:city", Name: "Brooklyn", Slug: "brooklyn", Country: CountryUS, ScopeTier: ScopeLocal})
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		s.AddOrg(Org{
			ID:        int64(i),
			Slug:      "org-" + string(rune('a'+i-1)),
			Name:      "Org",
			CreatedAt: base.Add(time.Duration(i) * time.Hour),
		}, []int64{1})
	}
	got, err := s.ListRecent(context.Background())
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 10 {
		t.Errorf("len: want 10, got %d", len(got))
	}
	if got[0].ID != 12 {
		t.Errorf("[0]: want id=12 (newest), got %d", got[0].ID)
	}
}

func orgSlugSlice(orgs []Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}
