package us

import (
	"sort"
)

// PostalAnchor is the smallest-anchor decision for one ZCTA after
// running the crosswalk. AnchorSlug is what the postal_codes row
// points at; Reason is a debug-friendly label of which fallback won
// ("city-leaf", "nyc-borough", "county-leaf", "msa", "state",
// "unknown").
type PostalAnchor struct {
	ZCTA       string
	AnchorSlug string
	Reason     string
}

// Crosswalk runs the smallest-anchor algorithm for every ZCTA in
// zctaPlace ∪ zctaCounty:
//
//  1. If the ZCTA's primary place has a curated city-leaf entry in
//     placeToLeaf → anchor = leaf slug. (Reason: "city-leaf".)
//  2. Else if the ZCTA's primary county is one of the 5 NYC borough
//     counties → anchor = borough slug. (Reason: "nyc-borough".)
//  3. Else if the ZCTA's primary county matches a curated non-NYC
//     county leaf (Cook, Lake-IN) → anchor = county slug. (Reason:
//     "county-leaf".)
//  4. Else if the county participates in an MSA → anchor = MSA slug
//     (looked up via msaSlugs[countyToMSA[county]]). (Reason: "msa".)
//  5. Else if the county's state FIPS resolves → anchor = state slug.
//     (Reason: "state".)
//  6. Otherwise the ZCTA is skipped — reported in the summary so
//     editorial gaps don't silently leave coverage holes.
//
// msaSlugs is built by the caller from the post-override slug
// assignments (see ApplyOverrides + AssignMSASlugs) so this layer
// doesn't have to know about the override file format.
func Crosswalk(
	zctaPlace map[string]ZCTAPlace,
	zctaCounty map[string]ZCTACounty,
	countyToMSA map[string]string,
	msaSlugs map[string]string, // CBSA code → slug
) ([]PostalAnchor, map[string]int) {
	zctas := map[string]struct{}{}
	for z := range zctaPlace {
		zctas[z] = struct{}{}
	}
	for z := range zctaCounty {
		zctas[z] = struct{}{}
	}
	sorted := make([]string, 0, len(zctas))
	for z := range zctas {
		sorted = append(sorted, z)
	}
	sort.Strings(sorted)

	out := make([]PostalAnchor, 0, len(sorted))
	reasonCounts := map[string]int{}

	for _, zcta := range sorted {
		anchor := PostalAnchor{ZCTA: zcta, Reason: "unknown"}

		place, hasPlace := zctaPlace[zcta]
		county, hasCounty := zctaCounty[zcta]

		switch {
		case hasPlace && placeToLeaf[place.PlaceGEOID] != "":
			anchor.AnchorSlug = placeToLeaf[place.PlaceGEOID]
			anchor.Reason = "city-leaf"
		case hasCounty && nycBoroughCounty[county.CountyGEOID] != "":
			anchor.AnchorSlug = nycBoroughCounty[county.CountyGEOID]
			anchor.Reason = "nyc-borough"
		case hasCounty && countyToLeaf[county.CountyGEOID] != "":
			anchor.AnchorSlug = countyToLeaf[county.CountyGEOID]
			anchor.Reason = "county-leaf"
		case hasCounty && msaSlugs[countyToMSA[county.CountyGEOID]] != "":
			anchor.AnchorSlug = msaSlugs[countyToMSA[county.CountyGEOID]]
			anchor.Reason = "msa"
		case hasCounty && stateFIPSToSlug[county.CountyGEOID[:2]] != "":
			anchor.AnchorSlug = stateFIPSToSlug[county.CountyGEOID[:2]]
			anchor.Reason = "state"
		}

		if anchor.AnchorSlug == "" {
			reasonCounts["unknown"]++
			continue
		}
		reasonCounts[anchor.Reason]++
		out = append(out, anchor)
	}
	return out, reasonCounts
}
