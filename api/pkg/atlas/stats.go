package atlas

// Stats is the atlas-wide size summary behind GET /api/v1/stats. The
// totals can't be derived from ListRegions, which covers only the browse
// kinds; see docs/architecture/atlas-library.md, "Stats".
type Stats struct {
	// TotalOrgCount counts each org once, however many regions it
	// attaches to, excluding orgs whose every attachment is national-tier.
	TotalOrgCount int

	// TotalRegionCount is every non-national region in the graph, of
	// every kind.
	TotalRegionCount int

	// BrowseRegionCount is the number of regions ListRegions returns.
	BrowseRegionCount int

	// ByCountry breaks the counts down per country, sorted by code.
	ByCountry []CountryStats
}

// CountryStats is one country's slice of Stats. An org is credited to
// every country it has a non-national attachment in, so OrgCount is not
// a partition of Stats.TotalOrgCount.
type CountryStats struct {
	Country     Country
	OrgCount    int
	RegionCount int // browse regions, i.e. this country's share of BrowseRegionCount
}
