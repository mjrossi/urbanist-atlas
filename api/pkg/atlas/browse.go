package atlas

// RegionSummary is the domain shape returned by Store.ListRegions:
// one region (any non-national kind) plus its approved-org counts.
//
// Two counts are exposed. OrgCount is the descendant-walk count
// (orgs attached to the region OR any descendant in the DAG) —
// suitable for per-row "how much coverage does this region have"
// displays. DirectOrgCount is restricted to orgs attached directly
// to the region, with no descendant walk; SPA totals that sum
// across rows use this to avoid double-counting orgs that surface
// under both a metro and one of its child cities.
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
	DirectOrgCount   int64
	BrowseParentSlug string
}

// RegionDetail is the domain shape returned by atlas.GetRegion: a
// region plus the orgs in scope for it, bucketed by attachment
// scope_tier, plus the upward ancestry walk and a descendant
// slug→name lookup used by the SPA.
//
// "In scope" means orgs attached to the region itself or any DAG
// descendant — a metro surfaces its constituent cities' orgs, a
// state surfaces every metro/city beneath it. Ancestor orgs are NOT
// pulled in: this is the "what does this region contain?" question,
// not the "what works at this address?" question (the latter is
// Lookup's job, with its upward ancestor walk). Keeping the in-scope
// set symmetric with ListRegions' descendant-walk org_count means
// the count on the browse card matches the count on the detail page.
//
// Local + Regional buckets are decided by the scope_tier of the org's
// matched attachment regions — same rule Lookup uses. National-tier
// attachments are always filtered.
//
// Ancestry is ordered closest-first (direct parent at index 0, then
// grandparent, …) and excludes the region itself plus any
// national-tier rows. It feeds the SPA's breadcrumb only — it does
// not affect org scope.
//
// DescendantRegionNames maps descendant region slugs (referenced via
// matched_region_slugs on the bucketed orgs) to their display names.
// Excludes the focus region's own slug and ancestry slugs (the SPA
// already has names for those). Empty map (not nil) when no
// descendants need resolving — the JSON contract is `{}`, never
// `null`.
type RegionDetail struct {
	Region                Region
	Local                 []Org
	Regional              []Org
	Ancestry              []Region
	DescendantRegionNames map[string]string
}
