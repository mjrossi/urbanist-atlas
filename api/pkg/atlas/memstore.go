package atlas

import (
	"context"
	"sort"
	"sync"
)

// MemStore is an in-memory Store implementation for tests, fixtures,
// and offline CLI use. It models the region graph as adjacency lists:
// a region->parents map and a postal_code->leaf-region-id map.
//
// MemStore is safe for concurrent use.
type MemStore struct {
	mu            sync.RWMutex
	regionsByID   map[int64]Region
	regionsBySlug map[string]int64
	parents       map[int64][]int64 // region id -> direct parent region ids
	orgs          []Org
	orgRegions    map[int64][]int64 // org id -> region ids it serves
	postalToLeaf  map[string]int64  // postalKey -> leaf region id
}

// NewMemStore returns an empty MemStore. Populate via AddRegion,
// AddParent, AddPostalCode, AddOrg — or call LoadDevFixtures for the
// built-in demo set.
func NewMemStore() *MemStore {
	return &MemStore{
		regionsByID:   map[int64]Region{},
		regionsBySlug: map[string]int64{},
		parents:       map[int64][]int64{},
		orgRegions:    map[int64][]int64{},
		postalToLeaf:  map[string]int64{},
	}
}

// AddRegion registers a region. Later calls with the same ID overwrite.
// ParentSlugs on the supplied region is used to populate the parents
// map; referenced parent slugs must already be registered (call order
// matters: add parents before children).
func (s *MemStore) AddRegion(r Region) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regionsByID[r.ID] = r
	s.regionsBySlug[r.Slug] = r.ID
	if len(r.ParentSlugs) > 0 {
		parentIDs := make([]int64, 0, len(r.ParentSlugs))
		for _, ps := range r.ParentSlugs {
			if pid, ok := s.regionsBySlug[ps]; ok {
				parentIDs = append(parentIDs, pid)
			}
		}
		s.parents[r.ID] = parentIDs
	}
}

// AddOrg registers an organization with the IDs of the regions it
// serves. The org's Regions field is overwritten on read; CreatedAt is
// preserved (it powers newest-first ordering in ListRecent and
// GetMetro). Callers that don't care about ordering may leave it zero.
func (s *MemStore) AddOrg(org Org, regionIDs []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org.Regions = nil
	org.MatchedRegionSlugs = nil
	s.orgs = append(s.orgs, org)
	s.orgRegions[org.ID] = append([]int64(nil), regionIDs...)
}

// AddPostalCode registers a (country, postal code) → leaf region id
// mapping. The code is normalized via NormalizePostalCode.
func (s *MemStore) AddPostalCode(country Country, code string, leafRegionID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postalToLeaf[postalKey(country, code)] = leafRegionID
}

// ResolveLeafRegion implements Store.
func (s *MemStore) ResolveLeafRegion(_ context.Context, country Country, postalCode string) (Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.postalToLeaf[postalKey(country, postalCode)]
	if !ok {
		return Region{}, ErrPostalCodeNotFound
	}
	r, ok := s.regionsByID[id]
	if !ok {
		return Region{}, ErrPostalCodeNotFound
	}
	return r, nil
}

// AncestorRegions implements Store. Returns the leaf followed by all
// transitive ancestors via BFS, dedupes via a visited set.
func (s *MemStore) AncestorRegions(_ context.Context, leafRegionID int64) ([]Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	visited := map[int64]struct{}{}
	out := []Region{}
	queue := []int64{leafRegionID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, seen := visited[id]; seen {
			continue
		}
		visited[id] = struct{}{}
		r, ok := s.regionsByID[id]
		if !ok {
			continue
		}
		out = append(out, r)
		queue = append(queue, s.parents[id]...)
	}
	return out, nil
}

// ListRegions implements Store. Walks every registered region,
// filters by membership in defaultBrowseKinds (metros + cities),
// and counts the orgs whose region attachments are in each
// region's downward DAG closure (so an org tagged only to Brooklyn
// counts toward NYC metro, and an org tagged to Chicago counts
// toward both Chicago and Chicago Metro). Excludes national-tier
// regions and regions with zero approved orgs. Orders by org count
// DESC, then name ASC.
//
// Each summary's BrowseParentSlug is the slug of the nearest
// ancestor whose kind is also in defaultBrowseKinds — the SPA's
// grouping hook for nesting cities under their parent metro. Empty
// string when no such ancestor exists.
func (s *MemStore) ListRegions(_ context.Context) ([]RegionSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []RegionSummary{}
	for id, r := range s.regionsByID {
		if !defaultBrowseKinds[r.Kind] || r.ScopeTier == ScopeNational {
			continue
		}
		descendants := s.descendantRegionIDs(id)
		count := s.countOrgsForRegions(descendants)
		if count == 0 {
			continue
		}
		out = append(out, RegionSummary{
			Region:           r,
			OrgCount:         int64(count),
			BrowseParentSlug: s.nearestBrowseableAncestorSlug(id),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OrgCount != out[j].OrgCount {
			return out[i].OrgCount > out[j].OrgCount
		}
		return out[i].Region.Name < out[j].Region.Name
	})
	return out, nil
}

// nearestBrowseableAncestorSlug walks upward from rootID via the
// parents map and returns the slug of the first ancestor whose kind
// is in defaultBrowseKinds (and is non-national). Returns "" when no
// browseable ancestor exists. Caller must hold s.mu.RLock().
//
// BFS processes shallowest first, so the first hit is the nearest
// ancestor. Walking continues past non-browseable intermediates
// (counties, multi-state regions) so a city like Chicago (parent:
// cook-county) still finds chicago-metro as its grouping anchor.
func (s *MemStore) nearestBrowseableAncestorSlug(rootID int64) string {
	visited := map[int64]bool{rootID: true}
	queue := append([]int64{}, s.parents[rootID]...)
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
		if r.ScopeTier == ScopeNational {
			continue
		}
		if defaultBrowseKinds[r.Kind] {
			return r.Slug
		}
		queue = append(queue, s.parents[id]...)
	}
	return ""
}

// GetRegion implements Store. Resolves any non-national region by
// slug — metros, cities, counties, boroughs, states, multi-state
// coalitions. Returns nil for unknown slugs and for national-tier
// regions (preserving the v1 editorial filter that keeps
// national-org content out of browse contexts). Returned orgs are
// newest-first by CreatedAt.
func (s *MemStore) GetRegion(_ context.Context, slug string) (*RegionDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.regionsBySlug[slug]
	if !ok {
		return nil, nil
	}
	region, ok := s.regionsByID[id]
	if !ok {
		return nil, nil
	}
	if region.ScopeTier == ScopeNational {
		return nil, nil
	}
	// Build the in-scope region set: the focus + every descendant
	// (downward walk) + every ancestor (upward walk). National-tier
	// rows are excluded from both walks. The bucketing then runs
	// against this combined set, so a city's Region page shows orgs
	// from both its constituent neighborhoods AND its parent metro /
	// state / multi-state — matching the Lookup result for any
	// postal code in that city.
	inScope := make(map[int64]Region)

	// Downward walk + collect IDs along the way.
	descendants := s.descendantRegionIDs(id)
	for _, did := range descendants {
		r, ok := s.regionsByID[did]
		if !ok || r.ScopeTier == ScopeNational {
			continue
		}
		inScope[did] = r
	}

	// Upward walk via BFS over parents map. Builds `ancestry`
	// (closest-first, excluding self + national) for the SPA
	// breadcrumb and folds the same rows into inScope.
	ancestry := []Region{}
	visited := map[int64]bool{id: true}
	queue := append([]int64{}, s.parents[id]...)
	for len(queue) > 0 {
		aid := queue[0]
		queue = queue[1:]
		if visited[aid] {
			continue
		}
		visited[aid] = true
		ar, ok := s.regionsByID[aid]
		if !ok {
			continue
		}
		if ar.ScopeTier == ScopeNational {
			continue
		}
		ancestry = append(ancestry, ar)
		inScope[ar.ID] = ar
		queue = append(queue, s.parents[aid]...)
	}

	// Fetch every org with at least one attachment in the in-scope
	// set, then bucket via the shared scope-tier helper.
	ids := make([]int64, 0, len(inScope))
	for k := range inScope {
		ids = append(ids, k)
	}
	orgs := s.orgsForRegionIDs(ids)
	local, regional := BucketOrgsByScope(inScope, orgs)

	return &RegionDetail{
		Region:   region,
		Local:    local,
		Regional: regional,
		Ancestry: ancestry,
	}, nil
}

// GetOrgBySlug implements Store. Scans the in-memory orgs by slug,
// hydrates Regions like the wire contract expects, returns
// ErrOrgNotFound when no row matches.
//
// Behavioral note: the Postgres impl additionally gates on
// status='approved', but atlas.Org has no Status field so MemStore
// can't replicate that filter. Dev/test callers are responsible for
// only loading approved orgs into MemStore (LoadDevFixtures does;
// hand-built fixtures should too).
func (s *MemStore) GetOrgBySlug(_ context.Context, slug string) (*Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, org := range s.orgs {
		if org.Slug != slug {
			continue
		}
		out := org
		out.Regions = s.regionsForOrg(org.ID)
		out.MatchedRegionSlugs = nil
		return &out, nil
	}
	return nil, ErrOrgNotFound
}

// ListRecent implements Store. Excludes orgs whose ONLY region
// attachments are scope_tier='national', orders newest-first, caps at
// 10.
func (s *MemStore) ListRecent(_ context.Context) ([]Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidates := make([]Org, 0, len(s.orgs))
	for _, org := range s.orgs {
		// Filter: at least one non-national region attachment.
		hasNonNational := false
		for _, rid := range s.orgRegions[org.ID] {
			r, ok := s.regionsByID[rid]
			if !ok {
				continue
			}
			if r.ScopeTier != ScopeNational {
				hasNonNational = true
				break
			}
		}
		if !hasNonNational {
			continue
		}
		// Hydrate Regions like the wire contract expects.
		out := org
		out.Regions = s.regionsForOrg(org.ID)
		out.MatchedRegionSlugs = nil
		candidates = append(candidates, out)
	}
	// Sort newest-first with ID DESC as tiebreak so a tied CreatedAt
	// doesn't drift between MemStore and Postgres (Postgres uses
	// "ORDER BY created_at DESC, id DESC" — see browse.sql).
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ID > b.ID
	})
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}
	return candidates, nil
}

// descendantRegionIDs returns rootID followed by every region reachable
// by walking the parents map in reverse (child-of relation). Used to
// answer "which orgs serve a metro or anything under it".
//
// Must be called with s.mu held (read or write).
func (s *MemStore) descendantRegionIDs(rootID int64) []int64 {
	// Build a child->parent index on the fly. With ~30 regions in v1
	// this is cheap; if it ever shows up in a profile, the index can
	// be cached on MemStore directly.
	childrenOf := map[int64][]int64{}
	for childID, parents := range s.parents {
		for _, p := range parents {
			childrenOf[p] = append(childrenOf[p], childID)
		}
	}
	visited := map[int64]bool{}
	out := []int64{}
	queue := []int64{rootID}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		visited[id] = true
		out = append(out, id)
		queue = append(queue, childrenOf[id]...)
	}
	return out
}

// countOrgsForRegions returns the number of distinct orgs with at least
// one attachment in regionIDs. Must be called with s.mu held.
func (s *MemStore) countOrgsForRegions(regionIDs []int64) int {
	if len(regionIDs) == 0 {
		return 0
	}
	wanted := make(map[int64]bool, len(regionIDs))
	for _, id := range regionIDs {
		wanted[id] = true
	}
	count := 0
	for _, org := range s.orgs {
		for _, rid := range s.orgRegions[org.ID] {
			if wanted[rid] {
				count++
				break
			}
		}
	}
	return count
}

// orgsForRegionIDs returns the distinct orgs with at least one
// attachment in regionIDs, with Regions hydrated. Must be called with
// s.mu held.
func (s *MemStore) orgsForRegionIDs(regionIDs []int64) []Org {
	if len(regionIDs) == 0 {
		return nil
	}
	wanted := make(map[int64]bool, len(regionIDs))
	for _, id := range regionIDs {
		wanted[id] = true
	}
	var out []Org
	for _, org := range s.orgs {
		match := false
		for _, rid := range s.orgRegions[org.ID] {
			if wanted[rid] {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		hydrated := org
		hydrated.Regions = s.regionsForOrg(org.ID)
		hydrated.MatchedRegionSlugs = nil
		out = append(out, hydrated)
	}
	return out
}

// regionsForOrg gathers the Region rows for an org's attachments. Must
// be called with s.mu held.
func (s *MemStore) regionsForOrg(orgID int64) []Region {
	ids := s.orgRegions[orgID]
	regions := make([]Region, 0, len(ids))
	for _, rid := range ids {
		if r, ok := s.regionsByID[rid]; ok {
			regions = append(regions, r)
		}
	}
	return regions
}

// OrgsForRegions implements Store.
func (s *MemStore) OrgsForRegions(_ context.Context, regionIDs []int64) ([]Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(regionIDs) == 0 {
		return nil, nil
	}
	wanted := make(map[int64]bool, len(regionIDs))
	for _, id := range regionIDs {
		wanted[id] = true
	}
	var out []Org
	for _, org := range s.orgs {
		orgRegionIDs := s.orgRegions[org.ID]
		match := false
		for _, rid := range orgRegionIDs {
			if wanted[rid] {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		regions := make([]Region, 0, len(orgRegionIDs))
		for _, rid := range orgRegionIDs {
			if r, ok := s.regionsByID[rid]; ok {
				regions = append(regions, r)
			}
		}
		org.Regions = regions
		out = append(out, org)
	}
	return out, nil
}
