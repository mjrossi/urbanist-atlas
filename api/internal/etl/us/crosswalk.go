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
//  2. Otherwise hand the ZCTA's primary county to countyResolver, which
//     walks NYC borough → county-leaf → MSA → state. (Reason values
//     come from the resolver: "nyc-borough", "county-leaf", "msa",
//     "state".) See county_resolver.go.
//  3. If neither path produces an anchor the ZCTA is skipped and
//     bucketed under "unknown" — reported in the summary so editorial
//     gaps don't silently leave coverage holes.
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
	resolver := newCountyResolver(countyToMSA, msaSlugs)

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
		case hasCounty:
			if slug, reason, ok := resolver.Resolve(county.CountyGEOID); ok {
				anchor.AnchorSlug = slug
				anchor.Reason = reason
			}
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
//  2. Walk the primary county FIPS through countyResolver — the same
//     4-tier chain Crosswalk uses after a failed city-place lookup
//     (NYC borough → county-leaf → MSA → state). HUD reasons are
//     prefixed "hud:" so a merged histogram can distinguish ZCTA-
//     sourced from HUD-sourced anchors.
//     If countyResolver returns ok=false (rare: APO/FPO 999xx, or a
//     county FIPS the curated graph doesn't cover) the anchor is
//     dropped and counted in the returned reason map under
//     "hud:unknown" so operators see the drop rate in the
//     orchestrator log instead of having to diff input vs. output
//     counts manually.
//
// Output is sorted by ZIP ASC so the merged CSV stays deterministic
// regardless of HUD's source ordering. The returned reason map is
// keyed by the full bucket label ("hud:msa", "hud:state",
// "hud:unknown", etc.) so it can be merged into the ZCTA-side
// reason histogram without collision.
//
// CrosswalkHUDBackfill does NOT mutate or shadow the existing
// Crosswalk output — it is purely additive. ZCTA-resolved ZIPs always
// win any tie at the writer layer (see WritePostalCodesCSV).
func CrosswalkHUDBackfill(
	huds []HUDZipCounty,
	zctaAnchors []PostalAnchor,
	countyToMSA map[string]string,
	msaSlugs map[string]string, // CBSA code → slug
) ([]PostalAnchor, map[string]int) {
	resolver := newCountyResolver(countyToMSA, msaSlugs)

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
	reasons := map[string]int{}
	for _, z := range zips {
		slug, reason, ok := resolver.Resolve(pick[z].county)
		if !ok {
			reasons["hud:unknown"]++
			continue
		}
		fullReason := "hud:" + reason
		reasons[fullReason]++
		out = append(out, PostalAnchor{
			ZCTA:       z,
			AnchorSlug: slug,
			Reason:     fullReason,
		})
	}
	return out, reasons
}
