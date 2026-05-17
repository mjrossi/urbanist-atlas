package atlas

import (
	"context"
	"fmt"
	"sort"
)

// Lookup is the core search operation: given a postal code, return the
// local + regional organizations advocating in that area.
//
// Algorithm:
//  1. Resolve the postal code to its geographic regions
//     (city / county / metro / state). 404 if unknown.
//  2. Fetch all approved orgs joined to any of those region IDs.
//  3. For each matched org, bucket it as Local if any of its matching
//     regions has ScopeTier=local; otherwise Regional. An org is never
//     shown in both buckets.
//  4. Within each bucket, sort by the most-specific region kind the
//     org serves (city → county → metro → state/province →
//     multi-state), with alphabetical name as the tiebreaker.
func Lookup(ctx context.Context, store Store, query LookupQuery) (LookupResult, error) {
	rpc, err := store.ResolvePostalCode(ctx, query.Country, query.PostalCode)
	if err != nil {
		return LookupResult{}, err
	}

	regionIDs := rpc.RegionIDs()
	orgs, err := store.OrgsForRegions(ctx, regionIDs)
	if err != nil {
		return LookupResult{}, fmt.Errorf("atlas: orgs lookup: %w", err)
	}

	matched := make(map[int64]bool, len(regionIDs))
	for _, id := range regionIDs {
		matched[id] = true
	}

	local := []Org{}
	regional := []Org{}
	for _, org := range orgs {
		if hasLocalMatch(org, matched) {
			local = append(local, org)
		} else {
			regional = append(regional, org)
		}
	}

	sortOrgs(local)
	sortOrgs(regional)

	return LookupResult{
		Query:              query,
		ResolvedPlaceLabel: placeLabel(rpc),
		Local:              local,
		Regional:           regional,
	}, nil
}

// hasLocalMatch is true when any of the org's regions both matched the
// postal code AND is classified as local.
func hasLocalMatch(org Org, matched map[int64]bool) bool {
	for _, r := range org.Regions {
		if matched[r.ID] && r.ScopeTier == ScopeLocal {
			return true
		}
	}
	return false
}

// regionKindRank gives a sort key for region granularity — lower means
// more specific (i.e. should appear higher in results).
func regionKindRank(k RegionKind) int {
	switch k {
	case RegionCity:
		return 0
	case RegionCounty:
		return 1
	case RegionMetro:
		return 2
	case RegionState, RegionProvince:
		return 3
	case RegionMultiState:
		return 4
	case RegionCountry:
		return 5
	}
	return 99
}

func mostSpecificRank(org Org) int {
	best := 99
	for _, r := range org.Regions {
		if rk := regionKindRank(r.Kind); rk < best {
			best = rk
		}
	}
	return best
}

func sortOrgs(orgs []Org) {
	sort.SliceStable(orgs, func(i, j int) bool {
		ri := mostSpecificRank(orgs[i])
		rj := mostSpecificRank(orgs[j])
		if ri != rj {
			return ri < rj
		}
		return orgs[i].Name < orgs[j].Name
	})
}

// placeLabel renders a human-readable header like
// "Brooklyn, NY — New York Metro" using whichever region pointers the
// postal code resolved.
func placeLabel(rpc ResolvedPostalCode) string {
	var city, state, metro string
	if rpc.City != nil {
		city = rpc.City.Name
	}
	if rpc.State != nil {
		state = rpc.State.Name
	}
	if rpc.Metro != nil {
		metro = rpc.Metro.Name
	}

	head := ""
	switch {
	case city != "" && state != "":
		head = city + ", " + state
	case city != "":
		head = city
	case state != "":
		head = state
	}

	if metro != "" && metro != head {
		if head == "" {
			return metro
		}
		return head + " — " + metro
	}
	return head
}
