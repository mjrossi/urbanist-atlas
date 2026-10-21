package atlas

import (
	"context"
	"errors"
	"fmt"
)

// GetRegion returns the RegionDetail for slug, or (nil, nil) when the
// slug is unknown or national-tier. Where Lookup walks up from a postal
// code, GetRegion walks down: the in-scope set is the region, its
// descendants, and the metro nodes that roll up to it via rollup_states.
// See docs/architecture/atlas-library.md, "Lookup and GetRegion".
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

	// Metro nodes only, never their descendants, so an out-of-state leaf
	// under a stateless metro stays off a state's page.
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

	local, regional, statewide := BucketOrgsByScope(inScope, orgs)

	// AncestorRegions puts the focus at [0]. Non-nil so the wire shape is
	// `[]`, not `null`, for a top-level region.
	ancestry := make([]Region, 0)
	if len(ancestors) > 1 {
		ancestry = append(ancestry, ancestors[1:]...)
	}

	// The focus and its ancestors are skipped because the response
	// already names them. Non-nil so the JSON is `{}`, not `null`.
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
	// Rolled-up metros can be matched regions without being
	// descendants, so they need names too.
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
