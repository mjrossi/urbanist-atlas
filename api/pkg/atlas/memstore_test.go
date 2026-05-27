package atlas

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestMemStore_GraphWalk constructs a small NYC subset and verifies
// ResolveLeafRegion + AncestorRegions produce the expected walk.
func TestMemStore_GraphWalk(t *testing.T) {
	s := NewMemStore()
	// Add parents-first so AddRegion can resolve slug→id.
	s.AddRegion(Region{ID: 1, Kind: "us:multi-state", Name: "Tri-State", Slug: "nyc-tristate", Country: "US", ScopeTier: ScopeRegional, SortPriority: 80})
	s.AddRegion(Region{ID: 2, Kind: "us:state", Name: "New York", Slug: "ny", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60})
	s.AddRegion(Region{ID: 3, Kind: "us:metro", Name: "NYC Metro", Slug: "nyc-metro", Country: "US", ScopeTier: ScopeRegional, SortPriority: 40, ParentSlugs: []string{"nyc-tristate"}})
	s.AddRegion(Region{ID: 4, Kind: "us:city", Name: "NYC", Slug: "nyc", Country: "US", ScopeTier: ScopeLocal, SortPriority: 15, ParentSlugs: []string{"nyc-metro", "ny"}})
	s.AddRegion(Region{ID: 5, Kind: "us:borough", Name: "Brooklyn", Slug: "brooklyn", Country: "US", ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"nyc"}})

	s.AddPostalCode("US", "11217", 5)

	leaf, err := s.ResolveLeafRegion(context.Background(), "US", "11217")
	if err != nil {
		t.Fatalf("ResolveLeafRegion: %v", err)
	}
	if leaf.Slug != "brooklyn" {
		t.Errorf("leaf = %q, want brooklyn", leaf.Slug)
	}

	ancestors, err := s.AncestorRegions(context.Background(), leaf.ID)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	gotSlugs := make([]string, 0, len(ancestors))
	for _, r := range ancestors {
		gotSlugs = append(gotSlugs, r.Slug)
	}
	// BFS from brooklyn → nyc → {nyc-metro, ny} → nyc-tristate.
	want := []string{"brooklyn", "nyc", "nyc-metro", "ny", "nyc-tristate"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("ancestor order (-want +got):\n%s", diff)
	}
}

func TestMemStore_ResolveLeafRegion_NotFound(t *testing.T) {
	s := NewMemStore()
	_, err := s.ResolveLeafRegion(context.Background(), "US", "00000")
	if err != ErrPostalCodeNotFound {
		t.Errorf("err = %v, want ErrPostalCodeNotFound", err)
	}
}

func TestMemStore_GetOrgBySlug_HappyPath(t *testing.T) {
	s := NewMemStore()
	s.AddRegion(Region{ID: 1, Kind: "us:metro", Name: "NYC Metro", Slug: "nyc-metro", Country: "US", ScopeTier: ScopeRegional, SortPriority: 40})
	s.AddRegion(Region{ID: 2, Kind: "us:borough", Name: "Brooklyn", Slug: "brooklyn", Country: "US", ScopeTier: ScopeLocal, SortPriority: 10, ParentSlugs: []string{"nyc-metro"}})
	s.AddOrg(Org{
		ID: 1, Slug: "transalt",
		Name:       "Transportation Alternatives",
		ShortDesc:  "NYC-wide advocacy for walking, biking, and public transit.",
		WebsiteURL: "https://www.transalt.org",
		Tags:       []Tag{"transit", "safe-streets"},
	}, []int64{1, 2})

	got, err := s.GetOrgBySlug(context.Background(), "transalt")
	if err != nil {
		t.Fatalf("GetOrgBySlug: %v", err)
	}
	if got == nil {
		t.Fatal("nil result for known slug")
	}
	if got.Slug != "transalt" || got.Name != "Transportation Alternatives" {
		t.Errorf("got %+v", got)
	}
	if len(got.Regions) != 2 {
		t.Errorf("regions: want 2, got %d", len(got.Regions))
	}
	gotSlugs := []string{}
	for _, r := range got.Regions {
		gotSlugs = append(gotSlugs, r.Slug)
	}
	want := []string{"nyc-metro", "brooklyn"}
	if diff := cmp.Diff(want, gotSlugs); diff != "" {
		t.Errorf("region slugs (-want +got):\n%s", diff)
	}
}

func TestMemStore_GetOrgBySlug_NotFound(t *testing.T) {
	s := NewMemStore()
	_, err := s.GetOrgBySlug(context.Background(), "nope")
	if !errors.Is(err, ErrOrgNotFound) {
		t.Errorf("err = %v, want ErrOrgNotFound", err)
	}
}

func TestMemStore_AncestorRegions_TopOfTree(t *testing.T) {
	s := NewMemStore()
	s.AddRegion(Region{ID: 1, Slug: "ny", Kind: "us:state", Name: "New York", Country: "US", ScopeTier: ScopeRegional, SortPriority: 60})
	got, err := s.AncestorRegions(context.Background(), 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "ny" {
		t.Errorf("got %v, want [{slug: ny}]", got)
	}
}

// BenchmarkMemStore_ListRegions measures the steady-state cost of
// ListRegions at a region count comparable to the v1 US seed (~600
// regions with a state-tier root and city-tier leaves). The hoist of
// buildChildrenOf out of the per-root descendantRegionIDs loop turns
// this from O(R · P) into O(R + P) per call. A reversion that puts
// the rebuild back in the inner loop will visibly regress.
func BenchmarkMemStore_ListRegions(b *testing.B) {
	const (
		metros = 50  // each a us:metro at SortPriority 40
		cities = 600 // each a us:city under one metro at SortPriority 10
	)
	s := NewMemStore()
	s.AddRegion(Region{ID: 1, Slug: "us-state", Kind: "us:state", Name: "Synthetic State",
		Country: "US", ScopeTier: ScopeRegional, SortPriority: 60})
	for i := range metros {
		slug := fmt.Sprintf("metro-%03d", i)
		s.AddRegion(Region{
			ID: int64(100 + i), Slug: slug, Kind: "us:metro",
			Name: slug, Country: "US", ScopeTier: ScopeRegional, SortPriority: 40,
			ParentSlugs: []string{"us-state"},
		})
	}
	for i := range cities {
		parent := fmt.Sprintf("metro-%03d", i%metros)
		slug := fmt.Sprintf("city-%04d", i)
		s.AddRegion(Region{
			ID: int64(10000 + i), Slug: slug, Kind: "us:city",
			Name: slug, Country: "US", ScopeTier: ScopeLocal, SortPriority: 10,
			ParentSlugs: []string{parent},
		})
		// One org per city so every metro has at least one descendant
		// org and ListRegions actually returns rows.
		s.AddOrg(Org{
			ID: int64(20000 + i), Slug: fmt.Sprintf("org-%04d", i),
			Name: "Org", ShortDesc: "synthetic", WebsiteURL: "https://example.org",
		}, []int64{int64(10000 + i)})
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := s.ListRegions(context.Background()); err != nil {
			b.Fatalf("ListRegions: %v", err)
		}
	}
}
