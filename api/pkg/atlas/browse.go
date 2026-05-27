package atlas

// RegionSummary is the domain shape returned by Store.ListRegions:
// one region (any non-national kind) and the number of approved
// organizations that serve it (directly or via the region DAG).
//
// BrowseParentSlug carries the slug of the nearest ancestor whose
// kind is also in the default browse set — the SPA's grouping hook
// for nesting cities under their parent metro. Empty string when no
// such ancestor exists (typical for metros), which maps to JSON
// null on the wire.
//
// The wire-level twin lives in the generated oapi package; the HTTP
// handler adapts one to the other so pkg/atlas stays free of
// OpenAPI types.
type RegionSummary struct {
	Region           Region
	OrgCount         int64
	BrowseParentSlug string
}

// RegionDetail is the domain shape returned by Store.GetRegion: a
// region plus the approved orgs that serve it (directly or via the
// region DAG) and the upward ancestry walk used to render a
// breadcrumb in the SPA.
//
// Ancestry is ordered closest-first (direct parent at index 0, then
// grandparent, …) and excludes the region itself plus any
// scope_tier='national' rows. Orgs is newest-first when the
// underlying store has a creation timestamp; the MemStore tolerates
// the field being zero.
type RegionDetail struct {
	Region   Region
	Orgs     []Org
	Ancestry []Region
}
