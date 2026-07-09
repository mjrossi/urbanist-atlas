package atlas

// defaultBrowseKinds names the region kinds the `/api/v1/regions`
// list endpoint returns by default — and today, in the only mode it
// ships. The editorial default for the homepage Browse panel: metros
// and cities, the granularities a user wandering the directory
// expects to see.
//
// The set is editorial, not derived from a suffix pattern, because
// the administrative geographies that qualify don't share a stable
// lexical suffix.
//
// This is a superset of metroKinds in metro_kinds.go — the /lookup
// label logic needs the narrower "metro-equivalent" predicate so a
// Brooklyn ZIP picks "New York Metro" as the broad-ancestor slot
// rather than "New York City". The two predicates intentionally
// coexist.
//
// In:
//   - us:metro              — MSA/CSA
//   - us:city               — US city leaves (NYC, Chicago, SF, Seattle, Boston…)
//   - ca:cma                — Census Metropolitan Area
//   - ca:regional-district  — BC's multi-municipal layer that plays a
//     metro role (e.g. Metro Vancouver)
//   - ca:city               — Canadian city leaves (Toronto, Montréal…)
//   - pt:area-metropolitana — AML, AMP
//
// Out (still queryable via an explicit `?kind=` parameter):
// states/provinces, multi-state regions, counties, boroughs,
// distritos, NUTS regions, autonomous communities, national tier.
//
// Inclusion in a response is also gated by the underlying queries on
// having ≥1 approved org attachment (directly or via the region DAG);
// regions with no attachments stay off Browse and surface organically
// when an org gets tagged to them.
//
// Adding a new country's default-browse kind is a one-line append
// here; the predicate stays unchanged. The per-country editorial
// conventions live in docs/region-graph.md.
var defaultBrowseKinds = map[RegionKind]bool{
	"us:metro":              true,
	"us:city":               true,
	"ca:cma":                true,
	"ca:regional-district":  true,
	"ca:city":               true,
	"pt:area-metropolitana": true,
}

// IsDefaultBrowseKind reports whether k is one of the kinds returned
// by `/api/v1/regions`. The unknown empty string returns false.
func IsDefaultBrowseKind(k RegionKind) bool { return defaultBrowseKinds[k] }
