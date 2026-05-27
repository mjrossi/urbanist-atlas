// Package storetest is a shared behavioral-contract test suite for
// atlas.Store implementations. MemStore (unit tests) and the Postgres
// adapter (integration tests, behind //go:build integration) both run
// the same assertions against the same fixtures so the two stores
// can't drift quietly.
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
// or, for the Postgres adapter, a factory that boots a testcontainer
// and returns a Seeder wired to the pool.
package storetest

import (
	"context"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Seeder writes test fixtures into a Store implementation. Each
// backing store provides its own Seeder; the contract tests call
// these methods in a deterministic order so MemStore and Postgres see
// the same rows.
type Seeder interface {
	// SeedRegion registers a region. Parent slugs in r.ParentSlugs
	// must reference regions seeded earlier (parents-before-children).
	SeedRegion(t *testing.T, r atlas.Region)
	// SeedPostalCode registers a postal-code -> leaf-region mapping.
	SeedPostalCode(t *testing.T, country atlas.Country, code string, leafRegionID int64)
	// SeedOrg registers an approved organization and its region
	// attachments. CreatedAt should round-trip through OrgsForRegions
	// and ListRecent.
	SeedOrg(t *testing.T, org atlas.Org, regionIDs []int64)
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
	t.Run("OrgsForRegions_PopulatesCreatedAt", func(t *testing.T) {
		testOrgsForRegionsPopulatesCreatedAt(t, factory)
	})
	t.Run("ListRegions_NearestBrowseableAncestor_AlphabeticTiebreak", func(t *testing.T) {
		testNearestBrowseableAncestorAlphabeticTiebreak(t, factory)
	})
	t.Run("ListRegions_FiltersNationalInDescendantWalk", func(t *testing.T) {
		testListRegionsFiltersNationalInDescendantWalk(t, factory)
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

// testOrgsForRegionsPopulatesCreatedAt seeds an org with a known
// CreatedAt and asserts it round-trips through OrgsForRegions.
func testOrgsForRegionsPopulatesCreatedAt(t *testing.T, factory Factory) {
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
		CreatedAt:  stamp,
	}, []int64{1})

	orgs, err := store.OrgsForRegions(context.Background(), []int64{1})
	if err != nil {
		t.Fatalf("OrgsForRegions: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected 1 org, got %d", len(orgs))
	}
	if orgs[0].CreatedAt.IsZero() {
		t.Errorf("Org.CreatedAt is zero; want %v round-tripped", stamp)
	} else if !orgs[0].CreatedAt.Equal(stamp) {
		t.Errorf("Org.CreatedAt = %v, want %v", orgs[0].CreatedAt, stamp)
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
