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
// "In scope" matches Lookup's rule, extended to the whole subtree under
// the focus: orgs attached to the focus itself, any descendant (so a
// metro surfaces its constituent cities' orgs), or any ancestor (so a
// city surfaces orgs covering its parent metro / state / multi-state
// region). Local + Regional buckets are decided by the scope_tier of the
// org's matched attachment regions — same rule Lookup uses.
//
// Counterpart to Lookup: same composition shape, same primitives,
// different scope rule (Lookup walks up only; GetRegion walks both ways).
// Living in pkg/atlas keeps the algorithm in one place — Store
// implementations only own data access.
//
// Algorithm:
//  1. ResolveRegionBySlug(slug) → focus Region. Returns ErrRegionNotFound
//     for unknown slugs and for national-tier rows. Callers map both to
//     404 — the wire contract makes no distinction.
//  2. AncestorRegions(focusID) → []Region, leaf-first, national-filtered.
//  3. DescendantRegions(focusID) → []Region, root-first, national-filtered.
//  4. OrgsForRegions(union of all in-scope region IDs) → []Org.
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

	ancestors, err := store.AncestorRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("atlas: ancestor regions: %w", err)
	}

	descendants, err := store.DescendantRegions(ctx, region.ID)
	if err != nil {
		return nil, fmt.Errorf("atlas: descendant regions: %w", err)
	}

	inScope := make(map[int64]Region, len(ancestors)+len(descendants))
	for _, r := range ancestors {
		inScope[r.ID] = r
	}
	for _, r := range descendants {
		if _, ok := inScope[r.ID]; ok {
			continue
		}
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
