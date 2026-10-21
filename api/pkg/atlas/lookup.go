package atlas

import (
	"context"
	"fmt"
)

// Lookup returns the organizations advocating at a postal code: the orgs
// attached to its leaf region or any ancestor, bucketed by
// BucketOrgsByScope. An unknown code yields an error wrapping
// ErrPostalCodeNotFound.
func Lookup(ctx context.Context, store Store, query LookupQuery) (LookupResult, error) {
	leaf, err := store.ResolveLeafRegion(ctx, query.Country, query.PostalCode)
	if err != nil {
		// errors.Is still finds ErrPostalCodeNotFound through the wrap.
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

// placeLabel formats the ancestry as "<leaf>, <inner> — <broad>",
// dropping empty slots. The inner slot is chosen by sort_priority rather
// than scope_tier, so NYC (regional) and Berlin (local) both qualify as
// sub-state civic units.
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

// pickBroadAncestor selects placeLabel's "broad" slot: the closest
// metro-kind ancestor, else the closest regional-tier one (a transit
// federation such as Berlin's VBB), else nil.
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

// pickInnerAncestor selects placeLabel's "inner" slot: the closest
// ancestor below state tier (SortPriority < 60) other than the leaf and
// broad, e.g. NYC for a borough. broad may be nil.
func pickInnerAncestor(ancestry []Region, leaf Region, broad *Region) *Region {
	for i := 1; i < len(ancestry); i++ {
		r := ancestry[i]
		if r.Slug == leaf.Slug {
			continue
		}
		if broad != nil && r.Slug == broad.Slug {
			continue
		}
		if r.SortPriority >= 60 { // state tier and above
			continue
		}
		return &r
	}
	return nil
}
