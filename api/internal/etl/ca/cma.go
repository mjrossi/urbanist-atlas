package ca

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// CMA is one Census Metropolitan Area row, derived from the StatsCan
// CMA boundary file's DBF attribute table. We keep only the fields the
// region-generation logic uses; the much-larger shapefile geometry is
// ignored.
//
// Note that the StatsCan CMA file includes both Census Metropolitan
// Areas (CMATYPE='B', population ≥100,000) and Census Agglomerations
// (CMATYPE='D', smaller). Only type 'B' is treated as a CMA for the
// purpose of building regions_ca_cmas.toml.
type CMA struct {
	// UID is the 3-digit Census CMA code (e.g., "535" for Toronto).
	// Multi-province CMAs (Ottawa-Gatineau, code 505) appear once per
	// province in the source file but collapse to a single CMA here.
	UID string
	// Name is the canonical CMA name with French/English variants and
	// parenthetical qualifiers stripped (e.g.,
	// "Greater Sudbury / Grand Sudbury" → "Greater Sudbury";
	// "Ottawa - Gatineau (Ontario part / ...)" → "Ottawa - Gatineau").
	Name string
	// ProvinceUIDs are the 2-digit province codes the CMA spans, in
	// the order encountered in the source. Single-province CMAs have
	// one entry; Ottawa-Gatineau has two.
	ProvinceUIDs []string
}

// ParseCMAs reads the StatsCan CMA boundary file zip (downloaded from
// https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lcma000b21a_e.zip)
// and returns one CMA per unique UID. Only CMATYPE='B' rows (true
// CMAs, pop ≥100k) are kept; every other CMATYPE (e.g. 'D', Census
// Agglomerations) is filtered out.
func ParseCMAs(zipPath string) ([]CMA, error) {
	dbf, closer, err := openDBFFromZip(zipPath, ".dbf")
	if err != nil {
		return nil, fmt.Errorf("parse cmas: %w", err)
	}
	defer closer()

	agg := map[string]*CMA{}
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
		agg[uid] = &CMA{
			UID:          uid,
			Name:         name,
			ProvinceUIDs: []string{pruid},
		}
		uidOrder = append(uidOrder, uid)
	}
	out := make([]CMA, 0, len(uidOrder))
	for _, uid := range uidOrder {
		out = append(out, *agg[uid])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out, nil
}

// cleanCMAName trims the parenthetical "(Ontario part / partie de
// l'Ontario)" sort of qualifiers and keeps the first half of an
// English / French split (e.g., "Greater Sudbury / Grand Sudbury"
// → "Greater Sudbury").
func cleanCMAName(raw string) string {
	if i := strings.Index(raw, "("); i >= 0 {
		raw = raw[:i]
	}
	if i := strings.Index(raw, " / "); i >= 0 {
		raw = raw[:i]
	}
	return strings.TrimSpace(raw)
}
