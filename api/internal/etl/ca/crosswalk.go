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
//  2. FSA prefix (2 then 1 char) → curated CMA slug
//     (fsaPrefixToCMA) — e.g., M3K → toronto-cma via the "M" rule.
//     The CMA must be in knownCMASlugs (i.e., one we actually wrote
//     to regions_ca_cmas.toml).
//  3. Province via PRUID (provinceUIDToSlug) — e.g., A0A → nl-province.
//  4. Otherwise "unknown" (FSA is dropped from the output and counted
//     in the unknown bucket).
//
// knownCMASlugs is built by the caller from the generated CMA list so
// prefix overrides that point at a CMA we filtered out don't silently
// fail.
func Crosswalk(fsas []FSARow, knownCMASlugs map[string]bool, portionByCMA map[string]string) ([]PostalAnchor, map[string]int) {
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
			if slug := lookupCMAPrefix(f.CFSAUID, knownCMASlugs); slug != "" {
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

// lookupCMAPrefix tries the 2-character prefix first, then the
// 1-character prefix. Returns "" if no match (or the match points at
// a CMA we didn't generate, e.g., the curated slug doesn't exist).
func lookupCMAPrefix(fsa string, known map[string]bool) string {
	if len(fsa) >= 2 {
		if slug := fsaPrefixToCMA[fsa[:2]]; slug != "" && known[slug] {
			return slug
		}
	}
	if len(fsa) >= 1 {
		if slug := fsaPrefixToCMA[fsa[:1]]; slug != "" && known[slug] {
			return slug
		}
	}
	return ""
}
