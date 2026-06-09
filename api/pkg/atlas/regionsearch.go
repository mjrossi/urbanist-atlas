package atlas

import (
	"sort"
	"strings"
)

// Region-search ranking tiers (lower sorts earlier) and result caps.
// A zero/negative limit selects the default; anything above the hard
// max is clamped so a client can't ask the type-ahead to materialize
// the whole graph.
const (
	rankExactSlug  = 0
	rankExactName  = 1
	rankNamePrefix = 2
	rankSlugPrefix = 3
	rankSubstring  = 4

	defaultRegionSearchLimit = 10
	maxRegionSearchLimit     = 20
)

// regionSearcher holds the type-ahead region-search/ranking cluster
// extracted from MemStore: the match-ranking, the ordered collection,
// and the state-ancestor context label. It reads the same maps the
// store owns (via the embedded *MemStore) rather than copying them, so
// it must run under the same s.mu.RLock the public SearchRegions takes.
// Splitting it out keeps MemStore from accreting the search-relevance
// concern alongside its graph-walk primitives.
type regionSearcher struct {
	store *MemStore
}

// collect runs the ranked search over the full non-national region graph
// for an already-normalized lowercase query and an already-clamped
// limit, returning results ordered by rank then Name then Slug, capped
// at limit, each carrying a state-ancestor context label. The caller
// (MemStore.SearchRegions) owns query normalization, limit clamping, and
// holding s.mu.RLock().
func (rs regionSearcher) collect(q string, limit int) []RegionSearchResult {
	type scored struct {
		region Region
		rank   int
	}
	var hits []scored
	for _, r := range rs.store.regionsByID {
		if r.ScopeTier == ScopeNational {
			continue
		}
		rank, ok := regionSearchRank(strings.ToLower(r.Name), strings.ToLower(r.Slug), q)
		if !ok {
			continue
		}
		hits = append(hits, scored{region: r, rank: rank})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		if hits[i].region.Name != hits[j].region.Name {
			return hits[i].region.Name < hits[j].region.Name
		}
		return hits[i].region.Slug < hits[j].region.Slug
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]RegionSearchResult, 0, len(hits))
	for _, h := range hits {
		out = append(out, RegionSearchResult{
			Region:       h.region,
			ContextLabel: rs.contextLabel(h.region.ID),
		})
	}
	return out
}

// regionSearchRank scores a region against a lowercased query, returning
// the best (lowest) matching tier and whether it matched at all.
func regionSearchRank(nameLower, slugLower, q string) (int, bool) {
	switch {
	case slugLower == q:
		return rankExactSlug, true
	case nameLower == q:
		return rankExactName, true
	case strings.HasPrefix(nameLower, q):
		return rankNamePrefix, true
	case strings.HasPrefix(slugLower, q):
		return rankSlugPrefix, true
	case strings.Contains(nameLower, q) || strings.Contains(slugLower, q):
		return rankSubstring, true
	default:
		return 0, false
	}
}

// contextLabel returns a disambiguation hint for a search result: the
// name of the nearest state/province-equivalent ancestor (BFS upward via
// the parents map; ties at the same depth broken by slug ASC). Falls
// back to the alphabetically-first direct parent's name when no state
// ancestor exists, and to "" when the region has no resolvable parents
// (a state itself, or a top-level region). Caller must hold
// s.mu.RLock().
func (rs regionSearcher) contextLabel(rootID int64) string {
	if r, ok := rs.store.bfsUpwardFirstMatch(rootID, func(r Region) bool {
		return IsStateKind(r.Kind)
	}); ok {
		return r.Name
	}
	return rs.firstParentName(rootID)
}

// firstParentName returns the Name of rootID's alphabetically-first
// (slug ASC) direct, non-national parent, or "" when none resolves.
// Caller must hold s.mu.RLock().
func (rs regionSearcher) firstParentName(rootID int64) string {
	var best *Region
	for _, pid := range rs.store.parents[rootID] {
		r, ok := rs.store.regionsByID[pid]
		if !ok || r.ScopeTier == ScopeNational {
			continue
		}
		if best == nil || r.Slug < best.Slug {
			rr := r
			best = &rr
		}
	}
	if best == nil {
		return ""
	}
	return best.Name
}
