package atlas

import (
	"context"
	"reflect"
	"testing"
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
	if !reflect.DeepEqual(gotSlugs, want) {
		t.Errorf("ancestor order:\n  got  %v\n  want %v", gotSlugs, want)
	}
}

func TestMemStore_ResolveLeafRegion_NotFound(t *testing.T) {
	s := NewMemStore()
	_, err := s.ResolveLeafRegion(context.Background(), "US", "00000")
	if err != ErrPostalCodeNotFound {
		t.Errorf("err = %v, want ErrPostalCodeNotFound", err)
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
