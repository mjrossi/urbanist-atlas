package atlas

import "sort"

// BucketOrgsByScope splits a set of approved orgs into Local + Regional
// buckets given the set of regions in scope for a query. The rule
// (shared with /lookup): if any of an org's attachment regions falls in
// `inScope` AND that region's scope_tier is local, the org is Local;
// otherwise (only regional matches) it's Regional. Orgs whose
// attachments don't intersect inScope at all are dropped.
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
//   - GetRegion: inScope = focus + ancestors + descendants (both
//     directions from a slug-addressed region).
//
// inScope must already have national-tier regions filtered out — the
// function trusts the caller's editorial gate.
func BucketOrgsByScope(inScope map[int64]Region, orgs []Org) (local, regional []Org) {
	var localBuckets, regionalBuckets []scopeBucketed
	for _, org := range orgs {
		matched := make([]Region, 0)
		for _, r := range org.Regions {
			if mr, ok := inScope[r.ID]; ok {
				matched = append(matched, mr)
			}
		}
		if len(matched) == 0 {
			continue
		}
		hasLocal := false
		bestSort := matched[0].SortPriority
		matchedSlugs := make([]string, 0, len(matched))
		for _, r := range matched {
			if r.ScopeTier == ScopeLocal {
				hasLocal = true
			}
			if r.SortPriority < bestSort {
				bestSort = r.SortPriority
			}
			matchedSlugs = append(matchedSlugs, r.Slug)
		}
		org.MatchedRegionSlugs = matchedSlugs
		b := scopeBucketed{org: org, sortKey: bestSort}
		if hasLocal {
			localBuckets = append(localBuckets, b)
		} else {
			regionalBuckets = append(regionalBuckets, b)
		}
	}
	sortScopeBuckets(localBuckets)
	sortScopeBuckets(regionalBuckets)
	return extractScopeOrgs(localBuckets), extractScopeOrgs(regionalBuckets)
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
	if len(b) == 0 {
		return []Org{}
	}
	out := make([]Org, len(b))
	for i, x := range b {
		out[i] = x.org
	}
	return out
}
