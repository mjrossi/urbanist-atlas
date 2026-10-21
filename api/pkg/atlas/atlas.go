// Package atlas is the importable core of Urbanist Atlas: the data model
// and the postal-code → organizations lookup algorithm. It deliberately
// has no transport or persistence dependencies, so a CLI can consume it
// as directly as the HTTP server does.
//
// The public types and the Lookup function are stable contracts.
// MemStore backs the runtime read path (the seed bundle is loaded into
// one at boot) as well as tests, fixtures, and CLI tooling. Design
// notes: docs/architecture/atlas-library.md.
package atlas

import "time"

// Country is the ISO-style country code used throughout the atlas. It's
// an opaque string so a new country is added by loading data, not by
// editing this file.
type Country string

const (
	CountryUS Country = "US"
	CountryCA Country = "CA"
)

// ScopeTier is a region's editorial tier, which drives result grouping
// (see BucketOrgsByScope). National-tier regions exist so country-wide
// umbrella orgs can be modeled without surfacing in the local-first
// defaults; see docs/region-graph.md §5.
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

// Tag is an open-ended label on an organization. It is a string so
// seed data can introduce a label without a code change; the canonical
// set is listed on Org.tags in openapi.yaml.
type Tag string

// Region is a geographic unit an organization can serve. Regions form a
// directed acyclic graph; ParentSlugs lists the direct parents (not
// transitive). SortPriority orders orgs within a result bucket (lower =
// more specific = earlier).
//
// RollupStates names the rollup targets (IsRollupTargetKind) on whose
// detail pages this region's own orgs also surface. It is not a graph edge, so it
// can't leak orgs into a /lookup; see docs/architecture/atlas-library.md,
// "rollup_states".
//
// The TOML tags name the seed-file shape (regions_<cc>.toml); ID and
// Country are stamped by the loader, hence `toml:"-"`.
type Region struct {
	ID           int64      `json:"id" toml:"-"`
	Kind         RegionKind `json:"kind" toml:"kind"`
	Name         string     `json:"name" toml:"name"`
	Slug         string     `json:"slug" toml:"slug"`
	Country      Country    `json:"country" toml:"-"`
	ScopeTier    ScopeTier  `json:"scope_tier" toml:"scope_tier"`
	ParentSlugs  []string   `json:"parent_slugs" toml:"parents"`
	SortPriority int        `json:"-" toml:"sort_priority"` // server-side only, not on the wire
	RollupStates []string   `json:"-" toml:"rollup_states"` // server-side only
}

// IsNational reports whether the region sits in the national scope
// tier. Such regions are left out of the ancestor and descendant walks,
// browse, region resolution, search, and ListRecent.
func (r Region) IsNational() bool { return r.ScopeTier == ScopeNational }

// Org is a single advocacy organization. Regions holds every region the
// org serves; MatchedRegionSlugs, set by BucketOrgsByScope, names the
// subset that made it surface for a query.
//
// AddedAt is the date the org joined the atlas, at midnight UTC; it
// orders ListRecent. The TOML tags name the orgs.toml shape; fields
// marked `toml:"-"` are filled in by the loader or at query time.
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

// LookupResult is Lookup's answer. Local, Regional, and Statewide are
// never nil; ResolvedAncestry is the leaf followed by its ancestors,
// most-specific first.
type LookupResult struct {
	Query              LookupQuery `json:"query"`
	ResolvedPlaceLabel string      `json:"resolved_place_label"`
	ResolvedAncestry   []Region    `json:"resolved_ancestry"`
	Local              []Org       `json:"local"`
	Regional           []Org       `json:"regional"`
	Statewide          []Org       `json:"statewide"`
}
