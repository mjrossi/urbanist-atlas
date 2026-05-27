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

// ErrOrgNotFound is returned by Store.GetOrgBySlug when no approved org
// matches the slug. The HTTP layer maps this to a 404 problem document.
var ErrOrgNotFound = errors.New("atlas: organization not found")

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

	// ListRegions returns every region in the default browse set
	// (see defaultBrowseKinds in browse_kinds.go — metros + cities)
	// that has at least one approved organization attached to it
	// (directly or via the region DAG), with the org count. Ordered
	// by OrgCount DESC, Region.Name ASC. Excludes national-tier
	// regions. An empty result is a non-error empty slice, not an
	// error.
	//
	// The list endpoint deliberately ships without a kind filter;
	// the right filter axis (taxonomy vs DAG-ancestor vs scope-tier)
	// will be designed when a concrete browse UI use case appears.
	ListRegions(ctx context.Context) ([]RegionSummary, error)

	// GetRegion returns the region identified by slug, plus the
	// approved orgs that serve it (directly or via the region DAG).
	// Resolves any non-national region — metros, cities, counties,
	// boroughs, states, multi-state coalitions. Returns (nil, nil)
	// when the slug is unknown or names a national-tier region; the
	// handler maps the nil pointer to 404.
	GetRegion(ctx context.Context, slug string) (*RegionDetail, error)

	// GetOrgBySlug returns the approved organization identified by slug,
	// with every region it serves denormalized at Org.Regions. Returns
	// ErrOrgNotFound when no row matches — the handler maps that to a
	// 404 problem document.
	GetOrgBySlug(ctx context.Context, slug string) (*Org, error)

	// ListRecent returns the 10 most-recently-approved organizations
	// across the whole atlas, ordered newest-first. Organizations
	// whose ONLY region attachments are scope_tier='national' are
	// excluded (consistent with the default /lookup filter from slice
	// #4.6). The 10-row cap is hardcoded; opening it would require an
	// OpenAPI spec edit.
	ListRecent(ctx context.Context) ([]Org, error)
}
