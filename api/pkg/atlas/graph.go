package atlas

// MemStore's region-graph walks and derived indexes. Every method here
// must run under s.mu (read or write).

// bfsCollectIDs returns start followed by every region reachable via adj
// (parents for the upward walk, children for the downward one) in
// breadth-first order. A national-tier row is neither collected nor
// expanded, so it can't act as a hop to a deeper region.
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

// buildChildrenOf returns a fresh parent → children map derived from
// s.parents. It isn't cached on the struct, so it is safe under RLock.
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
// descendant. It rebuilds the children map on each call, so a caller
// walking many roots should hoist buildChildrenOf instead.
func (s *MemStore) descendantRegionIDs(rootID int64) []int64 {
	return s.bfsCollectIDs(rootID, s.buildChildrenOf())
}

// buildOrgsByRegion returns a fresh region id → attached org ids index.
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
// region in regionIDs.
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
