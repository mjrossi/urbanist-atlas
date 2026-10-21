package ca

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// cma is one Census Metropolitan Area from the CMA boundary file's DBF
// table. The file also lists Census Agglomerations (CMATYPE 'D'); only
// type 'B' rows are CMAs.
type cma struct {
	// UID is the 3-digit CMA code (e.g. "535", Toronto). Ottawa-Gatineau
	// (505) has a source row per province but is one cma.
	UID string
	// Name is CMANAME with qualifiers and the French half stripped; see
	// cleanCMAName.
	Name string
	// ProvinceUIDs are the 2-digit province codes the CMA spans, in
	// source order.
	ProvinceUIDs []string
}

// parseCMAs reads the CMA boundary zip's DBF table into one cma per
// type-'B' CMAUID, sorted by UID.
func parseCMAs(zipPath string) ([]cma, error) {
	dbf, closer, err := openDBFFromZip(zipPath, ".dbf")
	if err != nil {
		return nil, fmt.Errorf("parse cmas: %w", err)
	}
	defer closer()

	agg := map[string]*cma{}
	uidOrder := []string{}
	for {
		row, err := dbf.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse cmas: %w", err)
		}
		if row["CMATYPE"] != "B" {
			continue
		}
		uid := row["CMAUID"]
		if uid == "" {
			continue
		}
		name := cleanCMAName(row["CMANAME"])
		pruid := row["PRUID"]
		if existing, ok := agg[uid]; ok {
			existing.ProvinceUIDs = append(existing.ProvinceUIDs, pruid)
			continue
		}
		agg[uid] = &cma{
			UID:          uid,
			Name:         name,
			ProvinceUIDs: []string{pruid},
		}
		uidOrder = append(uidOrder, uid)
	}
	out := make([]cma, 0, len(uidOrder))
	for _, uid := range uidOrder {
		out = append(out, *agg[uid])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out, nil
}

// cleanCMAName drops a parenthetical qualifier ("(Ontario part / partie
// de l'Ontario)") and the French half of a bilingual name
// ("Greater Sudbury / Grand Sudbury" → "Greater Sudbury").
func cleanCMAName(raw string) string {
	if i := strings.Index(raw, "("); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, " / "); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}
