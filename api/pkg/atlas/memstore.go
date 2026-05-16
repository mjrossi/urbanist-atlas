package atlas

import (
	"context"
	"strings"
	"sync"
)

// MemStore is an in-memory Store implementation. It exists for two
// audiences:
//
//   - Unit tests in pkg/atlas (and in any consumer that wants to test
//     against a real Store without spinning up Postgres).
//   - Local development before the Postgres store is wired up — see
//     LoadDevFixtures for a populated demo set.
//
// MemStore is safe for concurrent use.
type MemStore struct {
	mu         sync.RWMutex
	regions    map[int64]Region
	orgs       []Org
	orgRegions map[int64][]int64             // orgID → regionIDs the org serves
	postal     map[string]ResolvedPostalCode // key: postalKey(country, code)
}

// NewMemStore returns an empty MemStore. Use AddRegion / AddPostalCode
// / AddOrg to populate it, or call LoadDevFixtures to install the
// built-in demo data.
func NewMemStore() *MemStore {
	return &MemStore{
		regions:    map[int64]Region{},
		orgRegions: map[int64][]int64{},
		postal:     map[string]ResolvedPostalCode{},
	}
}

// AddRegion registers a region. Later calls with the same ID overwrite
// the earlier value.
func (s *MemStore) AddRegion(r Region) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.regions[r.ID] = r
}

// AddOrg registers an organization along with the IDs of the regions
// it serves. The org's Regions field is ignored (and overwritten on
// read); pass the region IDs explicitly.
func (s *MemStore) AddOrg(org Org, regionIDs []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org.Regions = nil
	s.orgs = append(s.orgs, org)
	s.orgRegions[org.ID] = append([]int64(nil), regionIDs...)
}

// AddPostalCode registers a postal-code → regions mapping. The Code
// field is normalized using the same rules ResolvePostalCode uses.
func (s *MemStore) AddPostalCode(rpc ResolvedPostalCode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rpc.Code = normalizePostalCode(rpc.Country, rpc.Code)
	s.postal[postalKey(rpc.Country, rpc.Code)] = rpc
}

// ResolvePostalCode implements Store.
func (s *MemStore) ResolvePostalCode(_ context.Context, country Country, postalCode string) (ResolvedPostalCode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rpc, ok := s.postal[postalKey(country, postalCode)]
	if !ok {
		return ResolvedPostalCode{}, ErrPostalCodeNotFound
	}
	return rpc, nil
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
		matched := false
		for _, rid := range orgRegionIDs {
			if wanted[rid] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		regions := make([]Region, 0, len(orgRegionIDs))
		for _, rid := range orgRegionIDs {
			if r, ok := s.regions[rid]; ok {
				regions = append(regions, r)
			}
		}
		org.Regions = regions
		out = append(out, org)
	}
	return out, nil
}

// normalizePostalCode applies the canonicalization rules used by both
// AddPostalCode and ResolvePostalCode so the two stay in sync.
// Canadian inputs are truncated to the first three characters (FSA).
func normalizePostalCode(country Country, code string) string {
	c := strings.ToUpper(strings.ReplaceAll(code, " ", ""))
	if country == CountryCA && len(c) > 3 {
		c = c[:3]
	}
	return c
}

func postalKey(country Country, code string) string {
	return string(country) + ":" + normalizePostalCode(country, code)
}
