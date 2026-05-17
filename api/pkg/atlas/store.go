package atlas

import (
	"context"
	"errors"
)

// ErrPostalCodeNotFound is returned by Store.ResolveLeafRegion when no
// row exists for the (country, postal code) pair. The HTTP layer maps
// this to a 404 with a helpful problem document so the SPA can suggest
// a nearby code or a submission.
var ErrPostalCodeNotFound = errors.New("atlas: postal code not found")

// Store is the persistence seam between pkg/atlas and the rest of the
// system. Three operations compose to satisfy Lookup; Postgres-backed
// implementations can optimize internally (e.g. fold AncestorRegions
// + OrgsForRegions into a single CTE) without changing the contract.
//
// All implementations must be safe for concurrent use.
type Store interface {
	// ResolveLeafRegion returns the leaf region a postal code points at.
	// The code argument should be the user's raw input; implementations
	// normalize via NormalizePostalCode before querying. Returns
	// ErrPostalCodeNotFound if no match exists.
	ResolveLeafRegion(ctx context.Context, country Country, postalCode string) (Region, error)

	// AncestorRegions returns the leaf region followed by all transitive
	// ancestors in the region graph, ordered most-specific first
	// (leaf, then immediate parents, then their parents, etc.).
	// Includes the leaf itself; deduplicates DAG diamonds.
	AncestorRegions(ctx context.Context, leafRegionID int64) ([]Region, error)

	// OrgsForRegions returns all approved organizations attached to any
	// of the given region IDs. Each returned Org has its full Regions
	// slice populated (every region the org serves, not just the ones
	// that matched). Order is unspecified — Lookup buckets and sorts.
	OrgsForRegions(ctx context.Context, regionIDs []int64) ([]Org, error)
}
