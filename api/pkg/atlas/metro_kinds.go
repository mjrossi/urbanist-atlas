package atlas

import "sort"

// metroKinds names the region kinds that count as "metro-equivalent"
// — administrative geographies at MSA / CMA / metropolitan-area
// granularity. Used by /lookup's placeLabel to pick the "broad"
// ancestor slot, so a Brooklyn ZIP renders as "Brooklyn, NYC — New
// York Metro" rather than "Brooklyn, NYC — New York City".
//
// This is intentionally narrower than defaultBrowseKinds in
// browse_kinds.go, which is the set surfaced on /api/v1/regions
// (Browse). Cities are in the default browse set but are not
// "metro-equivalent" for label-building.
//
// In:
//   - us:metro              — MSA/CSA
//   - ca:cma                — Census Metropolitan Area
//   - ca:regional-district  — BC's multi-municipal layer that plays a
//     metro role (e.g. Metro Vancouver)
//   - pt:area-metropolitana — AML, AMP
//
// Out: cities, states/provinces, counties, boroughs, distritos, NUTS
// regions, autonomous communities, national tier.
var metroKinds = map[RegionKind]bool{
	"us:metro":              true,
	"ca:cma":                true,
	"ca:regional-district":  true,
	"pt:area-metropolitana": true,
}

// IsMetroKind reports whether k is one of the metro-equivalent kinds.
// The unknown empty string returns false.
func IsMetroKind(k RegionKind) bool { return metroKinds[k] }

// MetroKinds returns the metro-equivalent kinds in deterministic
// alphabetical order. Callers must not mutate the returned slice —
// it's a fresh copy each call, but treat it as immutable.
func MetroKinds() []RegionKind {
	out := make([]RegionKind, 0, len(metroKinds))
	for k := range metroKinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// MetroKindStrings returns MetroKinds as []string. Same ordering and
// freshness guarantees as MetroKinds.
func MetroKindStrings() []string {
	kinds := MetroKinds()
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}
