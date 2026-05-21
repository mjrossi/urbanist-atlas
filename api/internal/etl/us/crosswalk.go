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

// CrosswalkHUDBackfill produces PostalAnchor rows for ZIPs that the
// Census ZCTA crosswalk could not resolve. HUD's quarterly USPS
// ZIP-to-County crosswalk covers P.O. Box-only ZIPs, single-building
// ZIPs, APO/FPO ZIPs, and the long tail of ZIPs Census omits from
// ZCTA (~5-10k US ZIPs depending on vintage).
//
// Algorithm, per ZIP not already in zctaAnchors:
//
//  1. Group huds by ZIP; pick the row with max(TOT_RATIO) as the
//     primary-county row. TOT_RATIO (residential + business + other)
//     keeps P.O. Box-only ZIPs anchoring correctly, where a
//     RES_RATIO pick would be undefined (RES_RATIO == 0 across all
//     rows of a P.O. Box-only ZIP).
//  2. Walk the primary county FIPS through the existing fallback
//     chain (same order as Crosswalk, minus the place-leaf tier
//     since HUD doesn't carry a place GEOID):
//     - NYC borough (county ∈ {36005, 36047, 36061, 36081, 36085})
//     → "hud:nyc-borough"
//     - countyToLeaf (Cook, Lake-IN) → "hud:county-leaf"
//     - countyToMSA via msaSlugs → "hud:msa"
//     - stateFIPSToSlug via county[:2] → "hud:state"
//     - else: silently drop (no PostalAnchor emitted; the operator
//     can compare the input HUD row count against the returned
//     anchor count to see the drop count)
//
// Output is sorted by ZIP ASC so the merged CSV stays deterministic
// regardless of HUD's source ordering.
//
// CrosswalkHUDBackfill does NOT mutate or shadow the existing
// Crosswalk output — it is purely additive. ZCTA-resolved ZIPs always
// win any tie at the writer layer (see WritePostalCodesCSV).
func CrosswalkHUDBackfill(
	huds []HUDZipCounty,
	zctaAnchors []PostalAnchor,
	countyToMSA map[string]string,
	msaSlugs map[string]string, // CBSA code → slug
) []PostalAnchor {
	// Build set of ZIPs already resolved by ZCTA.
	resolved := make(map[string]struct{}, len(zctaAnchors))
	for _, a := range zctaAnchors {
		resolved[a.ZCTA] = struct{}{}
	}

	// Group HUD rows by ZIP, keeping only the max-TOT_RATIO row per
	// ZIP. Stable tiebreak on the first row we encountered keeps
	// the choice deterministic when two rows match exactly.
	type row struct {
		county string
		tot    float64
	}
	pick := make(map[string]row, len(huds))
	for _, h := range huds {
		if _, skip := resolved[h.ZIP]; skip {
			continue
		}
		cur, seen := pick[h.ZIP]
		if !seen || h.TotRatio > cur.tot {
			pick[h.ZIP] = row{county: h.County, tot: h.TotRatio}
		}
	}

	// Walk the fallback for each picked ZIP; collect placed anchors.
	zips := make([]string, 0, len(pick))
	for z := range pick {
		zips = append(zips, z)
	}
	sort.Strings(zips)

	out := make([]PostalAnchor, 0, len(zips))
	for _, z := range zips {
		county := pick[z].county
		anchor := PostalAnchor{ZCTA: z}
		switch {
		case nycBoroughCounty[county] != "":
			anchor.AnchorSlug = nycBoroughCounty[county]
			anchor.Reason = "hud:nyc-borough"
		case countyToLeaf[county] != "":
			anchor.AnchorSlug = countyToLeaf[county]
			anchor.Reason = "hud:county-leaf"
		case msaSlugs[countyToMSA[county]] != "":
			anchor.AnchorSlug = msaSlugs[countyToMSA[county]]
			anchor.Reason = "hud:msa"
		case len(county) >= 2 && stateFIPSToSlug[county[:2]] != "":
			anchor.AnchorSlug = stateFIPSToSlug[county[:2]]
			anchor.Reason = "hud:state"
		}
		if anchor.AnchorSlug == "" {
			continue
		}
		out = append(out, anchor)
	}
	return out
}
