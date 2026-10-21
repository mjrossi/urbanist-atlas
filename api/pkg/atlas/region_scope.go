package atlas

import "sort"

// BucketOrgsByScope splits orgs into Local, Regional, and Statewide by
// the regions of theirs that are in inScope. An org with any local-tier
// match is Local; else one with any state-equivalent match (IsStateKind)
// is Statewide; else it is Regional. Orgs with no match are dropped.
//
// Each org's MatchedRegionSlugs is set to its matched regions, and each
// bucket is sorted by lowest matched sort_priority, then Name. inScope
// must already exclude national-tier regions. See docs/region-graph.md,
// "Open vocabulary", for why the tiers fall where they do.
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
