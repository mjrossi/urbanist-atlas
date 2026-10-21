package atlas

import (
	"context"
	"errors"
)

// ErrPostalCodeNotFound is returned by Store.ResolveLeafRegion when no
// row exists for the (country, postal code) pair.
var ErrPostalCodeNotFound = errors.New("atlas: postal code not found")

// ErrOrgNotFound is returned by Store.GetOrgBySlug when no approved org
// matches the slug.
var ErrOrgNotFound = errors.New("atlas: organization not found")

// ErrRegionNotFound is returned by Store.ResolveRegionBySlug for an
// unknown slug and for a national-tier region; the wire contract does
// not distinguish the two.
var ErrRegionNotFound = errors.New("atlas: region not found")

// Store is the read seam that the orchestrators in this package
// (Lookup, GetRegion) compose. Implementations hold no business logic
// and must be safe for concurrent use.
//
// # Behavioral contracts (enforced by storetest)
//
//   - AncestorRegions and DescendantRegions exclude scope_tier='national'
//     rows from both the seed and the recursion.
//   - OrgsForRegions hydrates Org.Regions sorted ascending by region ID
//     and populates Org.AddedAt when the row carries one.
//   - ListRegions' nearest-browseable-ancestor walk resolves ties
//     (multiple browseable parents at min depth) by slug ASC.
//   - RollupMetrosFor is browse/descendant direction only: it never feeds
//     AncestorRegions or Lookup, so the rollup_states relation cannot leak
//     orgs across a postal-code lookup.
type Store interface {
	// ResolveLeafRegion returns the leaf region a raw postal code points
	// at, normalizing it with NormalizePostalCode, or ErrPostalCodeNotFound.
	ResolveLeafRegion(ctx context.Context, country Country, postalCode string) (Region, error)

	// ResolveRegionBySlug returns the region identified by slug, or
	// ErrRegionNotFound for unknown slugs and national-tier rows.
	ResolveRegionBySlug(ctx context.Context, slug string) (Region, error)

	// AncestorRegions returns the leaf followed by its transitive
	// ancestors, most-specific first, deduplicating DAG diamonds.
	AncestorRegions(ctx context.Context, leafRegionID int64) ([]Region, error)

	// DescendantRegions returns the focus region followed by every
	// descendant, deduplicating DAG diamonds.
	DescendantRegions(ctx context.Context, focusRegionID int64) ([]Region, error)

	// RollupMetrosFor returns the non-national metro nodes (not their
	// descendants) that name stateRegionID in rollup_states, or an empty
	// slice.
	RollupMetrosFor(ctx context.Context, stateRegionID int64) ([]Region, error)

	// OrgsForRegions returns the approved orgs attached to any of
	// regionIDs, each with its full Regions slice. Order across orgs is
	// unspecified.
	OrgsForRegions(ctx context.Context, regionIDs []int64) ([]Org, error)

	// ListRegions returns every non-national defaultBrowseKinds region
	// with at least one org in its subtree, ordered by OrgCount DESC then
	// Name ASC. An empty result is an empty slice, not an error.
	ListRegions(ctx context.Context) ([]RegionSummary, error)

	// SearchRegions returns non-national regions of every kind whose name
	// or slug matches query case-insensitively, ranked exact-slug >
	// exact-name > name-prefix > slug-prefix > substring, then Name, then
	// Slug. limit <= 0 selects a default and a hard maximum applies; a
	// blank query returns an empty slice.
	//
	// Each result's ContextLabel carries the nearest state/province
	// ancestor's name for disambiguation; empty when none resolves.
	SearchRegions(ctx context.Context, query string, limit int) ([]RegionSearchResult, error)

	// GetOrgBySlug returns the approved org identified by slug, with
	// Regions sorted by ID, or ErrOrgNotFound.
	GetOrgBySlug(ctx context.Context, slug string) (*Org, error)

	// ListRecent returns the 10 newest orgs, excluding orgs whose only
	// attachments are national-tier. The cap is fixed by openapi.yaml.
	ListRecent(ctx context.Context) ([]Org, error)

	// Stats returns the atlas-wide counts, excluding orgs whose only
	// attachments are national-tier. Use it rather than summing over
	// ListRegions, which covers only the browse kinds; see atlas.Stats.
	Stats(ctx context.Context) (Stats, error)
}
