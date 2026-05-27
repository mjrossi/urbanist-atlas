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
//   - two metro-equivalent regions (us:metro, ca:cma) so ordering can be observed,
//   - two city-kind regions (us:city, ca:city) so the default browse
//     set is exercised — both surface as their own RegionSummary entries,
//   - a us:state and a us:county so the broadened detail-endpoint
//     behavior (any non-national slug resolves) is testable,
//   - an org tagged only to a descendant of a metro (Brooklyn, a us:city
//     under nyc-metro), so the metro's org count must include that org
//     via the downward DAG walk while the city itself also shows up
//     with its own direct count,
//   - a national-tier region + org so ListRecent's national filter
//     and GetRegion's national gate can be exercised.
//
// Timestamps are explicit so newest-first ordering is testable.
func newBrowseFixture() *MemStore {
	s := NewMemStore()
	// NYC chain: brooklyn-ny -> kings-county-ny -> nyc-metro -> ny
	s.AddRegion(Region{ID: 4, Kind: "us:state", Name: "NY", Slug: "ny", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 70})
	s.AddRegion(Region{ID: 3, Kind: "us:metro", Name: "New York Metro", Slug: "nyc-metro", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 50, ParentSlugs: []string{"ny"}})
	s.AddRegion(Region{ID: 2, Kind: "us:county", Name: "Kings County, NY", Slug: "kings-county-ny", Country: CountryUS, ScopeTier: ScopeLocal, SortPriority: 30, ParentSlugs: []string{"nyc-metro", "ny"}})
	s.AddRegion(Region{ID: 1, Kind: "us:city", Name: "Brooklyn", Slug: "brooklyn-ny", Country: CountryUS, ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"kings-county-ny"}})

	// Toronto chain: toronto-on -> toronto-cma -> ontario
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
	// in ListRecent or GetRegion. We give it the most-recent timestamp so
	// a forgotten filter would surface it at the top.
	s.AddOrg(Org{ID: 5, Slug: "mubi-nacional", Name: "MUBi Nacional", ShortDesc: "x", WebsiteURL: "https://x", Tags: []Tag{"national"}, CreatedAt: t0.AddDate(0, 0, 10)}, []int64{99})

	return s
}

func TestMemStore_ListRegions_OrderedByOrgCountThenName(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	// Expect 4 entries against the default browse set:
	//   - nyc-metro       (us:metro, count=2 via Brooklyn descendant + direct)
	//   - brooklyn-ny     (us:city, count=1 — transalt-brooklyn direct)
	//   - toronto-on      (ca:city, count=1 — ttcriders direct)
	//   - toronto-cma     (ca:cma, count=1 via Toronto descendant)
	// Order: count DESC, then name ASC.
	if len(got) != 4 {
		t.Fatalf("len: want 4, got %d (%+v)", len(got), got)
	}
	if got[0].Region.Slug != "nyc-metro" || got[0].OrgCount != 2 {
		t.Errorf("[0]: want nyc-metro count=2, got slug=%s count=%d", got[0].Region.Slug, got[0].OrgCount)
	}
	// Tiebreak among 1-count entries is alphabetical by Name:
	// "Brooklyn" < "Toronto" < "Toronto CMA".
	if got[1].Region.Slug != "brooklyn-ny" || got[1].OrgCount != 1 {
		t.Errorf("[1]: want brooklyn-ny count=1, got slug=%s count=%d", got[1].Region.Slug, got[1].OrgCount)
	}
	if got[2].Region.Slug != "toronto-on" || got[2].OrgCount != 1 {
		t.Errorf("[2]: want toronto-on count=1, got slug=%s count=%d", got[2].Region.Slug, got[2].OrgCount)
	}
	if got[3].Region.Slug != "toronto-cma" || got[3].OrgCount != 1 {
		t.Errorf("[3]: want toronto-cma count=1, got slug=%s count=%d", got[3].Region.Slug, got[3].OrgCount)
	}
}

func TestMemStore_ListRegions_ExcludesOutsideKindsAndNational(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	for _, p := range got {
		if !IsDefaultBrowseKind(p.Region.Kind) {
			t.Errorf("kind outside the default browse set leaked into result: %q (%s)", p.Region.Kind, p.Region.Slug)
		}
		if p.OrgCount == 0 {
			t.Errorf("zero-org region leaked into result: %s", p.Region.Slug)
		}
		if p.Region.ScopeTier == ScopeNational {
			t.Errorf("national-tier region leaked into result: %s", p.Region.Slug)
		}
	}
}

func TestMemStore_ListRegions_AlphabeticalTiebreak(t *testing.T) {
	// Two metros with the same org count: order should be alphabetical
	// by name.
	s := NewMemStore()
	s.AddRegion(Region{ID: 1, Kind: "us:metro", Name: "Atlanta", Slug: "atlanta", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 50})
	s.AddRegion(Region{ID: 2, Kind: "us:metro", Name: "Boston", Slug: "boston", Country: CountryUS, ScopeTier: ScopeRegional, SortPriority: 50})
	s.AddOrg(Org{ID: 1, Slug: "a", Name: "A"}, []int64{1})
	s.AddOrg(Org{ID: 2, Slug: "b", Name: "B"}, []int64{2})

	got, err := s.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	gotNames := make([]string, len(got))
	for i, p := range got {
		gotNames[i] = p.Region.Name
	}
	want := []string{"Atlanta", "Boston"}
	if diff := cmp.Diff(want, gotNames); diff != "" {
		t.Errorf("alphabetical tiebreak (-want +got):\n%s", diff)
	}
}

// TestMemStore_ListRegions_CitiesAppearAsTheirOwnEntries pins the
// behavioral change introduced when the endpoint broadened beyond
// metro-only: a us:city / ca:city region with direct org
// attachments shows up as its own row in the default Browse list,
// alongside (not under) its parent metro.
func TestMemStore_ListRegions_CitiesAppearAsTheirOwnEntries(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	cityKinds := map[RegionKind]bool{"us:city": true, "ca:city": true}
	var cities []string
	for _, p := range got {
		if cityKinds[p.Region.Kind] {
			cities = append(cities, p.Region.Slug)
		}
	}
	sort.Strings(cities)
	want := []string{"brooklyn-ny", "toronto-on"}
	if diff := cmp.Diff(want, cities); diff != "" {
		t.Errorf("city entries on Browse (-want +got):\n%s", diff)
	}
}

// TestMemStore_ListRegions_BrowseParentSlug pins the SPA grouping
// hook: cities carry the slug of their nearest browseable-kind
// ancestor; metros without a browseable ancestor carry "". The
// walk traverses non-browseable intermediates (county, state) so a
// city whose direct parent isn't in the default set still
// resolves through.
func TestMemStore_ListRegions_BrowseParentSlug(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	bySlug := map[string]string{}
	for _, p := range got {
		bySlug[p.Region.Slug] = p.BrowseParentSlug
	}
	cases := []struct {
		slug, wantParent string
	}{
		// Brooklyn walks up: kings-county (not browseable) -> nyc-metro
		// (browseable, us:metro). Skipped intermediate proves the walk
		// passes through non-browseable nodes.
		{"brooklyn-ny", "nyc-metro"},
		// Toronto's direct parent IS browseable (toronto-cma).
		{"toronto-on", "toronto-cma"},
		// Metros have no browseable ancestor in the fixture
		// (their parents are us:state / ca:province / us:multi-state).
		{"nyc-metro", ""},
		{"toronto-cma", ""},
	}
	for _, tc := range cases {
		got, ok := bySlug[tc.slug]
		if !ok {
			t.Errorf("slug %s missing from ListRegions output", tc.slug)
			continue
		}
		if got != tc.wantParent {
			t.Errorf("%s: BrowseParentSlug want %q, got %q", tc.slug, tc.wantParent, got)
		}
	}
}

func TestMemStore_GetRegion_Metro_DescendantsIncluded(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetRegion(context.Background(), "nyc-metro")
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if got == nil {
		t.Fatal("nil result for known metro slug")
	}
	if got.Region.Slug != "nyc-metro" {
		t.Errorf("region slug: want nyc-metro, got %s", got.Region.Slug)
	}
	gotSlugs := orgSlugs(got.Orgs)
	sort.Strings(gotSlugs)
	want := []string{"riders-alliance", "transalt-brooklyn"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("orgs (-want +got):\n%s", diff)
	}
}

// TestMemStore_GetRegion_City_OnlyOwnDescendants pins the asymmetric
// scoping: a city slug returns only its own direct + descendant
// orgs, NOT orgs from its parent metro (which is an ancestor, not
// a descendant).
func TestMemStore_GetRegion_City_OnlyOwnDescendants(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetRegion(context.Background(), "brooklyn-ny")
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if got == nil {
		t.Fatal("nil result for known city slug")
	}
	gotSlugs := orgSlugs(got.Orgs)
	// Brooklyn has no descendants in the fixture and its direct orgs
	// are just transalt-brooklyn. riders-alliance (tagged to the
	// metro ancestor) must NOT appear.
	want := []string{"transalt-brooklyn"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("orgs (-want +got):\n%s", diff)
	}
}

// TestMemStore_GetRegion_Ancestry_LeafToRoot pins the breadcrumb
// data: ancestry comes back closest-first (direct parent at index 0,
// root last), excludes the region itself, and filters national-tier
// rows. The SPA renders this as a clickable breadcrumb in the
// Region page kicker.
func TestMemStore_GetRegion_Ancestry_LeafToRoot(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetRegion(context.Background(), "brooklyn-ny")
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if got == nil {
		t.Fatal("nil result")
	}
	var slugs []string
	for _, r := range got.Ancestry {
		slugs = append(slugs, r.Slug)
		if r.Slug == "brooklyn-ny" {
			t.Errorf("ancestry includes the region itself; want excluded")
		}
		if r.ScopeTier == ScopeNational {
			t.Errorf("national-tier region leaked into ancestry: %s", r.Slug)
		}
	}
	// Brooklyn's parents map: brooklyn-ny -> kings-county-ny ->
	// {nyc-metro, ny}; nyc-metro -> ny. BFS gives kings-county-ny
	// first, then [nyc-metro, ny] in the order kings-county added
	// them. The fixture's ParentSlugs is ["nyc-metro", "ny"] so
	// nyc-metro precedes ny. Result is closest-first:
	want := []string{"kings-county-ny", "nyc-metro", "ny"}
	if diff := cmp.Diff(want, slugs); diff != "" {
		t.Errorf("ancestry (-want +got):\n%s", diff)
	}
}

// TestMemStore_GetRegion_Ancestry_EmptyForRoot pins the
// no-ancestors case: a top-level region (no parents) returns an
// empty ancestry slice, not nil.
func TestMemStore_GetRegion_Ancestry_EmptyForRoot(t *testing.T) {
	s := newBrowseFixture()
	// NY is a us:state, top-of-hierarchy (no parents in the fixture).
	got, err := s.GetRegion(context.Background(), "ny")
	if err != nil {
		t.Fatalf("GetRegion(ny): %v", err)
	}
	if got == nil {
		t.Fatal("nil result for ny")
	}
	if got.Ancestry == nil {
		t.Errorf("ancestry: want non-nil empty slice, got nil")
	}
	if len(got.Ancestry) != 0 {
		t.Errorf("ancestry: want empty for top-level region, got %v",
			func() []string {
				out := []string{}
				for _, r := range got.Ancestry {
					out = append(out, r.Slug)
				}
				return out
			}())
	}
}

// TestMemStore_GetRegion_AnyNonNationalKind_Resolves pins the
// broadened detail-endpoint contract: states, counties, multi-state
// regions all resolve, returning their descendant orgs.
func TestMemStore_GetRegion_AnyNonNationalKind_Resolves(t *testing.T) {
	s := newBrowseFixture()

	// State slug — descendants include NYC Metro + Kings County + Brooklyn.
	got, err := s.GetRegion(context.Background(), "ny")
	if err != nil {
		t.Fatalf("GetRegion(ny): %v", err)
	}
	if got == nil {
		t.Fatal("nil result for ny (state)")
	}
	gotSlugs := orgSlugs(got.Orgs)
	sort.Strings(gotSlugs)
	want := []string{"ny-state-org", "riders-alliance", "transalt-brooklyn"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("ny (state) orgs (-want +got):\n%s", diff)
	}

	// County slug — descendants include Brooklyn.
	got, err = s.GetRegion(context.Background(), "kings-county-ny")
	if err != nil {
		t.Fatalf("GetRegion(kings-county-ny): %v", err)
	}
	if got == nil {
		t.Fatal("nil result for kings-county-ny (county)")
	}
	gotSlugs = orgSlugs(got.Orgs)
	want = []string{"transalt-brooklyn"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("kings-county-ny orgs (-want +got):\n%s", diff)
	}
}

func TestMemStore_GetRegion_OrderNewestFirst(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetRegion(context.Background(), "nyc-metro")
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
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

func TestMemStore_GetRegion_NotFound(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetRegion(context.Background(), "does-not-exist")
	if err != nil {
		t.Errorf("err: want nil, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil, got %+v", got)
	}
}

// TestMemStore_GetRegion_NationalReturnsNil pins the national-tier
// gate: even though pt-nacional is a real slug in the fixture, the
// handler must treat it as 404 to keep national-org content out of
// browse contexts.
func TestMemStore_GetRegion_NationalReturnsNil(t *testing.T) {
	s := newBrowseFixture()
	got, err := s.GetRegion(context.Background(), "pt-nacional")
	if err != nil {
		t.Errorf("err: want nil, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil for national-tier slug, got %+v", got)
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

func orgSlugs(orgs []Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}

func orgSlugSlice(orgs []Org) []string {
	return orgSlugs(orgs)
}
