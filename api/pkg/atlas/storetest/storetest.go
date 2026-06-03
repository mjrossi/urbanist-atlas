// Package storetest is a shared behavioral-contract test suite for
// atlas.Store implementations. MemStore is the only implementation
// today and runs the suite via MemStoreFactory; the suite is kept
// implementation-agnostic so any future store can run the same
// assertions against the same fixtures and can't drift from MemStore.
//
// Each contract corresponds to a doc-comment claim on the Store
// interface (see api/pkg/atlas/store.go). A failure means the
// implementation under test does not satisfy the contract; the bug
// belongs in the store, not the test.
//
// Usage from a consumer test file:
//
//	storetest.RunContractSuite(t, storetest.MemStoreFactory)
//
// A future store provides its own Factory, which builds a fresh store
// and returns a Seeder wired to it.
package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Seeder writes test fixtures into a Store implementation. Each
// backing store provides its own Seeder; the contract tests call
// these methods in a deterministic order so every implementation sees
// the same rows.
type Seeder interface {
	// SeedRegion registers a region. Parent slugs in r.ParentSlugs
	// must reference regions seeded earlier (parents-before-children).
	SeedRegion(t *testing.T, r atlas.Region)
	// SeedPostalCode registers a postal-code -> leaf-region mapping.
	SeedPostalCode(t *testing.T, country atlas.Country, code string, leafRegionID int64)
	// SeedOrg registers an approved organization and its region
	// attachments. AddedAt should round-trip through OrgsForRegions
	// and ListRecent.
	SeedOrg(t *testing.T, org atlas.Org, regionIDs []int64)
	// SeedRollupState records a directional rollup_states edge: metroSlug's
	// own orgs also surface on stateSlug's detail page (browse direction
	// only). Both slugs must reference regions seeded earlier.
	SeedRollupState(t *testing.T, metroSlug, stateSlug string)
}

// Factory builds a fresh, empty Store for one contract test and
// returns a Seeder for populating it plus a teardown hook.
type Factory func(t *testing.T) (store atlas.Store, seed Seeder, teardown func())

// RunContractSuite runs every contract test against the Store the
// factory builds. Each contract gets its own fresh store.
func RunContractSuite(t *testing.T, factory Factory) {
	t.Run("AncestorRegions_FiltersNational", func(t *testing.T) {
		testAncestorRegionsFiltersNational(t, factory)
	})
	t.Run("DescendantRegions_FiltersNational", func(t *testing.T) {
		testDescendantRegionsFiltersNational(t, factory)
	})
	t.Run("OrgsForRegions_RegionsSortedByID", func(t *testing.T) {
		testOrgsForRegionsRegionsSortedByID(t, factory)
	})
	t.Run("OrgsForRegions_PopulatesAddedAt", func(t *testing.T) {
		testOrgsForRegionsPopulatesAddedAt(t, factory)
	})
	t.Run("ListRegions_NearestBrowseableAncestor_AlphabeticTiebreak", func(t *testing.T) {
		testNearestBrowseableAncestorAlphabeticTiebreak(t, factory)
	})
	t.Run("ListRegions_FiltersNationalInDescendantWalk", func(t *testing.T) {
		testListRegionsFiltersNationalInDescendantWalk(t, factory)
	})
	t.Run("ListRegions_DirectOrgCountExcludesDescendantOrgs", func(t *testing.T) {
		testListRegionsDirectOrgCountExcludesDescendantOrgs(t, factory)
	})
	t.Run("GetRegion_DescendantRegionNames_ExcludesFocusAndAncestors", func(t *testing.T) {
		testGetRegionDescendantRegionNamesExcludesFocusAndAncestors(t, factory)
	})
	t.Run("GetRegion_RollupMetro_SurfacesOnStatePage", func(t *testing.T) {
		testGetRegionRollupMetroSurfacesOnStatePage(t, factory)
	})
	t.Run("AncestorRegions_RollupState_NoLeak", func(t *testing.T) {
		testAncestorRegionsRollupStateNoLeak(t, factory)
	})
	t.Run("SearchRegions_RanksAndFiltersNational", func(t *testing.T) {
		testSearchRegionsRanksAndFiltersNational(t, factory)
	})
}

// testAncestorRegionsFiltersNational seeds a city under a metro under
// a (national-tier) country, then asserts the upward walk from the
// city excludes the country.
func testAncestorRegionsFiltersNational(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// Top: a national-tier region (e.g. country umbrella).
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "pt:nacional", Name: "Portugal", Slug: "pt-nacional",
		Country: "PT", ScopeTier: atlas.ScopeNational, SortPriority: 100,
	})
	// Mid: a regional-tier metro under the national row.
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "pt:area-metropolitana", Name: "Lisboa AM", Slug: "lisboa-am",
		Country: "PT", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
		ParentSlugs: []string{"pt-nacional"},
	})
	// Leaf: a city-tier under the metro.
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "pt:municipio", Name: "Lisboa", Slug: "lisboa-mun",
		Country: "PT", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"lisboa-am"},
	})

	ancestors, err := store.AncestorRegions(context.Background(), 3)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	for _, r := range ancestors {
		if r.ScopeTier == atlas.ScopeNational {
			t.Errorf("national-tier row leaked into ancestors: %s (tier=%s)", r.Slug, r.ScopeTier)
		}
	}
	// Sanity: leaf at [0], regional metro at [1], no national row.
	if len(ancestors) != 2 {
		t.Errorf("ancestor count: want 2 (city + metro), got %d (%v)", len(ancestors), slugs(ancestors))
	}
}

// testDescendantRegionsFiltersNational seeds a regional metro that has
// (against editorial policy but allowed by schema) a national-tier
// child, plus a normal city child. Asserts the downward walk excludes
// the national node.
func testDescendantRegionsFiltersNational(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:metro", Name: "Test Metro", Slug: "test-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
	})
	// National-tier "child" of the metro (synthetic, never seeded by
	// production data, but the contract guarantees the walk would
	// skip it if editorial drift ever introduced one).
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:national", Name: "Synthetic National", Slug: "synth-national",
		Country: "US", ScopeTier: atlas.ScopeNational, SortPriority: 100,
		ParentSlugs: []string{"test-metro"},
	})
	// Normal city child.
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:city", Name: "Test City", Slug: "test-city",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"test-metro"},
	})

	descendants, err := store.DescendantRegions(context.Background(), 1)
	if err != nil {
		t.Fatalf("DescendantRegions: %v", err)
	}
	for _, r := range descendants {
		if r.ScopeTier == atlas.ScopeNational {
			t.Errorf("national-tier row leaked into descendants: %s (tier=%s)", r.Slug, r.ScopeTier)
		}
	}
}

// testOrgsForRegionsRegionsSortedByID seeds an org tagged to two
// regions in REVERSE id order; asserts the returned Org.Regions slice
// is ascending by ID regardless.
func testOrgsForRegionsRegionsSortedByID(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// Two browseable regions; the city's id (1) is smaller than the
	// metro's id (2).
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:city", Name: "Cityville", Slug: "cityville",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:metro", Name: "Metro Two", Slug: "metro-two",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
	})
	// Seed the org with regionIDs in REVERSE order (metro first).
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "org-x", Name: "Org X", ShortDesc: "test",
		WebsiteURL: "https://example.test",
	}, []int64{2, 1})

	orgs, err := store.OrgsForRegions(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("OrgsForRegions: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(orgs))
	}
	regions := orgs[0].Regions
	for i := 1; i < len(regions); i++ {
		if regions[i-1].ID > regions[i].ID {
			t.Errorf("Org.Regions not sorted by ID ascending: %v", regionIDs(regions))
			break
		}
	}
}

// testOrgsForRegionsPopulatesAddedAt seeds an org with a known
// AddedAt and asserts it round-trips through OrgsForRegions.
func testOrgsForRegionsPopulatesAddedAt(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:city", Name: "Cityville", Slug: "cityville",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
	})
	stamp := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "org-y", Name: "Org Y", ShortDesc: "test",
		WebsiteURL: "https://example.test",
		AddedAt:    stamp,
	}, []int64{1})

	orgs, err := store.OrgsForRegions(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("OrgsForRegions: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(orgs))
	}
	if orgs[0].AddedAt.IsZero() {
		t.Errorf("Org.AddedAt is zero; want %v round-tripped", stamp)
	} else if !orgs[0].AddedAt.Equal(stamp) {
		t.Errorf("Org.AddedAt = %v, want %v", orgs[0].AddedAt, stamp)
	}
}

// testNearestBrowseableAncestorAlphabeticTiebreak seeds a city with
// TWO browseable parents at the same depth, in REVERSE alphabetic
// declaration order. ListRegions should pick the alphabetically-first
// parent as browse_parent_slug regardless of declaration order.
func testNearestBrowseableAncestorAlphabeticTiebreak(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// Two browseable-kind metros at the same level.
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:metro", Name: "Zeta Metro", Slug: "zeta-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:metro", Name: "Alpha Metro", Slug: "alpha-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
	})
	// City with parents in REVERSE alphabetic declaration order.
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:city", Name: "Twoparents", Slug: "twoparents",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"zeta-metro", "alpha-metro"},
	})
	// Attach one approved org so the city surfaces in ListRegions.
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "twoparents-org", Name: "Twoparents Org",
		ShortDesc: "test", WebsiteURL: "https://example.test",
	}, []int64{3})

	regions, err := store.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	var got string
	for _, rs := range regions {
		if rs.Region.Slug == "twoparents" {
			got = rs.BrowseParentSlug
			break
		}
	}
	if got != "alpha-metro" {
		t.Errorf("browse_parent_slug for twoparents = %q, want %q (alphabetic tiebreak)", got, "alpha-metro")
	}
}

// testListRegionsFiltersNationalInDescendantWalk seeds a metro with a
// national-tier descendant. An org attached ONLY to the national
// descendant should not be counted in the metro's org_count.
func testListRegionsFiltersNationalInDescendantWalk(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:metro", Name: "Test Metro", Slug: "test-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
	})
	// National-tier child of the metro (synthetic).
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:national", Name: "Synthetic National", Slug: "synth-national",
		Country: "US", ScopeTier: atlas.ScopeNational, SortPriority: 100,
		ParentSlugs: []string{"test-metro"},
	})
	// Add at least one normal (non-national) attachment so the metro
	// surfaces in ListRegions; otherwise the empty-org case removes it.
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:city", Name: "Test City", Slug: "test-city",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"test-metro"},
	})
	// Org A: tagged only to the national-tier descendant.
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "national-only", Name: "National Only",
		ShortDesc: "test", WebsiteURL: "https://example.test",
	}, []int64{2})
	// Org B: tagged to the regular city, surfaces the metro.
	seed.SeedOrg(t, atlas.Org{
		ID: 101, Slug: "city-only", Name: "City Only",
		ShortDesc: "test", WebsiteURL: "https://example.test",
	}, []int64{3})

	regions, err := store.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	var metroCount int64 = -1
	for _, rs := range regions {
		if rs.Region.Slug == "test-metro" {
			metroCount = rs.OrgCount
			break
		}
	}
	if metroCount < 0 {
		t.Fatalf("test-metro missing from ListRegions output")
	}
	if metroCount != 1 {
		t.Errorf("test-metro org_count = %d, want 1 (national-only org must not count)", metroCount)
	}
}

// testListRegionsDirectOrgCountExcludesDescendantOrgs pins the
// editorial-totals contract: OrgCount walks the DAG downward and
// includes descendant attachments, while DirectOrgCount counts only
// orgs attached to the row itself. A regression that collapses the two
// would silently double-count any org surfacing under both a metro and
// one of its child cities — exactly what the SPA's Browse totals avoid
// by summing DirectOrgCount instead of OrgCount.
func testListRegionsDirectOrgCountExcludesDescendantOrgs(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// Metro with no direct org attachments; only the descendant city
	// carries the org. ListRegions must still surface the metro (because
	// the downward walk picks up the city's org) but its DirectOrgCount
	// stays at zero.
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:metro", Name: "Test Metro", Slug: "test-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:city", Name: "Test City", Slug: "test-city",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"test-metro"},
	})
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "city-only", Name: "City Only",
		ShortDesc: "test", WebsiteURL: "https://example.test",
	}, []int64{2})

	regions, err := store.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	var metro, city *atlas.RegionSummary
	for i := range regions {
		switch regions[i].Region.Slug {
		case "test-metro":
			metro = &regions[i]
		case "test-city":
			city = &regions[i]
		}
	}
	if metro == nil {
		t.Fatalf("test-metro missing from ListRegions output (downward walk should surface it via city's org)")
	}
	if city == nil {
		t.Fatalf("test-city missing from ListRegions output")
	}
	if metro.OrgCount != 1 {
		t.Errorf("test-metro OrgCount = %d, want 1 (descendant walk picks up city's org)", metro.OrgCount)
	}
	if metro.DirectOrgCount != 0 {
		t.Errorf("test-metro DirectOrgCount = %d, want 0 (no orgs attached directly to the metro)", metro.DirectOrgCount)
	}
	if city.OrgCount != 1 {
		t.Errorf("test-city OrgCount = %d, want 1", city.OrgCount)
	}
	if city.DirectOrgCount != 1 {
		t.Errorf("test-city DirectOrgCount = %d, want 1 (org is attached directly)", city.DirectOrgCount)
	}
}

// testGetRegionDescendantRegionNamesExcludesFocusAndAncestors pins the
// exclusion logic in atlas.GetRegion: the descendant slug→name map
// must include child slugs but exclude the focus's own slug and every
// ancestor slug. The SPA already has names for the focus (via
// `region`) and the ancestors (via `ancestry`), so leaking them into
// the map is wasted bytes and risks display bugs if the SPA composes
// them into a list.
func testGetRegionDescendantRegionNamesExcludesFocusAndAncestors(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// Multi-state ancestor → metro (focus) → two city children.
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:multistate", Name: "Test Multistate", Slug: "test-multistate",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 50,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:metro", Name: "Focus Metro", Slug: "focus-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
		ParentSlugs: []string{"test-multistate"},
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:city", Name: "City Alpha", Slug: "city-alpha",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"focus-metro"},
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 4, Kind: "us:city", Name: "City Beta", Slug: "city-beta",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 15,
		ParentSlugs: []string{"focus-metro"},
	})
	// One org per city so the focus has populated buckets; the test
	// asserts on the descendant_region_names map, not the bucketing.
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "alpha-org", Name: "Alpha Org",
		ShortDesc: "test", WebsiteURL: "https://example.test",
	}, []int64{3})
	seed.SeedOrg(t, atlas.Org{
		ID: 101, Slug: "beta-org", Name: "Beta Org",
		ShortDesc: "test", WebsiteURL: "https://example.test",
	}, []int64{4})

	detail, err := atlas.GetRegion(context.Background(), store, "focus-metro")
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if detail == nil {
		t.Fatalf("GetRegion returned nil for focus-metro")
	}
	names := detail.DescendantRegionNames
	if names == nil {
		t.Fatalf("DescendantRegionNames is nil; want empty-but-non-nil map at minimum")
	}
	if _, ok := names["focus-metro"]; ok {
		t.Errorf("DescendantRegionNames leaked focus slug %q (SPA has it via .region)", "focus-metro")
	}
	if _, ok := names["test-multistate"]; ok {
		t.Errorf("DescendantRegionNames leaked ancestor slug %q (SPA has it via .ancestry)", "test-multistate")
	}
	if got, want := names["city-alpha"], "City Alpha"; got != want {
		t.Errorf("DescendantRegionNames[city-alpha] = %q, want %q", got, want)
	}
	if got, want := names["city-beta"], "City Beta"; got != want {
		t.Errorf("DescendantRegionNames[city-beta] = %q, want %q", got, want)
	}
}

func slugs(regions []atlas.Region) []string {
	out := make([]string, len(regions))
	for i, r := range regions {
		out[i] = r.Slug
	}
	return out
}

func regionIDs(regions []atlas.Region) []int64 {
	out := make([]int64, len(regions))
	for i, r := range regions {
		out[i] = r.ID
	}
	return out
}

// testGetRegionRollupMetroSurfacesOnStatePage pins the rollup_states
// contract: a stateless multi-state metro's OWN orgs surface in the
// Regional bucket of a state that names it via rollup_states — and the
// rollup is NODE-only, so an out-of-state leaf's org beneath the metro
// does NOT leak onto the state page.
func testGetRegionRollupMetroSurfacesOnStatePage(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// il (state) and chicagoland (multi-state, no parent) are both roots.
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:state", Name: "Illinois", Slug: "il",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 60,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:multi-state", Name: "Chicagoland", Slug: "chicagoland",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 80,
	})
	// chicago-metro is stateless (parent = chicagoland only) and rolls up
	// to il via rollup_states.
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:metro", Name: "Chicago Metro", Slug: "chicago-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
		ParentSlugs: []string{"chicagoland"},
	})
	// An out-of-state leaf beneath the metro (Gary's county). Used to
	// prove the rollup is node-only — its org must NOT reach /il.
	seed.SeedRegion(t, atlas.Region{
		ID: 4, Kind: "us:county", Name: "Lake County, IN", Slug: "lake-county-in",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 30,
		ParentSlugs: []string{"chicago-metro"},
	})
	seed.SeedRollupState(t, "chicago-metro", "il")

	// Metro-tier org on the metro NODE; county org on the out-of-state leaf.
	seed.SeedOrg(t, atlas.Org{
		ID: 100, Slug: "ata", Name: "Active Transportation Alliance",
		ShortDesc: "metro", WebsiteURL: "https://example.test",
	}, []int64{3})
	seed.SeedOrg(t, atlas.Org{
		ID: 101, Slug: "gary-org", Name: "Gary Org",
		ShortDesc: "indiana", WebsiteURL: "https://example.test",
	}, []int64{4})

	detail, err := atlas.GetRegion(context.Background(), store, "il")
	if err != nil {
		t.Fatalf("GetRegion: %v", err)
	}
	if detail == nil {
		t.Fatalf("GetRegion returned nil for il")
	}

	// ATA must surface in Regional (chicago-metro is regional, not state-kind).
	if !hasOrg(detail.Regional, "ata") {
		t.Errorf("Regional missing rolled-up metro org 'ata'; got %v", orgSlugs(detail.Regional))
	}
	if hasOrg(detail.Local, "ata") || hasOrg(detail.Statewide, "ata") {
		t.Errorf("'ata' should be Regional only; local=%v statewide=%v", orgSlugs(detail.Local), orgSlugs(detail.Statewide))
	}
	// MatchedRegionSlugs should point at the metro that caused the surface.
	for _, o := range detail.Regional {
		if o.Slug == "ata" {
			if got := o.MatchedRegionSlugs; len(got) != 1 || got[0] != "chicago-metro" {
				t.Errorf("ata MatchedRegionSlugs = %v, want [chicago-metro]", got)
			}
		}
	}
	// Node-only: the out-of-state leaf's org must NOT appear anywhere.
	if hasOrg(detail.Local, "gary-org") || hasOrg(detail.Regional, "gary-org") || hasOrg(detail.Statewide, "gary-org") {
		t.Errorf("node-only violated: gary-org (under chicago-metro) leaked onto /il")
	}
	// The rolled-up metro's slug→name must resolve for the SPA's "matched via".
	if got, want := detail.DescendantRegionNames["chicago-metro"], "Chicago Metro"; got != want {
		t.Errorf("DescendantRegionNames[chicago-metro] = %q, want %q", got, want)
	}
}

// testAncestorRegionsRollupStateNoLeak pins the leak-free guarantee:
// rollup_states is NOT an ancestor edge, so a leaf beneath a metro that
// rolls up to a state never reaches that state via the ancestor walk.
func testAncestorRegionsRollupStateNoLeak(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:state", Name: "Illinois", Slug: "il",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 60,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:multi-state", Name: "Chicagoland", Slug: "chicagoland",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 80,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:metro", Name: "Chicago Metro", Slug: "chicago-metro",
		Country: "US", ScopeTier: atlas.ScopeRegional, SortPriority: 40,
		ParentSlugs: []string{"chicagoland"},
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 4, Kind: "us:county", Name: "Lake County, IN", Slug: "lake-county-in",
		Country: "US", ScopeTier: atlas.ScopeLocal, SortPriority: 30,
		ParentSlugs: []string{"chicago-metro"},
	})
	seed.SeedRollupState(t, "chicago-metro", "il")

	ancestors, err := store.AncestorRegions(context.Background(), 4)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	got := slugs(ancestors)
	// Sanity: the real ancestor edges resolve.
	if !containsSlug(got, "chicago-metro") || !containsSlug(got, "chicagoland") {
		t.Errorf("ancestor walk missing real edges: got %v", got)
	}
	// The leak guard: il must NOT appear (rollup_states is browse-only).
	if containsSlug(got, "il") {
		t.Errorf("rollup_states leaked into the ancestor walk: il reached from lake-county-in (got %v)", got)
	}
}

// testSearchRegionsRanksAndFiltersNational seeds two same-named cities
// in different states plus a national-tier row whose name matches the
// query, then asserts SearchRegions: (a) excludes the national row,
// (b) ranks an exact slug match ahead of name matches, and (c) carries
// a state-ancestor ContextLabel that disambiguates the duplicates.
func testSearchRegionsRanksAndFiltersNational(t *testing.T, factory Factory) {
	store, seed, teardown := factory(t)
	defer teardown()

	// Two states.
	seed.SeedRegion(t, atlas.Region{
		ID: 1, Kind: "us:state", Name: "Illinois", Slug: "il",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeRegional,
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 2, Kind: "us:state", Name: "Massachusetts", Slug: "ma",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeRegional,
	})
	// Same-named cities, one per state.
	seed.SeedRegion(t, atlas.Region{
		ID: 3, Kind: "us:city", Name: "Springfield", Slug: "springfield-il",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeLocal,
		ParentSlugs: []string{"il"},
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 4, Kind: "us:city", Name: "Springfield", Slug: "springfield-ma",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeLocal,
		ParentSlugs: []string{"ma"},
	})
	// An exact-slug target and a national row that both match "springfield".
	seed.SeedRegion(t, atlas.Region{
		ID: 5, Kind: "us:metro", Name: "Greater Springfield", Slug: "springfield",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeRegional,
		ParentSlugs: []string{"il"},
	})
	seed.SeedRegion(t, atlas.Region{
		ID: 6, Kind: "xx:national", Name: "Springfield National Network", Slug: "springfield-national",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeNational,
	})

	got, err := store.SearchRegions(context.Background(), "springfield", 0)
	if err != nil {
		t.Fatalf("SearchRegions: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want results for 'springfield', got none")
	}
	// Exact slug match ranks first.
	if got[0].Region.Slug != "springfield" {
		t.Errorf("exact-slug match should rank first; got %q", got[0].Region.Slug)
	}
	// National row never surfaces.
	for _, r := range got {
		if r.Region.ScopeTier == atlas.ScopeNational || r.Region.Slug == "springfield-national" {
			t.Errorf("national-tier row leaked into search results: %s", r.Region.Slug)
		}
	}
	// The two duplicate-named cities carry distinct state context labels.
	labels := map[string]string{}
	for _, r := range got {
		labels[r.Region.Slug] = r.ContextLabel
	}
	if labels["springfield-il"] != "Illinois" {
		t.Errorf("springfield-il context label: want %q, got %q", "Illinois", labels["springfield-il"])
	}
	if labels["springfield-ma"] != "Massachusetts" {
		t.Errorf("springfield-ma context label: want %q, got %q", "Massachusetts", labels["springfield-ma"])
	}
}

func hasOrg(orgs []atlas.Org, slug string) bool {
	for _, o := range orgs {
		if o.Slug == slug {
			return true
		}
	}
	return false
}

func orgSlugs(orgs []atlas.Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}

func containsSlug(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
