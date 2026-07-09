// HUD ZIP-County crosswalk reader — the additive backfill source for
// operational USPS ZIPs Census ZCTA omits (P.O. Box-only, single-
// building, APO/FPO). See the package doc on us.go for how this
// source plugs into the two-source pipeline, and
// docs/superpowers/specs/2026-05-19-postal-coverage-design.md
// §"Two-source pipeline" for the design rationale + the worked
// 20811 → washington-dc-metro example.

package us

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// hudZipCounty is one row of HUD's quarterly USPS ZIP-County
// crosswalk. HUD publishes one row per (ZIP, COUNTY) combination —
// multi-county ZIPs span multiple rows. The four ratio columns
// (residential / business / other / total) share the implicit
// constraint that summing across all rows for a given ZIP yields ~1.0
// per ratio kind.
//
// We anchor a ZIP by the row with max(TOT_RATIO) — the total-
// deliveries share — rather than max(RES_RATIO). P.O. Box-only ZIPs
// (e.g., 20811) have RES_RATIO == 0 across every row, so a
// residential-share pick would mis-anchor them; TOT_RATIO weights
// residential + business + other together and always sums to a
// meaningful primary county.
type hudZipCounty struct {
	ZIP      string
	County   string
	ResRatio float64
	BusRatio float64
	OthRatio float64
	TotRatio float64
}

// parseHUDZipCounty reads HUD's USPS ZIP-to-County crosswalk CSV
// (downloaded from https://www.huduser.gov/portal/dataset/uspszip-api.html)
// and returns one entry per ZIP-County pair. HUD ships the file with a
// header row, quoted fields, and ratio columns as decimal strings. The
// real-world layout is ZIP, COUNTY, USPS_ZIP_PREF_CITY,
// USPS_ZIP_PREF_STATE, RES_RATIO, BUS_RATIO, OTH_RATIO, TOT_RATIO, but
// we resolve every column by header name so HUD reordering columns or
// dropping the city/state pair won't silently mis-anchor ZIPs.
//
// Blank lines are skipped silently. Trailing whitespace in ZIP and
// COUNTY columns is trimmed (HUD occasionally pads). ZIP and COUNTY
// are left-padded to 5 characters with leading zeros — HUD ships them
// quoted to preserve the leading zero, but an operator round-tripping
// the CSV through Excel can inadvertently coerce them to integers,
// which strips leading zeros ("00601" → "601"). Padding defensively
// here keeps Puerto Rico and New England ZIPs / county FIPS from
// silently mis-anchoring downstream. A malformed ratio column returns
// an error wrapping the offending line number — silently dropping a
// row would mean a silent coverage hole.
func parseHUDZipCounty(r io.Reader) ([]hudZipCounty, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("parse hud zip-county: read header: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToUpper(strings.TrimSpace(h))] = i
	}
	required := []string{"ZIP", "COUNTY", "RES_RATIO", "BUS_RATIO", "OTH_RATIO", "TOT_RATIO"}
	for _, name := range required {
		if _, ok := idx[name]; !ok {
			return nil, fmt.Errorf("parse hud zip-county: header missing required column %q; got %v", name, header)
		}
	}
	zipCol, countyCol := idx["ZIP"], idx["COUNTY"]
	resCol, busCol := idx["RES_RATIO"], idx["BUS_RATIO"]
	othCol, totCol := idx["OTH_RATIO"], idx["TOT_RATIO"]
	maxCol := zipCol
	for _, c := range []int{countyCol, resCol, busCol, othCol, totCol} {
		if c > maxCol {
			maxCol = c
		}
	}

	parseRatio := func(line int, name string, fields []string, col int) (float64, error) {
		v, err := strconv.ParseFloat(strings.TrimSpace(fields[col]), 64)
		if err != nil {
			return 0, fmt.Errorf("parse hud zip-county: line %d: %s %q: %w", line, name, fields[col], err)
		}
		return v, nil
	}

	out := make([]hudZipCounty, 0, 50_000)
	lineNum := 1
	for {
		fields, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse hud zip-county: read line %d: %w", lineNum+1, err)
		}
		lineNum++
		if isBlankRow(fields) {
			continue
		}
		if len(fields) <= maxCol {
			return nil, fmt.Errorf("parse hud zip-county: line %d: row has %d columns, header declared at least %d", lineNum, len(fields), maxCol+1)
		}
		zip := strings.TrimSpace(fields[zipCol])
		county := strings.TrimSpace(fields[countyCol])
		if zip == "" || county == "" {
			continue
		}
		zip = padLeadingZeros(zip, 5)
		county = padLeadingZeros(county, 5)
		res, err := parseRatio(lineNum, "RES_RATIO", fields, resCol)
		if err != nil {
			return nil, err
		}
		bus, err := parseRatio(lineNum, "BUS_RATIO", fields, busCol)
		if err != nil {
			return nil, err
		}
		oth, err := parseRatio(lineNum, "OTH_RATIO", fields, othCol)
		if err != nil {
			return nil, err
		}
		tot, err := parseRatio(lineNum, "TOT_RATIO", fields, totCol)
		if err != nil {
			return nil, err
		}
		out = append(out, hudZipCounty{
			ZIP:      zip,
			County:   county,
			ResRatio: res,
			BusRatio: bus,
			OthRatio: oth,
			TotRatio: tot,
		})
	}
	return out, nil
}

// padLeadingZeros left-pads s with '0' until it reaches width n. If s
// is already >= n characters it is returned unchanged. See
// parseHUDZipCounty doc for why this matters for ZIP / county FIPS.
func padLeadingZeros(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat("0", n-len(s)) + s
}

func isBlankRow(fields []string) bool {
	for _, f := range fields {
		if strings.TrimSpace(f) != "" {
			return false
		}
	}
	return true
}
