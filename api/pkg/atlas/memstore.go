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
// OrgsForRegions). Callers that don't care about ordering may leave it zero.
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
// transitive ancestors via BFS, dedupes via a visited set. Excludes
// scope_tier='national' rows from both the seed and the recursion
// (matches the Postgres CTE contract; the storetest harness pins
// this).
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
		if r.ScopeTier == ScopeNational {
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
			DirectOrgCount:   int64(s.countOrgsForRegions([]int64{id})),
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
// parents map and returns the slug of the nearest ancestor whose kind
// is in defaultBrowseKinds (and is non-national). Returns "" when no
// browseable ancestor exists. Caller must hold s.mu.RLock().
//
// Walks depth-by-depth past non-browseable intermediates (counties,
// multi-state regions) so a city like Chicago (parent: cook-county)
// still finds chicago-metro as its grouping anchor. When multiple
// browseable parents share the minimum depth, ties are broken by
// slug ASC — same rule as Postgres' nearest_browseable_parent CTE
// (browse.sql `ORDER BY a.depth ASC, r.slug ASC`).
func (s *MemStore) nearestBrowseableAncestorSlug(rootID int64) string {
	visited := map[int64]bool{rootID: true}
	current := append([]int64{}, s.parents[rootID]...)
	for len(current) > 0 {
		var hits []string
		var next []int64
		for _, id := range current {
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
				hits = append(hits, r.Slug)
				continue
			}
			next = append(next, s.parents[id]...)
		}
		if len(hits) > 0 {
			sort.Strings(hits)
			return hits[0]
		}
		current = next
	}
	return ""
}

// ResolveRegionBySlug implements Store. Returns ErrRegionNotFound for
// unknown slugs and for national-tier rows.
func (s *MemStore) ResolveRegionBySlug(_ context.Context, slug string) (Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.regionsBySlug[slug]
	if !ok {
		return Region{}, ErrRegionNotFound
	}
	r, ok := s.regionsByID[id]
	if !ok {
		return Region{}, ErrRegionNotFound
	}
	if r.ScopeTier == ScopeNational {
		return Region{}, ErrRegionNotFound
	}
	return r, nil
}

// DescendantRegions implements Store. Walks the parent->child relation
// via the parents map and returns the focus at index 0 followed by
// every reachable descendant. Excludes national-tier rows from both
// the seed and the recursion (matches the Postgres CTE contract).
func (s *MemStore) DescendantRegions(_ context.Context, focusRegionID int64) ([]Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	focus, ok := s.regionsByID[focusRegionID]
	if !ok || focus.ScopeTier == ScopeNational {
		return nil, nil
	}
	ids := s.descendantRegionIDs(focusRegionID)
	out := make([]Region, 0, len(ids))
	for _, id := range ids {
		r, ok := s.regionsByID[id]
		if !ok || r.ScopeTier == ScopeNational {
			continue
		}
		out = append(out, r)
	}
	return out, nil
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

// descendantRegionIDs returns rootID followed by every non-national
// region reachable by walking the parents map in reverse (child-of
// relation). Excludes scope_tier='national' rows from both the seed
// and the recursion so an editorial slip-up (a national region wired
// under a metro) can't inflate a metro's org_count via ListRegions or
// leak into GetRegion's in-scope set. Matches the Postgres
// DescendantRegions CTE.
//
// Must be called with s.mu held (read or write).
func (s *MemStore) descendantRegionIDs(rootID int64) []int64 {
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
		r, ok := s.regionsByID[id]
		if !ok {
			continue
		}
		if r.ScopeTier == ScopeNational {
			continue
		}
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

// regionsForOrg gathers the Region rows for an org's attachments,
// sorted ascending by region ID so the wire shape matches the Postgres
// `ARRAY(... ORDER BY orx.region_id)`. Must be called with s.mu held.
func (s *MemStore) regionsForOrg(orgID int64) []Region {
	ids := s.orgRegions[orgID]
	regions := make([]Region, 0, len(ids))
	for _, rid := range ids {
		if r, ok := s.regionsByID[rid]; ok {
			regions = append(regions, r)
		}
	}
	sort.Slice(regions, func(i, j int) bool { return regions[i].ID < regions[j].ID })
	return regions
}

// OrgsForRegions implements Store. Each org's Regions slice is
// hydrated sorted ascending by region ID (matches Postgres SQL).
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
		org.Regions = s.regionsForOrg(org.ID)
		out = append(out, org)
	}
	return out, nil
}
