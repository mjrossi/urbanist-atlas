// Package atlas is the importable core of Urbanist Atlas: the data model
// and the postal-code → organizations lookup algorithm. It deliberately
// has no transport or persistence dependencies — those live in the
// internal/httpapi and internal/store packages, which compose atlas.
//
// The public types and the Lookup function are stable contracts. The
// MemStore implementation is suitable for tests, fixtures, and CLI use;
// a Postgres-backed Store lives in internal/store/postgres.
package atlas

// Country is the ISO-style country code used everywhere in the atlas.
// Only US and CA are supported in v1.
type Country string

const (
	CountryUS Country = "US"
	CountryCA Country = "CA"
)

// ScopeTier drives result grouping in Lookup. Regions classified as
// "local" cause their orgs to appear in the Local bucket; "regional"
// regions cause their orgs to appear in the Regional bucket.
type ScopeTier string

const (
	ScopeLocal    ScopeTier = "local"
	ScopeRegional ScopeTier = "regional"
)

// RegionKind is the granularity of a region. "multi-state" is for
// regions like the NY/NJ/CT tri-state area.
type RegionKind string

const (
	RegionCity       RegionKind = "city"
	RegionCounty     RegionKind = "county"
	RegionMetro      RegionKind = "metro"
	RegionState      RegionKind = "state"
	RegionProvince   RegionKind = "province"
	RegionCountry    RegionKind = "country"
	RegionMultiState RegionKind = "multi-state"
)

// Tag is an open-ended label on an organization. The canonical set is
// documented in CLAUDE.md (transit, safe-streets, cycling, walking,
// vision-zero, policy, grassroots, political, neighborhood); the type
// is left as a string so the seed data can introduce new labels
// without a code change.
type Tag string

// Region is a geographic unit an organization can serve.
type Region struct {
	ID        int64      `json:"id"`
	Kind      RegionKind `json:"kind"`
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	Country   Country    `json:"country"`
	ScopeTier ScopeTier  `json:"scope_tier"`
}

// Org is a single advocacy organization. Regions is denormalized onto
// the org for ergonomic JSON output — Store implementations populate
// it when returning results.
type Org struct {
	ID         int64    `json:"id"`
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	ShortDesc  string   `json:"short_desc"`
	WebsiteURL string   `json:"website_url"`
	ContactURL string   `json:"contact_url,omitempty"`
	Tags       []Tag    `json:"tags"`
	Regions    []Region `json:"regions"`
}

// ResolvedPostalCode is the result of looking up a single postal code:
// the geographic regions it falls within. Any of the region pointers
// may be nil (e.g. Canadian postal codes have no county; some
// US ZIPs have no metro).
type ResolvedPostalCode struct {
	Code    string  `json:"code"`
	Country Country `json:"country"`
	City    *Region `json:"city,omitempty"`
	County  *Region `json:"county,omitempty"`
	Metro   *Region `json:"metro,omitempty"`
	State   *Region `json:"state,omitempty"` // also holds the province for CA
}

// RegionIDs returns the non-nil region IDs in city → county → metro →
// state order.
func (r ResolvedPostalCode) RegionIDs() []int64 {
	ids := make([]int64, 0, 4)
	if r.City != nil {
		ids = append(ids, r.City.ID)
	}
	if r.County != nil {
		ids = append(ids, r.County.ID)
	}
	if r.Metro != nil {
		ids = append(ids, r.Metro.ID)
	}
	if r.State != nil {
		ids = append(ids, r.State.ID)
	}
	return ids
}

// LookupQuery is the input to Lookup.
type LookupQuery struct {
	PostalCode string  `json:"postal_code"`
	Country    Country `json:"country"`
}

// LookupResult is what the API returns. Local and Regional are always
// non-nil slices (possibly empty); see Lookup for the bucketing rules.
type LookupResult struct {
	Query              LookupQuery `json:"query"`
	ResolvedPlaceLabel string      `json:"resolved_place_label"`
	Local              []Org       `json:"local"`
	Regional           []Org       `json:"regional"`
}
