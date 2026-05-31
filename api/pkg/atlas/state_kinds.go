package atlas

import "sort"

// stateKinds names the region kinds that count as "state-equivalent" —
// the top administrative tier a postal code rolls up into (state,
// province). Used by BucketOrgsByScope to split the Regional bucket
// into a sub-state Regional tier (metro/CMA/regional-district/transit-
// federation) and a Statewide tier, so /lookup can present "Local",
// "Regional", and "State / Provincial" sections distinctly.
//
// Like metroKinds in metro_kinds.go, this is an explicit editorial set
// — the map is the source of truth, not a derivation from
// sort_priority. A band test (sort_priority == 60) would couple the
// presentational tier to an ordering hint §9 explicitly frames as a
// hint; a kind set is self-documenting and immune to band reshuffles.
//
// In (v1, US + CA only):
//   - us:state     — the 50 states
//   - us:territory — Puerto Rico (parent of its own metros; a
//     territory-wide org sits above any single metro)
//   - ca:province  — the 10 provinces
//   - ca:territory — Yukon, NWT, Nunavut (Canada groups these with
//     provinces: "provinces and territories")
//
// Future markets add their top-admin kind here when they ship with
// data: de:land, uk:nation, pt:nuts-ii, pt:regiao-autonoma, au:state.
// NOT included by design:
//   - us:federal-district — DC is a city-state: the district is
//     coextensive with one city and one metro, and colloquially DC
//     advocacy is either city-scale ("local") or DMV-scale ("the
//     DMV" — the regional framing locals use), never "statewide": DC
//     is emphatically not a state. The seed already splits it across two
//     nodes: washington-dc (a us:city local leaf, where DC ZIPs
//     anchor) and dc (the district, parent of washington-dc-metro). A
//     city-scale DC org tags the local leaf → Local; a DMV org tags
//     the metro → Regional. Promoting the kind here would yank every
//     DMV-tagged org (e.g. Greater Greater Washington) into "State /
//     Provincial", so the district stays Regional and the slug choice
//     does the local/regional split.
//   - us:multi-state — multi-state coalitions (NYC Tri-State,
//     Chicagoland) are broader than a single state but are advocacy
//     federations, not a top-admin tier; they stay in Regional
//     (editorial ruling). Excluding the kind here is the entire
//     mechanism, so the decision is trivially reversible.
//   - de:land for city-states (Berlin, Hamburg, Bremen) does NOT
//     misfire even though de:land is state-equivalent: city-states are
//     editorially scope_tier='local' (sort 15), so their orgs hit the
//     Local branch in BucketOrgsByScope first and never reach the
//     statewide test. Local precedence shields them.
//   - metros, counties, boroughs, transit federations, national tier.
var stateKinds = map[RegionKind]bool{
	"us:state":     true,
	"us:territory": true,
	"ca:province":  true,
	"ca:territory": true,
}

// IsStateKind reports whether k is one of the state-equivalent (top
// administrative tier) kinds. The unknown empty string returns false.
func IsStateKind(k RegionKind) bool { return stateKinds[k] }

// StateKinds returns the state-equivalent kinds in deterministic
// alphabetical order. Callers must not mutate the returned slice — it's
// a fresh copy each call, but treat it as immutable.
func StateKinds() []RegionKind {
	out := make([]RegionKind, 0, len(stateKinds))
	for k := range stateKinds {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// StateKindStrings returns StateKinds as []string. Same ordering and
// freshness guarantees as StateKinds.
func StateKindStrings() []string {
	kinds := StateKinds()
	out := make([]string, len(kinds))
	for i, k := range kinds {
		out[i] = string(k)
	}
	return out
}
