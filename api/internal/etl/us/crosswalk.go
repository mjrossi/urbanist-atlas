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
	msaSlugs map[string]string, // CBSA code → umbrella slug
	portionSlugs map[string]string, // "CBSAcode:stateFIPS" → portion slug
) ([]PostalAnchor, map[string]int) {
	resolver := newCountyResolver(countyToMSA, msaSlugs, portionSlugs)

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
	msaSlugs map[string]string, // CBSA code → umbrella slug
	portionSlugs map[string]string, // "CBSAcode:stateFIPS" → portion slug
) ([]PostalAnchor, map[string]int) {
	resolver := newCountyResolver(countyToMSA, msaSlugs, portionSlugs)

	// Build set of ZIPs already resolved by ZCTA.
	resolved := make(map[string]struct{}, len(zctaAnchors))
	for _, a := range zctaAnchors {
		resolved[a.ZCTA] = struct{}{}
	}

	// Pick the primary (max-TOT_RATIO) county per ZIP, then walk the
	// fallback for each ZIP not already resolved by ZCTA.
	primary := hudPrimaryCounty(huds)
	zips := make([]string, 0, len(primary))
	for z := range primary {
		if _, skip := resolved[z]; skip {
			continue
		}
		zips = append(zips, z)
	}
	sort.Strings(zips)

	out := make([]PostalAnchor, 0, len(zips))
	reasons := map[string]int{}
	for _, z := range zips {
		slug, reason, ok := resolver.Resolve(primary[z])
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

// hudPrimaryCounty groups HUD rows by ZIP and returns ZIP → the county
// FIPS with the largest TOT_RATIO (residential + business + other). Using
// TOT_RATIO (not RES_RATIO) keeps P.O. Box-only ZIPs anchoring correctly,
// where every row's RES_RATIO is 0. The tiebreak is stable on the first
// row encountered (strict `>`), so the choice is deterministic when two
// rows match exactly.
//
// Shared by CrosswalkHUDBackfill (which then drops ZIPs already resolved
// by ZCTA) and ReconcileCTLegacyCounties (which uses it to repair CT
// ZCTA anchors stranded by the county-vintage gap).
func hudPrimaryCounty(huds []HUDZipCounty) map[string]string {
	type row struct {
		county string
		tot    float64
	}
	pick := make(map[string]row, len(huds))
	for _, h := range huds {
		cur, seen := pick[h.ZIP]
		if !seen || h.TotRatio > cur.tot {
			pick[h.ZIP] = row{county: h.County, tot: h.TotRatio}
		}
	}
	out := make(map[string]string, len(pick))
	for z, r := range pick {
		out[z] = r.county
	}
	return out
}

// ReconcileCTLegacyCounties repairs the Connecticut county-vintage gap
// in place on the ZCTA anchor slice.
//
// The 2020 ZCTA→county relationship file keys Connecticut ZCTAs by the
// retired legacy counties (FIPS 09001–09015), but countyToMSA — built
// from the July-2023 CBSA delineation — keys CT metros by the planning
// regions that replaced them in 2022 (FIPS 09110–09190). The mismatch
// strands every CT ZCTA ZIP at the bare `ct` state anchor. (HUD's
// crosswalk already uses the current planning-region FIPS, which is why
// HUD-sourced CT P.O.-box ZIPs resolve to their metro correctly.)
//
// For each ZCTA anchor whose source county GEOID (from zctaCounty) is a
// retired CT legacy county AND which resolved only to the state, this
// re-resolves the ZIP through HUD's current-vintage primary county and
// the standard county fallback chain. A more specific HUD result (a
// metro, county-leaf, etc.) replaces the anchor's slug, tagged
// "ct-reconciled:<tier>". Anchors that already won a finer tier (e.g.
// the bridgeport city-leaf via the place crosswalk) are left untouched,
// preserving the smallest-anchor invariant. ZIPs HUD can't improve
// (rural CT with no MSA, or ZIPs absent from HUD) keep their state
// anchor.
//
// Mutates zctaAnchors in place; returns a reason→count histogram for
// operator logging (e.g. "ct-reconciled:msa", "ct-skip:no-hud").
//
// No-op when huds is empty (no HUD source staged), matching the
// orchestrator's graceful ZCTA-only degradation.
func ReconcileCTLegacyCounties(
	zctaAnchors []PostalAnchor,
	zctaCounty map[string]ZCTACounty,
	huds []HUDZipCounty,
	countyToMSA map[string]string,
	msaSlugs map[string]string, // CBSA code → umbrella slug
	portionSlugs map[string]string, // "CBSAcode:stateFIPS" → portion slug
) map[string]int {
	counts := map[string]int{}
	if len(huds) == 0 {
		return counts
	}
	resolver := newCountyResolver(countyToMSA, msaSlugs, portionSlugs)
	primary := hudPrimaryCounty(huds)

	for i := range zctaAnchors {
		a := &zctaAnchors[i]

		// Only ZCTAs that fell through to the bare state are candidates;
		// any finer tier (city-leaf, etc.) already won and must stand.
		if a.Reason != "state" {
			continue
		}
		county, ok := zctaCounty[a.ZCTA]
		if !ok {
			continue
		}
		if _, legacy := ctLegacyCounties[county.CountyGEOID]; !legacy {
			continue
		}

		hudFIPS, ok := primary[a.ZCTA]
		if !ok {
			counts["ct-skip:no-hud"]++
			continue
		}
		slug, reason, ok := resolver.Resolve(hudFIPS)
		if !ok {
			counts["ct-skip:hud-unresolved"]++
			continue
		}
		if slug == a.AnchorSlug {
			// HUD's current county also resolves to the state (rural CT
			// with no MSA) — nothing gained. Comparing slugs is sufficient
			// here because a candidate is always at the bare state anchor
			// (Reason == "state" above), whose slug is invariably `ct`; a
			// slug match therefore means HUD landed on the same state.
			counts["ct-unchanged:"+reason]++
			continue
		}
		a.AnchorSlug = slug
		a.Reason = "ct-reconciled:" + reason
		counts["ct-reconciled:"+reason]++
	}
	return counts
}
