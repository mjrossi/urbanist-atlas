package atlas

import "sort"

// metroKinds names the region kinds that count as "metro-equivalent"
// for the purpose of /api/v1/metros and the homepage Browse panel. The
// set is editorial, not derived from a suffix pattern, because the
// administrative geographies that qualify don't share a stable lexical
// suffix.
//
// In:
//   - us:metro              — MSA/CSA
//   - ca:cma                — Census Metropolitan Area
//   - ca:regional-district  — BC's multi-municipal layer that plays a
//     metro role (e.g. Metro Vancouver)
//   - pt:area-metropolitana — AML, AMP
//
// Out: states/provinces, distritos, NUTS regions, autonomous communities,
// national tier.
//
// Adding a new country's metro-equivalent kind is a one-line append
// here; the predicate and the accessor stay unchanged. The
// per-country editorial conventions live in docs/region-graph.md.
var metroKinds = map[RegionKind]bool{
	"us:metro":              true,
	"ca:cma":                true,
	"ca:regional-district":  true,
	"pt:area-metropolitana": true,
}

// IsMetroKind reports whether k is one of the metro-equivalent kinds
// recognized by /api/v1/metros. The unknown empty string returns false.
func IsMetroKind(k RegionKind) bool { return metroKinds[k] }

// MetroKinds returns the metro-equivalent kinds in deterministic
// alphabetical order. The SQL layer passes this as a $1::text[]
// parameter, so a stable order keeps query plans (and EXPLAINs)
// readable. Callers must not mutate the returned slice — it's a fresh
// copy each call, but treat it as immutable to keep the API obvious.
func MetroKinds() []RegionKind {
	out := make([]RegionKind, 0, len(metroKinds))
	for k := range metroKinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
