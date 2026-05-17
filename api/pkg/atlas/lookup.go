package atlas

import (
	"context"
	"fmt"
	"sort"
)

// Lookup is the core search operation: given a postal code, return the
// local + regional organizations advocating in that area.
//
// Algorithm (per docs/superpowers/specs/2026-05-16-region-graph-design.md):
//  1. ResolveLeafRegion(country, code) → leaf Region; 404 if unknown.
//  2. AncestorRegions(leafID) → []Region (leaf + all transitive parents).
//  3. OrgsForRegions(ancestorIDs) → []Org with each org's full
//     attachment list populated.
//  4. For each org, intersect its regions with the ancestor set. If any
//     matched region has scope_tier=local, bucket as Local; else Regional.
//     Compute the org's sort key as the minimum sort_priority across
//     its matched regions.
//  5. Within each bucket, sort by (sortKey asc, org.Name asc).
func Lookup(ctx context.Context, store Store, query LookupQuery) (LookupResult, error) {
	leaf, err := store.ResolveLeafRegion(ctx, query.Country, query.PostalCode)
	if err != nil {
		return LookupResult{}, err
	}

	ancestry, err := store.AncestorRegions(ctx, leaf.ID)
	if err != nil {
		return LookupResult{}, fmt.Errorf("atlas: ancestor regions: %w", err)
	}

	ancestorIDs := make([]int64, len(ancestry))
	ancestorByID := make(map[int64]Region, len(ancestry))
	for i, r := range ancestry {
		ancestorIDs[i] = r.ID
		ancestorByID[r.ID] = r
	}

	orgs, err := store.OrgsForRegions(ctx, ancestorIDs)
	if err != nil {
		return LookupResult{}, fmt.Errorf("atlas: orgs lookup: %w", err)
	}

	var local, regional []bucketed
	for _, org := range orgs {
		matched := make([]Region, 0)
		for _, r := range org.Regions {
			if _, ok := ancestorByID[r.ID]; ok {
				matched = append(matched, ancestorByID[r.ID])
			}
		}
		if len(matched) == 0 {
			continue
		}
		hasLocal := false
		bestSort := matched[0].SortPriority
		matchedSlugs := make([]string, 0, len(matched))
		for _, r := range matched {
			if r.ScopeTier == ScopeLocal {
				hasLocal = true
			}
			if r.SortPriority < bestSort {
				bestSort = r.SortPriority
			}
			matchedSlugs = append(matchedSlugs, r.Slug)
		}
		org.MatchedRegionSlugs = matchedSlugs
		b := bucketed{org: org, sortKey: bestSort}
		if hasLocal {
			local = append(local, b)
		} else {
			regional = append(regional, b)
		}
	}

	sortBucket(local)
	sortBucket(regional)

	return LookupResult{
		Query:              query,
		ResolvedPlaceLabel: placeLabel(ancestry),
		ResolvedAncestry:   ancestry,
		Local:              extractOrgs(local),
		Regional:           extractOrgs(regional),
	}, nil
}

// bucketed is a private result-row type used by Lookup for sorting.
type bucketed struct {
	org     Org
	sortKey int
}

func sortBucket(b []bucketed) {
	sort.SliceStable(b, func(i, j int) bool {
		if b[i].sortKey != b[j].sortKey {
			return b[i].sortKey < b[j].sortKey
		}
		return b[i].org.Name < b[j].org.Name
	})
}

func extractOrgs(b []bucketed) []Org {
	if len(b) == 0 {
		return []Org{}
	}
	out := make([]Org, len(b))
	for i, x := range b {
		out[i] = x.org
	}
	return out
}

// placeLabel returns a human-readable header derived from the ancestry.
// Format: "<leaf>, <most-specific-local-ancestor-different-from-leaf> — <most-specific-regional-ancestor>".
// Segments without content are dropped; the SPA can roll its own from
// ResolvedAncestry if it wants something different.
func placeLabel(ancestry []Region) string {
	if len(ancestry) == 0 {
		return ""
	}
	leaf := ancestry[0]
	var localAncestor, regionalAncestor *Region
	for i := 1; i < len(ancestry); i++ {
		r := ancestry[i]
		if r.ScopeTier == ScopeLocal && localAncestor == nil && r.Slug != leaf.Slug {
			cp := r
			localAncestor = &cp
		}
		if r.ScopeTier == ScopeRegional && regionalAncestor == nil {
			cp := r
			regionalAncestor = &cp
		}
	}
	switch {
	case localAncestor != nil && regionalAncestor != nil:
		return leaf.Name + ", " + localAncestor.Name + " — " + regionalAncestor.Name
	case regionalAncestor != nil:
		return leaf.Name + " — " + regionalAncestor.Name
	case localAncestor != nil:
		return leaf.Name + ", " + localAncestor.Name
	default:
		return leaf.Name
	}
}
