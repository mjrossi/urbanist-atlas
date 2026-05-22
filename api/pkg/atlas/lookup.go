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
		// Wrap so log lines (and any outer errors.Wrap chain) show the
		// resolve step alongside the underlying message. errors.Is still
		// finds atlas.ErrPostalCodeNotFound through the wrap, so the
		// HTTP layer's 404 mapping is preserved.
		return LookupResult{}, fmt.Errorf("atlas: resolve postal code: %w", err)
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
			if ar, ok := ancestorByID[r.ID]; ok {
				matched = append(matched, ar)
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
// Format: "<leaf>, <inner-ancestor> — <broad-ancestor>".
// Segments without content are dropped; the SPA can roll its own from
// ResolvedAncestry if it wants something different.
//
// Ancestor picks:
//
//   - broad: the most-specific IsMetroKind ancestor (us:metro, ca:cma,
//     ca:regional-district, pt:area-metropolitana). If no metro-kind
//     ancestor exists, falls back to the first regional ancestor —
//     handles transit federations (Berlin's VBB) and other non-metro
//     regional contexts.
//   - inner: the first non-leaf ancestor that is *below* state tier
//     (sort_priority < 60) and distinct from broad. Captures local
//     civic context — NYC for borough leaves, Berlin for Mitte,
//     Cook County for a Chicago neighborhood. State, multi-state,
//     and the broad slot itself are excluded so the label doesn't
//     repeat itself or pad with administrative geography.
//
// Both NYC (regional, kind us:city) and Berlin (local, kind de:land)
// land in the inner slot via the sort_priority < 60 test — they're
// sub-state-tier intermediate civic units regardless of scope_tier.
func placeLabel(ancestry []Region) string {
	if len(ancestry) == 0 {
		return ""
	}
	leaf := ancestry[0]

	var broad *Region
	for i := 1; i < len(ancestry); i++ {
		r := ancestry[i]
		if IsMetroKind(r.Kind) {
			cp := r
			broad = &cp
			break
		}
	}
	if broad == nil {
		for i := 1; i < len(ancestry); i++ {
			r := ancestry[i]
			if r.ScopeTier == ScopeRegional {
				cp := r
				broad = &cp
				break
			}
		}
	}

	var inner *Region
	for i := 1; i < len(ancestry); i++ {
		r := ancestry[i]
		if r.Slug == leaf.Slug {
			continue
		}
		if broad != nil && r.Slug == broad.Slug {
			continue
		}
		if r.SortPriority >= 60 { // state-tier or higher: excludes state/multi-state.
			continue
		}
		cp := r
		inner = &cp
		break
	}

	switch {
	case inner != nil && broad != nil:
		return leaf.Name + ", " + inner.Name + " — " + broad.Name
	case broad != nil:
		return leaf.Name + " — " + broad.Name
	case inner != nil:
		return leaf.Name + ", " + inner.Name
	default:
		return leaf.Name
	}
}
