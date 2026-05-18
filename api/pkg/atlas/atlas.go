// Package atlas is the importable core of Urbanist Atlas: the data model
// and the postal-code → organizations lookup algorithm. It deliberately
// has no transport or persistence dependencies — those live in the
// internal/httpapi and internal/store packages, which compose atlas.
//
// The public types and the Lookup function are stable contracts. The
// MemStore implementation is suitable for tests, fixtures, and CLI use;
// a Postgres-backed Store lives in internal/store/postgres.
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
// walk (see internal/store/postgres/queries/lookup.sql) so their orgs
// don't surface in default results. The national tier exists so that
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
type Region struct {
	ID           int64      `json:"id"`
	Kind         RegionKind `json:"kind"`
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	Country      Country    `json:"country"`
	ScopeTier    ScopeTier  `json:"scope_tier"`
	ParentSlugs  []string   `json:"parent_slugs"`
	SortPriority int        `json:"-"` // server-side only, not on the wire
}

// Org is a single advocacy organization. Regions is denormalized onto
// the org for ergonomic JSON output — Store implementations populate
// it with every region the org serves (not just the ones that matched).
// MatchedRegionSlugs is populated only by Lookup and identifies the
// subset of Regions that caused the org to surface for that lookup.
//
// CreatedAt is server-side only (`json:"-"`) and powers newest-first
// ordering in Store.ListRecent and Store.GetMetro. The wire contract
// in api/openapi.yaml does not include it; a future spec addition can
// expose it without changing the data model.
type Org struct {
	ID                 int64     `json:"id"`
	Slug               string    `json:"slug"`
	Name               string    `json:"name"`
	ShortDesc          string    `json:"short_desc"`
	WebsiteURL         string    `json:"website_url"`
	ContactURL         string    `json:"contact_url,omitempty"`
	Tags               []Tag     `json:"tags"`
	Regions            []Region  `json:"regions"`
	MatchedRegionSlugs []string  `json:"matched_region_slugs,omitempty"`
	CreatedAt          time.Time `json:"-"`
}

// LookupQuery is the input to Lookup.
type LookupQuery struct {
	PostalCode string  `json:"postal_code"`
	Country    Country `json:"country"`
}

// LookupResult is what the API returns. Local and Regional are always
// non-nil slices (possibly empty); see Lookup for the bucketing rules.
// ResolvedAncestry is the leaf region followed by all ancestors,
// ordered most-specific first, so the client can render breadcrumbs.
type LookupResult struct {
	Query              LookupQuery `json:"query"`
	ResolvedPlaceLabel string      `json:"resolved_place_label"`
	ResolvedAncestry   []Region    `json:"resolved_ancestry"`
	Local              []Org       `json:"local"`
	Regional           []Org       `json:"regional"`
}
