package atlas

import (
	"context"
	"fmt"
)

// Lookup is the core search operation: given a postal code, return the
// local + regional organizations advocating in that area.
//
// Algorithm (per docs/superpowers/specs/2026-05-16-region-graph-design.md):
//  1. ResolveLeafRegion(country, code) → leaf Region; 404 if unknown.
//  2. AncestorRegions(leafID) → []Region (leaf + all transitive parents).
//  3. OrgsForRegions(ancestorIDs) → []Org with each org's full
//     attachment list populated.
//  4. BucketOrgsByScope splits orgs into Local / Regional / Statewide
//     by the scope_tier and kind of the matched attachment region
//     (shared with GetRegion, which walks both directions instead of
//     just up).
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
	inScope := make(map[int64]Region, len(ancestry))
	for i, r := range ancestry {
		ancestorIDs[i] = r.ID
		inScope[r.ID] = r
	}

	orgs, err := store.OrgsForRegions(ctx, ancestorIDs)
	if err != nil {
		return LookupResult{}, fmt.Errorf("atlas: orgs lookup: %w", err)
	}

	local, regional, statewide := BucketOrgsByScope(inScope, orgs)

	return LookupResult{
		Query:              query,
		ResolvedPlaceLabel: placeLabel(ancestry),
		ResolvedAncestry:   ancestry,
		Local:              local,
		Regional:           regional,
		Statewide:          statewide,
	}, nil
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
	broad := pickBroadAncestor(ancestry)
	inner := pickInnerAncestor(ancestry, leaf, broad)

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

// pickBroadAncestor selects placeLabel's "broad" slot: the most-specific
// metro-kind ancestor, or — when no metro-kind ancestor exists — the
// first regional-tier ancestor (so transit federations and other
// non-metro regional contexts still fill the slot). Scans ancestry[1:]
// (skipping the leaf). Returns nil when neither match exists.
//
// r := ancestry[i] inside each loop body is a fresh local each iteration
// — taking &r is safe and escapes to the heap, so the caller keeps a
// valid pointer after the return.
func pickBroadAncestor(ancestry []Region) *Region {
	for i := 1; i < len(ancestry); i++ {
		r := ancestry[i]
		if IsMetroKind(r.Kind) {
			return &r
		}
	}
	for i := 1; i < len(ancestry); i++ {
		r := ancestry[i]
		if r.ScopeTier == ScopeRegional {
			return &r
		}
	}
	return nil
}

// pickInnerAncestor selects placeLabel's "inner" slot: the first
// non-leaf ancestor below state tier (SortPriority < 60) that is
// distinct from the leaf and the already-chosen broad slot, so the label
// captures local civic context (NYC for a borough, Cook County for a
// Chicago neighborhood) without repeating the broad slot or padding with
// state/multi-state geography. broad may be nil. Returns nil when no
// such ancestor exists.
func pickInnerAncestor(ancestry []Region, leaf Region, broad *Region) *Region {
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
		return &r
	}
	return nil
}
