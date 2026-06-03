// Package atlas is the importable core of Urbanist Atlas: the data model
// and the postal-code → organizations lookup algorithm. It deliberately
// has no transport or persistence dependencies — those live in the
// internal/httpapi and internal/store packages, which compose atlas.
//
// The public types and the Lookup function are stable contracts. The
// MemStore implementation backs the runtime read path (the seed bundle
// is loaded into a MemStore at boot) and is also used directly for
// tests, fixtures, and CLI tooling.
package atlas

import "time"

// Country is the ISO-style country code used throughout the atlas. It's
// an opaque string; values come from seed data, not from a closed set.
// US and CA are the v1 anchors; DE/FR/UK/AU/etc. are added by loading
// data, not by editing this file.
type Country string

const (
	CountryUS Country = "US"
	CountryCA Country = "CA"
)

// ScopeTier drives result grouping in Lookup. Regions classified as
// "local" cause their orgs to appear in the Local bucket; "regional"
// regions cause their orgs to appear in the Regional bucket;
// "national" regions are filtered from the default Lookup ancestor
// walk (see MemStore.AncestorRegions) so their orgs don't surface in
// default results. The national tier exists so that
// national-scope advocacy orgs (e.g. MUBi for PT, Living Streets for
// UK, Fietsersbond for NL) can be modeled without distorting the
// local-first defaults; surfacing them is a future opt-in.
type ScopeTier string

const (
	ScopeLocal    ScopeTier = "local"
	ScopeRegional ScopeTier = "regional"
	ScopeNational ScopeTier = "national"
)

// RegionKind is the granularity of a region. It's an opaque string;
// the recommended vocabulary is country-prefixed (`us:city`, `de:land`,
// `fr:metropole`, …) and is documented in docs/region-graph.md.
type RegionKind string

// Tag is an open-ended label on an organization. The canonical set is
// documented in CLAUDE.md (transit, safe-streets, cycling, walking,
// vision-zero, policy, grassroots, political, neighborhood); the type
// is left as a string so the seed data can introduce new labels
// without a code change.
type Tag string

// Region is a geographic unit an organization can serve. Regions form a
// directed acyclic graph; ParentSlugs lists the direct parents (not
// transitive). SortPriority is a server-side hint used by Lookup to
// order orgs within the Regional bucket (lower = more specific = earlier).
//
// RollupStates is a directional, server-side-only association
// (json:"-", like SortPriority): the state-equivalent region slugs on
// whose detail/browse pages this region's OWN orgs should additionally
// surface (in the Regional bucket), in the descendant/browse direction
// ONLY. Unlike ParentSlugs it is NOT a graph edge — it is never added to
// the parent map, cycle detection, or the ancestor walk — so it cannot
// leak orgs across a /lookup. It lets a stateless multi-state metro
// (e.g. chicago-metro) surface its orgs on its constituent states'
// pages without the cross-state ancestor leak docs/region-graph.md §1
// forbids. Resolved to a state->metros index at load (seedfiles).
//
// The TOML tags name the seed-file shape (regions_<cc>.toml). ID and
// Country are stamped by the seedfiles loader after parsing and
// carry `toml:"-"`. The TOML field for parents is `parents` (a
// historical name; the Go field is ParentSlugs to disambiguate from
// the full Region structs that would otherwise be implied).
type Region struct {
	ID           int64      `json:"id" toml:"-"`
	Kind         RegionKind `json:"kind" toml:"kind"`
	Name         string     `json:"name" toml:"name"`
	Slug         string     `json:"slug" toml:"slug"`
	Country      Country    `json:"country" toml:"-"`
	ScopeTier    ScopeTier  `json:"scope_tier" toml:"scope_tier"`
	ParentSlugs  []string   `json:"parent_slugs" toml:"parents"`
	SortPriority int        `json:"-" toml:"sort_priority"` // server-side only, not on the wire
	RollupStates []string   `json:"-" toml:"rollup_states"` // server-side only; directional metro→state page rollup (see above)
}

// Org is a single advocacy organization. Regions is denormalized onto
// the org for ergonomic JSON output — Store implementations populate
// it with every region the org serves (not just the ones that matched).
// MatchedRegionSlugs is populated only by Lookup and identifies the
// subset of Regions that caused the org to surface for that lookup.
//
// AddedAt is the date the org was added to the atlas (held at midnight
// UTC), sourced from the required `added_at` field in orgs.toml. It
// powers newest-first ordering in Store.ListRecent and
// Store.OrgsForRegions. It keeps `json:"-"`: the wire exposes the date
// through the generated oapi.Org type (toOAPIOrg maps it to an
// openapi_types.Date), so this internal struct is never serialized
// directly.
//
// The TOML tags name the seed-file shape (orgs.toml). Fields that the
// seedfiles loader stamps after parsing (ID, Regions hydration,
// AddedAt) or that are server-side-only (MatchedRegionSlugs) carry
// `toml:"-"`. The TOML schema has `region_slugs` and `added_at` fields
// that aren't decoded onto this type directly; seedfiles unmarshals
// them onto an OrgEntry wrapper that embeds this struct, then resolves
// region_slugs to IDs and copies added_at over at load time.
type Org struct {
	ID                 int64     `json:"id" toml:"-"`
	Slug               string    `json:"slug" toml:"slug"`
	Name               string    `json:"name" toml:"name"`
	ShortDesc          string    `json:"short_desc" toml:"short_desc"`
	WebsiteURL         string    `json:"website_url" toml:"website_url"`
	ContactURL         string    `json:"contact_url,omitempty" toml:"contact_url,omitempty"`
	Tags               []Tag     `json:"tags" toml:"tags"`
	Regions            []Region  `json:"regions" toml:"-"`
	MatchedRegionSlugs []string  `json:"matched_region_slugs,omitempty" toml:"-"`
	AddedAt            time.Time `json:"-" toml:"-"`
}

// LookupQuery is the input to Lookup.
type LookupQuery struct {
	PostalCode string  `json:"postal_code"`
	Country    Country `json:"country"`
}

// LookupResult is what the API returns. Local, Regional, and Statewide
// are always non-nil slices (possibly empty); see Lookup and
// BucketOrgsByScope for the bucketing rules. ResolvedAncestry is the
// leaf region followed by all ancestors, ordered most-specific first,
// so the client can render breadcrumbs.
type LookupResult struct {
	Query              LookupQuery `json:"query"`
	ResolvedPlaceLabel string      `json:"resolved_place_label"`
	ResolvedAncestry   []Region    `json:"resolved_ancestry"`
	Local              []Org       `json:"local"`
	Regional           []Org       `json:"regional"`
	Statewide          []Org       `json:"statewide"`
}
