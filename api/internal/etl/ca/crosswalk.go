package ca

import (
	"sort"
)

// PostalAnchor records the smallest-anchor decision for one FSA.
// Reason is one of "city-leaf", "cma", "province", "unknown".
type PostalAnchor struct {
	FSA        string
	AnchorSlug string
	Reason     string
}

// Crosswalk runs the smallest-anchor algorithm over the FSA rows:
//
//  1. Exact FSA → curated city leaf (fsaToLeaf) — e.g., M5V → toronto.
//  2. FSA → curated CMA slug via the max-overlap spatial join
//     (cmaSlugByFSA, built in ca.go from SpatialJoinFSAToCMA) — e.g.,
//     V8W → victoria-cma. Multi-province CMAs route to the FSA's own
//     province portion.
//  3. Province via PRUID (provinceUIDToSlug) — e.g., A0A → nl-province.
//  4. Otherwise "unknown" (FSA is dropped from the output and counted
//     in the unknown bucket).
//
// cmaSlugByFSA contains only slugs the caller verified are in the
// generated CMA list, so an FSA whose max-overlap CMA was somehow not
// emitted falls through to province rather than dangling.
func Crosswalk(fsas []FSARow, cmaSlugByFSA map[string]string, portionByCMA map[string]string) ([]PostalAnchor, map[string]int) {
	sort.Slice(fsas, func(i, j int) bool { return fsas[i].CFSAUID < fsas[j].CFSAUID })

	out := make([]PostalAnchor, 0, len(fsas))
	reasonCounts := map[string]int{}

	for _, f := range fsas {
		anchor := PostalAnchor{FSA: f.CFSAUID, Reason: "unknown"}
		switch {
		case fsaToLeaf[f.CFSAUID] != "":
			anchor.AnchorSlug = fsaToLeaf[f.CFSAUID]
			anchor.Reason = "city-leaf"
		default:
			if slug := cmaSlugByFSA[f.CFSAUID]; slug != "" {
				// Multi-province CMA: route to the FSA's own-province
				// portion so the ancestor walk reaches only its own
				// province (leak-free). Single-province CMAs have no portion
				// entry and keep the bare umbrella slug.
				if p := portionByCMA[slug+":"+f.PRUID]; p != "" {
					anchor.AnchorSlug = p
					anchor.Reason = "cma-portion"
				} else {
					anchor.AnchorSlug = slug
					anchor.Reason = "cma"
				}
			} else if slug := provinceUIDToSlug[f.PRUID]; slug != "" {
				anchor.AnchorSlug = slug
				anchor.Reason = "province"
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
