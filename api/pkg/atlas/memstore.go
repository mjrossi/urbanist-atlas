package atlas

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// MemStore is the in-memory Store. It backs the runtime read path as
// well as tests, fixtures, and CLI use, and is safe for concurrent use.
type MemStore struct {
	mu            sync.RWMutex
	regionsByID   map[int64]Region
	regionsBySlug map[string]int64
	parents       map[int64][]int64 // region id -> direct parent region ids
	orgs          []Org
	orgRegions    map[int64][]int64 // org id -> region ids it serves
	postalToLeaf  map[string]int64  // postalKey -> leaf region id
	// rollupByState maps a rollup-target region id to the metro ids that
	// name it in rollup_states. The ancestor walk never reads it.
	rollupByState map[int64][]int64
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
		rollupByState: map[int64][]int64{},
	}
}

// AddRegion registers a region. Later calls with the same ID overwrite.
// Every slug in r.ParentSlugs must already be registered, so add parents
// before children; an unknown parent is an error and r is not added.
// A slug with uppercase letters is an error, because slug lookups fold
// case and two slugs differing only in case would be ambiguous.
func (s *MemStore) AddRegion(r Region) error {
	if r.Slug != strings.ToLower(r.Slug) {
		return fmt.Errorf("atlas: region slug %q must be lowercase", r.Slug)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parentIDs := make([]int64, 0, len(r.ParentSlugs))
	for _, ps := range r.ParentSlugs {
		pid, ok := s.regionsBySlug[ps]
		if !ok {
			return fmt.Errorf("atlas: region %q references unknown parent slug %q (add the parent first)", r.Slug, ps)
		}
		parentIDs = append(parentIDs, pid)
	}
	s.regionsByID[r.ID] = r
	s.regionsBySlug[r.Slug] = r.ID
	if len(parentIDs) > 0 {
		s.parents[r.ID] = parentIDs
	}
	return nil
}

// AddRollupState records that metroSlug's own orgs also surface on
// stateSlug's detail page. It is not an ancestor edge. Idempotent; an
// unregistered slug is silently ignored, so callers validate first.
func (s *MemStore) AddRollupState(metroSlug, stateSlug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	metroID, ok := s.regionsBySlug[metroSlug]
	if !ok {
		return
	}
	stateID, ok := s.regionsBySlug[stateSlug]
	if !ok {
		return
	}
	for _, id := range s.rollupByState[stateID] {
		if id == metroID {
			return
		}
	}
	s.rollupByState[stateID] = append(s.rollupByState[stateID], metroID)
}

// AddOrg registers an organization with the IDs of the regions it
// serves. Regions is rebuilt on read; AddedAt is kept and orders
// ListRecent. org.Slug must be lowercase: GetOrgBySlug folds the
// requested slug to lowercase, so a slug with uppercase letters is never
// found.
func (s *MemStore) AddOrg(org Org, regionIDs []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org.Regions = nil
	org.MatchedRegionSlugs = nil
	s.orgs = append(s.orgs, org)
	s.orgRegions[org.ID] = append([]int64(nil), regionIDs...)
}

// AddPostalCode registers a (country, postal code) → leaf region id
// mapping. The code is normalized via NormalizePostalCode. Re-adding an
// identical mapping is a no-op; mapping a code to a second region is an
// error, and the first mapping is kept.
func (s *MemStore) AddPostalCode(country Country, code string, leafRegionID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := postalKey(country, code)
	if prev, ok := s.postalToLeaf[key]; ok && prev != leafRegionID {
		return fmt.Errorf("atlas: duplicate postal code %s %s: already mapped to region %s, cannot also map to %s",
			country, NormalizePostalCode(country, code), s.regionLabel(prev), s.regionLabel(leafRegionID))
	}
	s.postalToLeaf[key] = leafRegionID
	return nil
}

// regionLabel names a region by slug for error messages, falling back
// to the ID for one not yet registered. Caller must hold s.mu.
func (s *MemStore) regionLabel(id int64) string {
	if r, ok := s.regionsByID[id]; ok {
		return strconv.Quote(r.Slug)
	}
	return "id " + strconv.FormatInt(id, 10)
}

// Slugs returns every registered region slug, national tier included,
// in unspecified order. Unlike ListRegions it is not limited to the
// browse subset, because the published-slug guard
// (seedfiles/published_slugs_test.go) must see every addressable slug.
func (s *MemStore) Slugs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.regionsBySlug))
	for slug := range s.regionsBySlug {
		out = append(out, slug)
	}
	return out
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

// AncestorRegions implements Store.
func (s *MemStore) AncestorRegions(_ context.Context, leafRegionID int64) ([]Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.bfsCollectIDs(leafRegionID, s.parents)
	out := make([]Region, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.regionsByID[id])
	}
	return out, nil
}

// ListRegions implements Store.
func (s *MemStore) ListRegions(_ context.Context) ([]RegionSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listRegionsLocked(), nil
}

// listRegionsLocked is the body of ListRegions, shared with Stats so the
// browse predicate exists once. Must be called with s.mu held.
func (s *MemStore) listRegionsLocked() []RegionSummary {
	// Both indexes are built once per call rather than once per region.
	childrenOf := s.buildChildrenOf()
	// orgsByRegion turns each region's counts into a set union.
	orgsByRegion := s.buildOrgsByRegion()
	out := []RegionSummary{}
	for id, r := range s.regionsByID {
		if !defaultBrowseKinds[r.Kind] || r.IsNational() {
			continue
		}
		descendants := s.bfsCollectIDs(id, childrenOf)
		count := countDistinctOrgs(orgsByRegion, descendants)
		if count == 0 {
			continue
		}
		out = append(out, RegionSummary{
			Region:           r,
			OrgCount:         int64(count),
			DirectOrgCount:   int64(len(orgsByRegion[id])),
			BrowseParentSlug: s.nearestBrowseableAncestorSlug(id),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OrgCount != out[j].OrgCount {
			return out[i].OrgCount > out[j].OrgCount
		}
		return out[i].Region.Name < out[j].Region.Name
	})
	return out
}

// nearestBrowseableAncestorSlug returns the slug of the nearest
// non-national browse-kind ancestor, or "". It walks past non-browse
// intermediates, so a city under a county still finds its metro.
// Caller must hold s.mu.
func (s *MemStore) nearestBrowseableAncestorSlug(rootID int64) string {
	if r, ok := s.bfsUpwardFirstMatch(rootID, func(r Region) bool {
		return defaultBrowseKinds[r.Kind]
	}); ok {
		return r.Slug
	}
	return ""
}

// bfsUpwardFirstMatch returns the shallowest ancestor of rootID (not
// rootID itself) satisfying match, breaking ties within a level by slug
// ASC. National-tier rows are neither matched nor expanded. Caller must
// hold s.mu.
func (s *MemStore) bfsUpwardFirstMatch(rootID int64, match func(Region) bool) (Region, bool) {
	visited := map[int64]bool{rootID: true}
	current := append([]int64{}, s.parents[rootID]...)
	for len(current) > 0 {
		var hits []Region
		var next []int64
		for _, id := range current {
			if visited[id] {
				continue
			}
			visited[id] = true
			r, ok := s.regionsByID[id]
			if !ok || r.IsNational() {
				continue
			}
			if match(r) {
				hits = append(hits, r)
				continue
			}
			next = append(next, s.parents[id]...)
		}
		if len(hits) > 0 {
			sort.Slice(hits, func(i, j int) bool { return hits[i].Slug < hits[j].Slug })
			return hits[0], true
		}
		current = next
	}
	return Region{}, false
}

// SearchRegions implements Store. The ranking lives in regionsearch.go.
func (s *MemStore) SearchRegions(_ context.Context, query string, limit int) ([]RegionSearchResult, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []RegionSearchResult{}, nil
	}
	if limit <= 0 {
		limit = defaultRegionSearchLimit
	}
	if limit > maxRegionSearchLimit {
		limit = maxRegionSearchLimit
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.searchRegionsLocked(q, limit), nil
}

// ResolveRegionBySlug implements Store.
func (s *MemStore) ResolveRegionBySlug(_ context.Context, slug string) (Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.regionsBySlug[strings.ToLower(slug)]
	if !ok {
		return Region{}, ErrRegionNotFound
	}
	r, ok := s.regionsByID[id]
	if !ok {
		return Region{}, ErrRegionNotFound
	}
	if r.IsNational() {
		return Region{}, ErrRegionNotFound
	}
	return r, nil
}

// DescendantRegions implements Store.
func (s *MemStore) DescendantRegions(_ context.Context, focusRegionID int64) ([]Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	focus, ok := s.regionsByID[focusRegionID]
	if !ok || focus.IsNational() {
		return nil, nil
	}
	ids := s.descendantRegionIDs(focusRegionID)
	out := make([]Region, 0, len(ids))
	for _, id := range ids {
		r, ok := s.regionsByID[id]
		if !ok || r.IsNational() {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// RollupMetrosFor implements Store, sorting by ID.
func (s *MemStore) RollupMetrosFor(_ context.Context, stateRegionID int64) ([]Region, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := s.rollupByState[stateRegionID]
	out := make([]Region, 0, len(ids))
	for _, id := range ids {
		r, ok := s.regionsByID[id]
		if !ok || r.IsNational() {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// GetOrgBySlug implements Store. Org has no status field, so MemStore
// can't filter on approval: load only approved orgs into it.
func (s *MemStore) GetOrgBySlug(_ context.Context, slug string) (*Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	slug = strings.ToLower(slug)
	for _, org := range s.orgs {
		if org.Slug != slug {
			continue
		}
		out := org
		out.Regions = s.regionsForOrg(org.ID)
		out.MatchedRegionSlugs = nil
		// Clone Tags; see OrgsForRegions.
		out.Tags = append([]Tag(nil), out.Tags...)
		return &out, nil
	}
	return nil, ErrOrgNotFound
}

// ListRecent implements Store.
func (s *MemStore) ListRecent(_ context.Context) ([]Org, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	candidates := make([]Org, 0, len(s.orgs))
	for _, org := range s.orgs {
		// Skip orgs whose only attachments are national-tier.
		hasNonNational := false
		for _, rid := range s.orgRegions[org.ID] {
			r, ok := s.regionsByID[rid]
			if !ok {
				continue
			}
			if !r.IsNational() {
				hasNonNational = true
				break
			}
		}
		if !hasNonNational {
			continue
		}
		// out is a copy; the stored org stays unhydrated.
		out := org
		out.Regions = s.regionsForOrg(org.ID)
		out.MatchedRegionSlugs = nil
		// Clone Tags; see OrgsForRegions.
		out.Tags = append([]Tag(nil), out.Tags...)
		candidates = append(candidates, out)
	}
	// The seed backfill shares a handful of AddedAt dates, so the ID DESC
	// tiebreak decides much of the order.
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if !a.AddedAt.Equal(b.AddedAt) {
			return a.AddedAt.After(b.AddedAt)
		}
		return a.ID > b.ID
	})
	if len(candidates) > 10 {
		candidates = candidates[:10]
	}
	return candidates, nil
}

// Stats implements Store. It counts over s.orgs directly, never by
// summing per-region counts; see atlas.Stats.
func (s *MemStore) Stats(_ context.Context) (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orgsByCountry := map[Country]int{}
	totalOrgs := 0
	for _, org := range s.orgs {
		// The same live-org filter as ListRecent.
		hasNonNational := false
		countries := map[Country]struct{}{}
		for _, rid := range s.orgRegions[org.ID] {
			r, ok := s.regionsByID[rid]
			if !ok || r.ScopeTier == ScopeNational {
				continue
			}
			hasNonNational = true
			// Hand-built fixtures may leave Country blank; such an org
			// counts toward the total but no country row.
			if r.Country != "" {
				countries[r.Country] = struct{}{}
			}
		}
		if !hasNonNational {
			continue
		}
		totalOrgs++
		for c := range countries {
			orgsByCountry[c]++
		}
	}

	browse := s.listRegionsLocked()
	regionsByCountry := map[Country]int{}
	for _, summary := range browse {
		// As above, a blank Country counts toward the total only.
		if summary.Region.Country != "" {
			regionsByCountry[summary.Region.Country]++
		}
	}

	totalRegions := 0
	for _, r := range s.regionsByID {
		if r.ScopeTier != ScopeNational {
			totalRegions++
		}
	}

	codes := make([]Country, 0, len(orgsByCountry))
	seen := map[Country]struct{}{}
	for _, m := range []map[Country]int{orgsByCountry, regionsByCountry} {
		for c := range m {
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			codes = append(codes, c)
		}
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })

	byCountry := make([]CountryStats, 0, len(codes))
	for _, c := range codes {
		byCountry = append(byCountry, CountryStats{
			Country:     c,
			OrgCount:    orgsByCountry[c],
			RegionCount: regionsByCountry[c],
		})
	}

	return Stats{
		TotalOrgCount:     totalOrgs,
		TotalRegionCount:  totalRegions,
		BrowseRegionCount: len(browse),
		ByCountry:         byCountry,
	}, nil
}

// regionsForOrg returns an org's attached regions sorted by ID. Must be
// called with s.mu held.
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
		org.Regions = s.regionsForOrg(org.ID)
		// Clone Tags so a caller mutating the result can't write into the
		// store, which is only read-locked here. nil stays nil.
		org.Tags = append([]Tag(nil), org.Tags...)
		out = append(out, org)
	}
	return out, nil
}
