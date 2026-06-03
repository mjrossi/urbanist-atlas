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
// "In scope" means the focus region itself, every DAG descendant, and
// any metro that rolls up to the focus via rollup_states — so a metro
// surfaces its constituent cities' orgs, and a state surfaces every
// metro/city beneath it PLUS the OWN orgs of any stateless multi-state
// metro that names it in rollup_states (the metro NODE only, never the
// metro's out-of-state subtree). Ancestor orgs are NOT pulled in: a
// city's detail page shouldn't be flooded with the state's catch-all
// coalitions just because the city sits under the state. For browseable
// regions (metros/cities) this still matches the descendant-walk
// org_count on the browse list (ListRegions) — rollup targets are
// state-equivalent kinds, which ListRegions does not browse, so the
// card↔detail count invariant is unaffected.
//
// Local + Regional + Statewide buckets are decided by the scope_tier
// and kind of the org's matched attachment regions — same rule Lookup
// uses for its own in-scope set (see BucketOrgsByScope).
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
//     2b. RollupMetrosFor(focusID) → []Region (rollup_states). The metro
//     NODES are added to the in-scope set and DescendantRegionNames;
//     browse/descendant direction only, never the ancestor walk.
//  3. AncestorRegions(focusID) → []Region, leaf-first, national-filtered.
//     Used only for the SPA breadcrumb (Ancestry) — never for org scope.
//  4. OrgsForRegions(focus ∪ descendants ∪ rollups) → []Org.
//  5. BucketOrgsByScope splits into Local / Regional / Statewide.
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

	// Metros that roll up to this region via rollup_states (browse/
	// descendant direction only). For a state focus this is how a
	// stateless multi-state metro's OWN orgs reach the state page; empty
	// for every non-rollup-target focus. The metro NODE only is added —
	// never its descendants — so an out-of-state leaf beneath the metro is
	// not pulled onto this page.
	rollups, err := store.RollupMetrosFor(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("atlas: rollup metros: %w", err)
	}

	ancestors, err := store.AncestorRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("atlas: ancestor regions: %w", err)
	}

	inScope := make(map[int64]Region, len(descendants)+len(rollups))
	for _, r := range descendants {
		inScope[r.ID] = r
	}
	for _, r := range rollups {
		inScope[r.ID] = r // metro NODE only — not its descendants
	}

	ids := make([]int64, 0, len(inScope))
	for k := range inScope {
		ids = append(ids, k)
	}
	orgs, err := store.OrgsForRegions(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("atlas: orgs for regions: %w", err)
	}

	local, regional, statewide := BucketOrgsByScope(inScope, orgs)

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
	// Rolled-up metros are referenced by matched_region_slugs but are not
	// in `descendants`, so their slug->name is added here too (the SPA
	// needs it to label "matched via <Metro>"). skip still excludes the
	// focus and its ancestors.
	for _, r := range rollups {
		if _, ok := skip[r.Slug]; ok {
			continue
		}
		descendantNames[r.Slug] = r.Name
	}

	return &RegionDetail{
		Region:                region,
		Local:                 local,
		Regional:              regional,
		Statewide:             statewide,
		Ancestry:              ancestry,
		DescendantRegionNames: descendantNames,
	}, nil
}
