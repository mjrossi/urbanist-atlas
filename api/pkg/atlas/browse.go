package atlas

// RegionSummary is one Store.ListRegions row. OrgCount counts orgs
// attached to the region or any descendant; DirectOrgCount counts only
// orgs attached to the region itself, so summing it across rows doesn't
// count an org under both a metro and its city. BrowseParentSlug is the
// nearest browse-kind ancestor's slug, or "" when there is none.
type RegionSummary struct {
	Region           Region
	OrgCount         int64
	DirectOrgCount   int64
	BrowseParentSlug string
}

// RegionSearchResult is one Store.SearchRegions match. ContextLabel is
// the name of the region's nearest state/province-equivalent ancestor
// (e.g. "New York" for the `queens` borough), so a type-ahead can tell
// same-named leaves apart ("Springfield" in IL vs MA). With no state
// ancestor it is the slug-first direct non-national parent's name, and
// empty only when there is no such parent.
type RegionSearchResult struct {
	Region       Region
	ContextLabel string
}

// RegionDetail is what GetRegion returns: the region, its in-scope orgs
// bucketed by BucketOrgsByScope, its non-national ancestors
// closest-first (for a breadcrumb only), and DescendantRegionNames, a
// never-nil slug→name map of the in-scope regions other than the focus
// and its ancestors.
type RegionDetail struct {
	Region                Region
	Local                 []Org
	Regional              []Org
	Statewide             []Org
	Ancestry              []Region
	DescendantRegionNames map[string]string
}
