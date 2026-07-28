package atlas

import (
	"context"
	"sort"
	"strings"
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
	// rollupByState maps a state-equivalent (or us:federal-district)
	// region id to the metro region ids whose OWN orgs surface on that
	// region's detail page (the rollup_states relation). Browse/descendant
	// direction ONLY — never read by AncestorRegions/Lookup, so it cannot
	// leak orgs across a postal-code lookup.
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

// AddRollupState records that metroSlug's OWN orgs should additionally
// surface on stateSlug's detail page (the rollup_states relation). This
// is the descendant/browse direction ONLY: it is never an ancestor edge,
// so a leaf under metroSlug can never reach stateSlug via the ancestor
// walk. Both slugs must already be registered (the seedfiles loader calls
// this in a post-pass, after every region is added). Idempotent per
// (state, metro) pair; an unregistered slug is silently ignored (the
// loader validates existence and kind before calling).
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
// serves. The org's Regions field is overwritten on read; AddedAt is
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

// Slugs returns every registered region slug — the FULL set across all
// tiers and kinds (states/provinces, hand-curated leaves, generated
// MSA/CMA, and national umbrellas), unlike ListRegions which returns
// only the browseable, org-bearing subset. The order is unspecified;
// callers that need determinism should sort. Backs the published-slug
// append-only guard (the slug→consumer permanence contract), which
// must see every slug a /regions/{slug} consumer could address.
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

// AncestorRegions implements Store. Returns the leaf followed by all
// transitive ancestors via BFS (bfsCollectIDs over the parents map),
// dedupes via a visited set. Excludes scope_tier='national' rows from
// both the seed and the recursion (the storetest harness pins this
// contract).
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
	return s.listRegionsLocked(), nil
}

// listRegionsLocked is the body of ListRegions, split out so Stats can
// reuse the identical result rather than re-deriving the "browseable
// kind AND non-national AND >=1 org in subtree" predicate. A second
// copy of that predicate would be free to drift from this one, and a
// drifting duplicate of exactly this logic is what made the frontend
// under-report the org total in the first place.
//
// Must be called with s.mu held (read or write).
func (s *MemStore) listRegionsLocked() []RegionSummary {
	// Build the reverse-adjacency map once for the whole call instead
	// of inside each descendantRegionIDs invocation. Without this hoist
	// the cost was O(R · P) per ListRegions (R browseable regions, P
	// parent edges) — at v1 scale (~5k of each) that's noticeable in
	// tests that hammer the endpoint. Hoisted, the whole call is O(R + P).
	childrenOf := s.buildChildrenOf()
	// Build the region->orgs inverted index once for the whole call too.
	// The old code counted orgs by scanning every org (and each org's
	// attachments) once per browseable region for OrgCount AND a second
	// time for DirectOrgCount — O(R · O · A). With the inverted index,
	// each region's counts are a set-union over its descendant ids and a
	// single map lookup, so the whole call is O(O · A + R + P) instead.
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

// nearestBrowseableAncestorSlug walks upward from rootID via the
// parents map and returns the slug of the nearest ancestor whose kind
// is in defaultBrowseKinds (and is non-national). Returns "" when no
// browseable ancestor exists. Caller must hold s.mu.RLock().
//
// Walks depth-by-depth past non-browseable intermediates (counties,
// multi-state regions) so a city like Chicago (parent: cook-county)
// still finds chicago-metro as its grouping anchor. When multiple
// browseable parents share the minimum depth, ties are broken by
// slug ASC (depth ASC, then slug ASC) so the choice is deterministic.
func (s *MemStore) nearestBrowseableAncestorSlug(rootID int64) string {
	if r, ok := s.bfsUpwardFirstMatch(rootID, func(r Region) bool {
		return defaultBrowseKinds[r.Kind]
	}); ok {
		return r.Slug
	}
	return ""
}

// bfsUpwardFirstMatch walks the parents map upward from rootID
// depth-by-depth and returns the first Region satisfying match. "First"
// is the shallowest level that contains any match; ties within that
// level are broken by slug ASC so the choice is deterministic. National-
// tier rows are skipped entirely — never matched and never expanded —
// mirroring the ancestor-walk exclusion, so they can neither be returned
// nor act as a transit hop to a deeper match. rootID itself is excluded
// (seeded into the visited set), matching the "ancestor, not self"
// semantics both callers want. Returns (zero Region, false) when no
// ancestor matches. Caller must hold s.mu.RLock().
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

// SearchRegions implements Store. Case-insensitive name/slug match over
// the full non-national region graph, ranked for type-ahead relevance,
// each result carrying a state-ancestor disambiguation label. The
// ranking/collection/labeling cluster lives on regionSearcher
// (regionsearch.go); this method owns query normalization, limit
// clamping, and the read lock, then delegates the locked body.
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

	return regionSearcher{store: s}.collect(q, limit), nil
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
	if r.IsNational() {
		return Region{}, ErrRegionNotFound
	}
	return r, nil
}

// DescendantRegions implements Store. Walks the parent->child relation
// via the parents map and returns the focus at index 0 followed by
// every reachable descendant. Excludes national-tier rows from both
// the seed and the recursion (pinned by the storetest contract).
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

// RollupMetrosFor implements Store. Returns the metro NODES (not their
// descendants) whose OWN orgs should additionally surface on the given
// state-equivalent region's detail page — the rollup_states relation,
// resolved at load. Empty slice for any region that is not a rollup
// target. National-tier metros are excluded. This relation is
// browse/descendant direction ONLY; AncestorRegions never consults it,
// so a leaf under the metro cannot leak the state via the ancestor walk.
// Sorted by ID for a deterministic order.
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

// GetOrgBySlug implements Store. Scans the in-memory orgs by slug,
// hydrates Regions like the wire contract expects, returns
// ErrOrgNotFound when no row matches.
//
// Behavioral note: MemStore has no approval gate — atlas.Org has no
// Status field, so the store cannot filter on it. Callers are
// responsible for only loading approved orgs into MemStore (the seed
// bundle carries only approved orgs; LoadDevFixtures does too, and
// hand-built fixtures should).
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
		// Clone Tags so the returned copy doesn't alias the stored
		// backing array under RLock (see OrgsForRegions). nil stays nil.
		out.Tags = append([]Tag(nil), out.Tags...)
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
			if !r.IsNational() {
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
		// Clone Tags so the returned copy doesn't alias the stored
		// backing array under RLock (see OrgsForRegions). nil stays nil.
		out.Tags = append([]Tag(nil), out.Tags...)
		candidates = append(candidates, out)
	}
	// Sort newest-first; ID DESC breaks ties so same-day orgs order
	// deterministically. Backfill yields only a handful of distinct
	// AddedAt dates, so the ID tiebreak does real work.
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

// Stats implements Store. Counts distinct orgs over s.orgs directly —
// never by summing per-region counts, which is what makes the total
// immune to both the browse-subset undercount and the multi-region
// double-count. See atlas.Stats for why that matters.
func (s *MemStore) Stats(_ context.Context) (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	orgsByCountry := map[Country]int{}
	totalOrgs := 0
	for _, org := range s.orgs {
		// Same "at least one non-national attachment" filter ListRecent
		// applies, so the two surfaces agree on what counts as a live org.
		// An org is credited to every country it touches; see CountryStats
		// on why that column need not sum to totalOrgs.
		hasNonNational := false
		countries := map[Country]struct{}{}
		for _, rid := range s.orgRegions[org.ID] {
			r, ok := s.regionsByID[rid]
			if !ok || r.ScopeTier == ScopeNational {
				continue
			}
			hasNonNational = true
			// Country is stamped by the seedfiles loader; hand-built
			// fixtures may leave it blank. Such an org still counts toward
			// the total, it just can't be attributed to a country row.
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

	// Derive the browse counts from ListRegions' own output rather than
	// re-implementing its predicate, so BrowseRegionCount cannot drift
	// from the list the SPA actually renders.
	browse := s.listRegionsLocked()
	regionsByCountry := map[Country]int{}
	for _, summary := range browse {
		// Mirrors the org-side rule above: a blank Country (possible only
		// in hand-built fixtures) still counts toward BrowseRegionCount
		// but has no country row to be attributed to, so the by_country
		// region_count columns may sum to less than BrowseRegionCount.
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

// regionsForOrg gathers the Region rows for an org's attachments,
// sorted ascending by region ID so the wire shape is deterministic
// (ordered by region ID). Must be called with s.mu held.
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
// hydrated sorted ascending by region ID (deterministic wire order).
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
		// Clone Tags so the returned copy's slice header does not alias
		// the stored backing array — a caller mutating the result must
		// not reach into the store (which is read under RLock here, with
		// no write coordination). Mirrors the defensive copy AddOrg makes
		// of the attachment ids. A nil Tags stays nil.
		org.Tags = append([]Tag(nil), org.Tags...)
		out = append(out, org)
	}
	return out, nil
}
