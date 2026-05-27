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
// region plus the orgs in scope for it, bucketed by attachment
// scope_tier, plus the upward ancestry walk used to render a
// breadcrumb in the SPA.
//
// "In scope" means orgs attached to the region itself, any
// descendant (so a metro surfaces its constituent cities' orgs), or
// any ancestor (so a city surfaces orgs covering its parent metro /
// state / multi-state region). Local + Regional buckets are decided
// by the scope_tier of the org's matched attachment regions — same
// rule Lookup uses. National-tier attachments are always filtered.
//
// Ancestry is ordered closest-first (direct parent at index 0, then
// grandparent, …) and excludes the region itself plus any
// national-tier rows.
type RegionDetail struct {
	Region   Region
	Local    []Org
	Regional []Org
	Ancestry []Region
}
