package us

import (
	"sort"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// PostalAnchor aliases the shared etl.PostalAnchor: the smallest-anchor
// decision for one ZIP. Reason labels which fallback tier won.
type PostalAnchor = etl.PostalAnchor

// crosswalk anchors every ZCTA in zctaPlaces ∪ zctaCounties: at the
// curated city leaf for its primary place if there is one, otherwise at
// whatever countyResolver picks for its primary county. ZCTAs neither
// path resolves are dropped and counted under "unknown" so editorial
// gaps show up in the summary.
func crosswalk(
	zctaPlaces map[string]zctaPlace,
	zctaCounties map[string]zctaCounty,
	countyToMSA map[string]string,
	msaSlugs map[string]string, // CBSA code → umbrella slug
	portionSlugs map[string]string, // "CBSAcode:stateFIPS" → portion slug
) ([]PostalAnchor, map[string]int) {
	resolver := newCountyResolver(countyToMSA, msaSlugs, portionSlugs)

	zctas := map[string]struct{}{}
	for z := range zctaPlaces {
		zctas[z] = struct{}{}
	}
	for z := range zctaCounties {
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
		anchor := PostalAnchor{PostalCode: zcta, Reason: "unknown"}

		place, hasPlace := zctaPlaces[zcta]
		county, hasCounty := zctaCounties[zcta]

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

// crosswalkHUDBackfill anchors the ZIPs Census leaves out of ZCTA
// (P.O. Box-only, single-building, APO/FPO: ~5-10k per vintage) from
// HUD's USPS ZIP-County crosswalk. Each ZIP not already in zctaAnchors
// goes through countyResolver from its primary county. Reasons carry a
// "hud:" prefix, unresolved ZIPs count as "hud:unknown", and output is
// sorted by ZIP so HUD's row order can't leak into the CSV.
func crosswalkHUDBackfill(
	huds []hudZipCounty,
	zctaAnchors []PostalAnchor,
	countyToMSA map[string]string,
	msaSlugs map[string]string, // CBSA code → umbrella slug
	portionSlugs map[string]string, // "CBSAcode:stateFIPS" → portion slug
) ([]PostalAnchor, map[string]int) {
	resolver := newCountyResolver(countyToMSA, msaSlugs, portionSlugs)

	resolved := make(map[string]struct{}, len(zctaAnchors))
	for _, a := range zctaAnchors {
		resolved[a.PostalCode] = struct{}{}
	}

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
			PostalCode: z,
			AnchorSlug: slug,
			Reason:     fullReason,
		})
	}
	return out, reasons
}

// hudPrimaryCounty returns ZIP → the county FIPS with the largest
// TOT_RATIO. RES_RATIO would not do: it is 0 on every row of a P.O.
// Box-only ZIP. Ties keep the first row seen, so the pick is
// deterministic.
func hudPrimaryCounty(huds []hudZipCounty) map[string]string {
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

// reconcileCTLegacyCounties re-anchors, in place, CT ZCTA anchors that
// fell through to the bare `ct` state because their county is a retired
// legacy FIPS (see ctLegacyCounties). Each goes through countyResolver
// again with HUD's current-vintage primary county, and a finer result
// replaces the state, tagged "ct-reconciled:<tier>". Anchors that won a
// finer tier are left alone. It returns a reason→count histogram and is
// a no-op when huds is empty.
func reconcileCTLegacyCounties(
	zctaAnchors []PostalAnchor,
	zctaCounties map[string]zctaCounty,
	huds []hudZipCounty,
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

		// Any finer tier (city-leaf, etc.) already won and must stand.
		if a.Reason != "state" {
			continue
		}
		county, ok := zctaCounties[a.PostalCode]
		if !ok {
			continue
		}
		if _, legacy := ctLegacyCounties[county.CountyGEOID]; !legacy {
			continue
		}

		hudFIPS, ok := primary[a.PostalCode]
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
			// A candidate always sits at the bare `ct` anchor, so a slug
			// match means HUD landed on the same state.
			counts["ct-unchanged:"+reason]++
			continue
		}
		a.AnchorSlug = slug
		a.Reason = "ct-reconciled:" + reason
		counts["ct-reconciled:"+reason]++
	}
	return counts
}
