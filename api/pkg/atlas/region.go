package atlas

import (
	"context"
	"errors"
	"fmt"
)

// GetRegion returns the RegionDetail for the region identified by slug:
// the focus region plus the orgs in scope for it, bucketed by attachment
// scope_tier, plus the upward ancestry walk used to render a breadcrumb
// in the SPA.
//
// "In scope" means the focus region itself and every DAG descendant —
// so a metro surfaces its constituent cities' orgs, and a state
// surfaces every metro/city beneath it. Ancestor orgs are NOT pulled
// in: a city's detail page shouldn't be flooded with the state's
// catch-all coalitions just because the city sits under the state.
// This matches the descendant-walk org_count shown on the browse list
// (ListRegions), so the card's promised count and the detail page's
// delivered count agree.
//
// Local + Regional buckets are decided by the scope_tier of the org's
// matched attachment regions — same rule Lookup uses for its own
// in-scope set.
//
// Sibling to Lookup, not a clone of it: Lookup answers "what works at
// this postal-code address?" and walks ancestors upward (a Brooklyn
// ZIP's address is also in NYC Metro and New York State, so orgs
// covering those should surface). GetRegion answers "what does this
// region contain?" and walks descendants downward. Same primitives,
// opposite direction.
//
// Algorithm:
//  1. ResolveRegionBySlug(slug) → focus Region. Returns ErrRegionNotFound
//     for unknown slugs and for national-tier rows. Callers map both to
//     404 — the wire contract makes no distinction.
//  2. DescendantRegions(focusID) → []Region, root-first, national-filtered.
//     Used for both the in-scope set and DescendantRegionNames.
//  3. AncestorRegions(focusID) → []Region, leaf-first, national-filtered.
//     Used only for the SPA breadcrumb (Ancestry) — never for org scope.
//  4. OrgsForRegions(focus ∪ descendants) → []Org.
//  5. BucketOrgsByScope splits into Local / Regional.
//  6. Build the breadcrumb-friendly ancestry slice (closest-first,
//     excluding the focus itself).
func GetRegion(ctx context.Context, store Store, slug string) (*RegionDetail, error) {
	region, err := store.ResolveRegionBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, ErrRegionNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("atlas: resolve region: %w", err)
	}

	descendants, err := store.DescendantRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("atlas: descendant regions: %w", err)
	}

	ancestors, err := store.AncestorRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("atlas: ancestor regions: %w", err)
	}

	inScope := make(map[int64]Region, len(descendants))
	for _, r := range descendants {
		inScope[r.ID] = r
	}

	ids := make([]int64, 0, len(inScope))
	for k := range inScope {
		ids = append(ids, k)
	}
	orgs, err := store.OrgsForRegions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("atlas: orgs for regions: %w", err)
	}

	local, regional := BucketOrgsByScope(inScope, orgs)

	// Ancestry for the SPA breadcrumb: closest-first, excluding the
	// focus itself. AncestorRegions returns focus at [0]; the slice
	// stays non-nil even for top-of-hierarchy regions so the wire
	// shape is stable (`[]` not `null`).
	ancestry := make([]Region, 0)
	if len(ancestors) > 1 {
		ancestry = append(ancestry, ancestors[1:]...)
	}

	// Descendant slug → display-name lookup. Excludes the focus and
	// any ancestor (the SPA already has names for those via `region`
	// and `ancestry`). Empty-but-non-nil so the JSON contract is `{}`
	// not `null`.
	skip := make(map[string]struct{}, len(ancestors))
	skip[region.Slug] = struct{}{}
	for _, r := range ancestors {
		skip[r.Slug] = struct{}{}
	}
	descendantNames := make(map[string]string)
	for _, r := range descendants {
		if _, ok := skip[r.Slug]; ok {
			continue
		}
		descendantNames[r.Slug] = r.Name
	}

	return &RegionDetail{
		Region:                region,
		Local:                 local,
		Regional:              regional,
		Ancestry:              ancestry,
		DescendantRegionNames: descendantNames,
	}, nil
}
