package atlas

import "sort"

// BucketOrgsByScope splits a set of approved orgs into Local, Regional,
// and Statewide buckets given the set of regions in scope for a query.
// The rule (shared with /lookup), in precedence order:
//
//   - if any of an org's matched attachment regions has
//     scope_tier='local', the org is Local;
//   - else if any matched region is a state-equivalent kind
//     (IsStateKind: us:state, us:territory, ca:province, ca:territory),
//     the org is Statewide;
//   - else (only sub-state regional matches: metro/CMA/regional-
//     district/transit-federation/multi-state) it's Regional.
//
// Orgs whose attachments don't intersect inScope at all are dropped.
//
// The Statewide split is derived from region kind, not a new scope_tier
// value — the closed scope_tier enum (local/regional/national) is
// unchanged. Local precedence means editorial city-state overrides
// (Berlin: kind=de:land but scope_tier=local) correctly stay Local and
// never reach the IsStateKind test. Multi-state coalitions
// (us:multi-state) are intentionally NOT state-equivalent, so they stay
// Regional. See state_kinds.go for the editorial set.
//
// Each returned Org has its MatchedRegionSlugs populated with the slugs
// of the in-scope attachment regions that caused it to surface. Useful
// for "Matched via X" affordances in the SPA.
//
// Each bucket is sorted by (min sort_priority of matched regions asc,
// org.Name asc) — narrower-scope matches surface first within a
// bucket, alphabetic tiebreak.
//
// Callers:
//   - Lookup: inScope = leaf + ancestors (upward walk from a postal
//     code's leaf region).
//   - GetRegion: inScope = focus + descendants + rolled-up metros
//     (rollup_states). Ancestors are used only for the breadcrumb, never
//     for org scope.
//
// inScope must already have national-tier regions filtered out — the
// function trusts the caller's editorial gate.
func BucketOrgsByScope(inScope map[int64]Region, orgs []Org) (local, regional, statewide []Org) {
	var localBuckets, regionalBuckets, statewideBuckets []scopeBucketed
	for _, org := range orgs {
		matched := make([]Region, 0, len(org.Regions))
		for _, r := range org.Regions {
			if mr, ok := inScope[r.ID]; ok {
				matched = append(matched, mr)
			}
		}
		if len(matched) == 0 {
			continue
		}
		hasLocal := false
		hasState := false
		bestSort := matched[0].SortPriority
		matchedSlugs := make([]string, 0, len(matched))
		for _, r := range matched {
			if r.ScopeTier == ScopeLocal {
				hasLocal = true
			}
			if IsStateKind(r.Kind) {
				hasState = true
			}
			if r.SortPriority < bestSort {
				bestSort = r.SortPriority
			}
			matchedSlugs = append(matchedSlugs, r.Slug)
		}
		org.MatchedRegionSlugs = matchedSlugs
		b := scopeBucketed{org: org, sortKey: bestSort}
		switch {
		case hasLocal:
			localBuckets = append(localBuckets, b)
		case hasState:
			statewideBuckets = append(statewideBuckets, b)
		default:
			regionalBuckets = append(regionalBuckets, b)
		}
	}
	sortScopeBuckets(localBuckets)
	sortScopeBuckets(regionalBuckets)
	sortScopeBuckets(statewideBuckets)
	return extractScopeOrgs(localBuckets), extractScopeOrgs(regionalBuckets), extractScopeOrgs(statewideBuckets)
}

type scopeBucketed struct {
	org     Org
	sortKey int
}

func sortScopeBuckets(b []scopeBucketed) {
	sort.SliceStable(b, func(i, j int) bool {
		if b[i].sortKey != b[j].sortKey {
			return b[i].sortKey < b[j].sortKey
		}
		return b[i].org.Name < b[j].org.Name
	})
}

func extractScopeOrgs(b []scopeBucketed) []Org {
	out := make([]Org, len(b))
	for i, x := range b {
		out[i] = x.org
	}
	return out
}
