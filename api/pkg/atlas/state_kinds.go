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
//   - us:state    — the 50 states + DC + PR territory rows
//   - ca:province — the 10 provinces + 3 territories
//
// Future markets add their top-admin kind here when they ship with
// data: ca:territory (if split out), de:land, uk:nation, pt:nuts-ii,
// pt:regiao-autonoma, au:state. NOT included by design:
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
	"us:state":    true,
	"ca:province": true,
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
