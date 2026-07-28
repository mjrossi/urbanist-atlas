package atlas

// Stats is the atlas-wide size summary behind GET /api/v1/stats — the
// numbers the SPA prints in its masthead and its "by the numbers"
// panel.
//
// It exists because those numbers CANNOT be derived from ListRegions.
// That endpoint deliberately returns only defaultBrowseKinds (metros
// and cities), so summing RegionSummary.DirectOrgCount over its result
// silently drops every org attached solely to a state, province,
// borough, or multi-state coalition. The frontend did exactly that and
// under-reported the catalog by 30%. Counting here — over the store's
// own org list rather than over a presentational subset — is immune to
// that class of bug by construction.
type Stats struct {
	// TotalOrgCount is the number of DISTINCT organizations in the
	// atlas, counted once each regardless of how many regions they
	// attach to. Orgs whose every attachment is scope_tier='national'
	// are excluded, matching the default /lookup, /recent, /regions,
	// and /regions/search filters.
	TotalOrgCount int

	// TotalRegionCount is every non-national region in the graph —
	// states, counties, metros, MSA portions, cities, boroughs, the
	// lot. Much larger than BrowseRegionCount.
	TotalRegionCount int

	// BrowseRegionCount is the number of regions ListRegions returns:
	// browse-kind regions carrying at least one org in their subtree.
	// It is computed from ListRegions' own result, so the two can
	// never disagree.
	BrowseRegionCount int

	// ByCountry breaks the counts down per country, sorted by country
	// code ASC for a deterministic wire order.
	ByCountry []CountryStats
}

// CountryStats is one country's slice of Stats.
//
// Attribution note: an org is credited to every country it has a
// non-national attachment in. An org spanning the border would
// therefore appear in two rows, making the OrgCount column sum to
// more than Stats.TotalOrgCount. No such org exists in the v1 seed,
// but the wire contract permits it — consumers must not treat the
// column as a partition.
type CountryStats struct {
	Country     Country
	OrgCount    int
	RegionCount int // browse regions, i.e. this country's share of BrowseRegionCount
}
