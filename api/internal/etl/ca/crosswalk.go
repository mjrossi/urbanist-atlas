package ca

import (
	"sort"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// PostalAnchor aliases the shared etl.PostalAnchor: the smallest-anchor
// decision for one FSA. PostalCode is the FSA; Reason is one of
// "city-leaf", "cma", "cma-portion", "province", "unknown".
type PostalAnchor = etl.PostalAnchor

// crosswalk anchors each FSA at its curated city leaf (fsaToLeaf), else
// its max-overlap CMA (or that CMA's own-province portion), else its
// province. FSAs none of these resolve are dropped and counted as
// "unknown".
func crosswalk(fsas []fsaRow, cmaSlugByFSA, portionByCMA map[string]string) ([]PostalAnchor, map[string]int) {
	sort.Slice(fsas, func(i, j int) bool { return fsas[i].CFSAUID < fsas[j].CFSAUID })

	out := make([]PostalAnchor, 0, len(fsas))
	reasonCounts := map[string]int{}

	for _, f := range fsas {
		anchor := PostalAnchor{PostalCode: f.CFSAUID, Reason: "unknown"}
		switch {
		case fsaToLeaf[f.CFSAUID] != "":
			anchor.AnchorSlug = fsaToLeaf[f.CFSAUID]
			anchor.Reason = "city-leaf"
		default:
			if slug := cmaSlugByFSA[f.CFSAUID]; slug != "" {
				// A portion's ancestor walk reaches only its own province.
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
