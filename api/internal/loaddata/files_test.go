package loaddata

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// seedDir resolves to the repo's api/seed/ directory regardless of
// where `go test` is invoked from (typically the package directory).
func seedDir(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("..", "..", "seed"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func TestBuildMemStore_LoadsBundledSeed(t *testing.T) {
	store, err := BuildMemStore(nil, seedDir(t))
	if err != nil {
		t.Fatalf("BuildMemStore: %v", err)
	}
	ctx := context.Background()

	// US Brooklyn ZIP should resolve and surface at least one org.
	leaf, err := store.ResolveLeafRegion(ctx, atlas.CountryUS, "11217")
	if err != nil {
		t.Fatalf("ResolveLeafRegion 11217: %v", err)
	}
	if leaf.Slug == "" {
		t.Fatal("11217 resolved to a region with empty slug")
	}
	ancestors, err := store.AncestorRegions(ctx, leaf.ID)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	if len(ancestors) < 2 {
		t.Fatalf("expected leaf + at least one ancestor, got %d", len(ancestors))
	}
	ancestorIDs := make([]int64, 0, len(ancestors))
	for _, r := range ancestors {
		ancestorIDs = append(ancestorIDs, r.ID)
	}
	orgs, err := store.OrgsForRegions(ctx, ancestorIDs)
	if err != nil {
		t.Fatalf("OrgsForRegions: %v", err)
	}
	if len(orgs) == 0 {
		t.Fatal("expected at least one org for 11217 ancestry, got none")
	}

	// CA Montreal FSA — sanity-check Canadian postal handling.
	if _, err := store.ResolveLeafRegion(ctx, atlas.CountryCA, "H3A 0G4"); err != nil {
		t.Fatalf("ResolveLeafRegion H3A 0G4: %v", err)
	}

	// ListRegions should return a non-trivial set with org counts.
	regions, err := store.ListRegions(ctx)
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) == 0 {
		t.Fatal("ListRegions returned no regions with orgs")
	}

	// ListRecent should return up to 10 orgs.
	recent, err := store.ListRecent(ctx)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(recent) == 0 {
		t.Fatal("ListRecent returned no orgs")
	}
	if len(recent) > 10 {
		t.Fatalf("ListRecent returned %d > 10 orgs", len(recent))
	}
}

func TestBuildMemStore_Idempotent(t *testing.T) {
	// Building twice should yield equivalent stores (same slug -> same
	// answer for a known lookup). IDs may differ but the wire surface
	// is slug-keyed.
	dir := seedDir(t)
	a, err := BuildMemStore(nil, dir)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	b, err := BuildMemStore(nil, dir)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	ctx := context.Background()
	la, errA := a.ResolveLeafRegion(ctx, atlas.CountryUS, "11217")
	lb, errB := b.ResolveLeafRegion(ctx, atlas.CountryUS, "11217")
	if errA != nil || errB != nil {
		t.Fatalf("resolve errs: a=%v b=%v", errA, errB)
	}
	if la.Slug != lb.Slug {
		t.Fatalf("non-deterministic leaf for 11217: a=%q b=%q", la.Slug, lb.Slug)
	}
}
