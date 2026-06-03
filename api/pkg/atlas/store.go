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

// ErrRegionNotFound is returned by Store.ResolveRegionBySlug when no
// row matches the slug OR the row names a scope_tier='national' region
// (the v1 editorial gate keeps national-tier content out of browse
// contexts). The HTTP layer maps both to 404 — the wire contract makes
// no distinction between "unknown" and "national".
var ErrRegionNotFound = errors.New("atlas: region not found")

// Store is the persistence seam between pkg/atlas and the rest of the
// system. Higher-level orchestrators in pkg/atlas (Lookup, GetRegion)
// compose Store primitives; implementations stay free of business
// logic. The package-level storetest harness exercises every contract
// below against MemStore so its behavior can't drift quietly from the
// documented contracts. (The seam stays implementation-agnostic so a
// future downstream-backed Store can be slotted in behind the same
// suite.)
//
// All implementations must be safe for concurrent use.
//
// # Behavioral contracts (enforced by storetest)
//
//   - AncestorRegions and DescendantRegions exclude scope_tier='national'
//     rows from both the seed and the recursion.
//   - ResolveRegionBySlug returns ErrRegionNotFound for unknown slugs
//     and for slugs that name a national-tier region.
//   - OrgsForRegions hydrates Org.Regions sorted ascending by region ID
//     and populates Org.AddedAt when the row carries one.
//   - ListRegions' nearest-browseable-ancestor walk resolves ties
//     (multiple browseable parents at min depth) by slug ASC.
//   - RollupMetrosFor is browse/descendant direction only: it never feeds
//     AncestorRegions or Lookup, so the rollup_states relation cannot leak
//     orgs across a postal-code lookup.
type Store interface {
	// ResolveLeafRegion returns the leaf region a postal code points at.
	// The code argument should be the user's raw input; implementations
	// normalize via NormalizePostalCode before querying. Returns
	// ErrPostalCodeNotFound if no match exists.
	ResolveLeafRegion(ctx context.Context, country Country, postalCode string) (Region, error)

	// ResolveRegionBySlug returns the region identified by slug.
	// Returns ErrRegionNotFound for unknown slugs and for national-tier
	// rows. Used by the GetRegion orchestrator as the entry point to
	// any /regions/{slug} call.
	ResolveRegionBySlug(ctx context.Context, slug string) (Region, error)

	// AncestorRegions returns the leaf region followed by all transitive
	// ancestors in the region graph, ordered most-specific first
	// (leaf, then immediate parents, then their parents, etc.).
	// Includes the leaf itself; deduplicates DAG diamonds; excludes
	// scope_tier='national' rows from both the seed and the recursion.
	AncestorRegions(ctx context.Context, leafRegionID int64) ([]Region, error)

	// DescendantRegions returns the focus region followed by every
	// descendant reachable by walking region_parents in the
	// parent->child direction. Symmetric to AncestorRegions: includes
	// the focus at index 0, deduplicates DAG diamonds, and excludes
	// scope_tier='national' rows from both the seed and the recursion.
	DescendantRegions(ctx context.Context, focusRegionID int64) ([]Region, error)

	// RollupMetrosFor returns the metro NODES (not their descendants)
	// whose OWN orgs should additionally surface on the given
	// state-equivalent region's detail page — the directional
	// rollup_states relation. Returns an empty slice for a region that is
	// not a rollup target. National-tier metros are excluded. This
	// relation is browse/descendant direction ONLY; it is never consulted
	// by AncestorRegions, so a leaf under the metro cannot leak the state
	// via the ancestor walk.
	RollupMetrosFor(ctx context.Context, stateRegionID int64) ([]Region, error)

	// OrgsForRegions returns all approved organizations attached to any
	// of the given region IDs. Each returned Org has its full Regions
	// slice populated (every region the org serves, not just the ones
	// that matched), hydrated sorted ascending by region ID, with
	// AddedAt populated when the storage layer carries one. Order
	// across orgs is unspecified — callers bucket and sort.
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
	//
	// Each RegionSummary.BrowseParentSlug carries the slug of the
	// nearest browseable-kind ancestor. Ties at min depth are
	// resolved by slug ASC so the choice is deterministic.
	ListRegions(ctx context.Context) ([]RegionSummary, error)

	// SearchRegions returns regions whose name or slug matches query
	// (case-insensitive), for type-ahead use. Unlike ListRegions it
	// searches the FULL graph (every kind — boroughs, counties, metros,
	// states), not just the browseable, org-bearing subset, so a
	// submitter can attach an org to any node. Excludes
	// scope_tier='national' rows. Ranked exact-slug > exact-name >
	// name-prefix > slug-prefix > substring, with Name ASC then Slug ASC
	// as the stable tiebreak. Capped at limit (<=0 selects a default;
	// the implementation applies a hard maximum). A blank query returns
	// an empty slice, not an error.
	//
	// Each result's ContextLabel carries the nearest state/province
	// ancestor's name for disambiguation; empty when none resolves.
	SearchRegions(ctx context.Context, query string, limit int) ([]RegionSearchResult, error)

	// GetOrgBySlug returns the approved organization identified by slug,
	// with every region it serves denormalized at Org.Regions (sorted
	// ascending by region ID). Returns ErrOrgNotFound when no row
	// matches — the handler maps that to a 404 problem document.
	GetOrgBySlug(ctx context.Context, slug string) (*Org, error)

	// ListRecent returns the 10 most-recently-approved organizations
	// across the whole atlas, ordered newest-first. Organizations
	// whose ONLY region attachments are scope_tier='national' are
	// excluded (consistent with the default /lookup filter from slice
	// #4.6). The 10-row cap is hardcoded; opening it would require an
	// OpenAPI spec edit.
	ListRecent(ctx context.Context) ([]Org, error)
}
