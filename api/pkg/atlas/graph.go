package atlas

// This file holds MemStore's region-graph machinery, extracted from
// memstore.go (following the regionsearch.go precedent): the shared
// BFS traversal both DAG directions run on, the derived adjacency /
// inverted-index builders, and the distinct-org counter behind browse
// org counts. Everything here reads the same maps the store owns and
// must run under s.mu (read or write); memstore.go keeps storage,
// the mutation API, and the Store-method dispatch. Splitting it out
// keeps MemStore from accreting the graph-walk concern alongside its
// storage plumbing.

// bfsCollectIDs is the shared queue-BFS skeleton behind both DAG
// directions: AncestorRegions walks the parents map upward, and the
// descendant walks (descendantRegionIDs / ListRegions) walk a
// precomputed parent → children map downward. It returns start
// followed by every region reachable via adj, in breadth-first visit
// order, deduped via a visited set. Rows missing from regionsByID and
// scope_tier='national' rows are excluded from both the seed and the
// recursion — a national row is neither collected nor expanded, so it
// can't act as a transit hop (the storetest harness pins this contract
// for both directions).
//
// Must be called with s.mu held (read or write).
func (s *MemStore) bfsCollectIDs(start int64, adj map[int64][]int64) []int64 {
	visited := map[int64]bool{}
	out := []int64{}
	queue := []int64{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		r, ok := s.regionsByID[id]
		if !ok {
			continue
		}
		if r.IsNational() {
			continue
		}
		out = append(out, id)
		queue = append(queue, adj[id]...)
	}
	return out
}

// buildChildrenOf returns a fresh reverse-adjacency map (parent → list
// of children) derived from s.parents. O(P) in the parent-edge count.
// Caller must hold s.mu (read or write). Allocate-and-return rather
// than caching on the struct so the function is safe under RLock
// without coordinating writes; the dominant caller (ListRegions)
// builds it once per request and reuses across the loop.
func (s *MemStore) buildChildrenOf() map[int64][]int64 {
	out := map[int64][]int64{}
	for childID, parents := range s.parents {
		for _, p := range parents {
			out[p] = append(out[p], childID)
		}
	}
	return out
}

// descendantRegionIDs returns rootID followed by every non-national
// region reachable by walking the parents map in reverse (child-of
// relation). Excludes scope_tier='national' rows from both the seed
// and the recursion so an editorial slip-up (a national region wired
// under a metro) can't inflate a metro's org_count via ListRegions or
// leak into GetRegion's in-scope set. Shares the DescendantRegions
// exclusion contract.
//
// Builds childrenOf inline — convenience wrapper for callers like
// DescendantRegions that don't loop over multiple roots. Multi-root
// callers (ListRegions) should hoist the buildChildrenOf call and
// pass it to bfsCollectIDs directly to avoid the O(P) rebuild per
// root.
//
// Must be called with s.mu held (read or write).
func (s *MemStore) descendantRegionIDs(rootID int64) []int64 {
	return s.bfsCollectIDs(rootID, s.buildChildrenOf())
}

// buildOrgsByRegion returns a fresh inverted index (region id → set of
// the org ids attached to it). O(O · A) in the org count × attachments
// per org — one pass over the org→regions adjacency. Caller must hold
// s.mu. ListRegions builds it once per request so org counts become a
// set-union over a region's descendant ids plus a map lookup for the
// direct count, instead of re-scanning every org per browseable region.
func (s *MemStore) buildOrgsByRegion() map[int64]map[int64]struct{} {
	out := map[int64]map[int64]struct{}{}
	for _, org := range s.orgs {
		for _, rid := range s.orgRegions[org.ID] {
			set := out[rid]
			if set == nil {
				set = map[int64]struct{}{}
				out[rid] = set
			}
			set[org.ID] = struct{}{}
		}
	}
	return out
}

// countDistinctOrgs returns the number of distinct orgs attached to any
// region in regionIDs, reading the precomputed orgsByRegion inverted
// index. Equivalent to the old per-region full org scan, but O(sum of
// per-region set sizes) instead of O(O · A) per region.
func countDistinctOrgs(orgsByRegion map[int64]map[int64]struct{}, regionIDs []int64) int {
	if len(regionIDs) == 0 {
		return 0
	}
	seen := map[int64]struct{}{}
	for _, rid := range regionIDs {
		for orgID := range orgsByRegion[rid] {
			seen[orgID] = struct{}{}
		}
	}
	return len(seen)
}
