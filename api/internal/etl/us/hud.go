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
	"fmt"
	"io"
	"strconv"
	"strings"
)

// HUDZipCounty is one row of HUD's quarterly USPS ZIP-County
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
type HUDZipCounty struct {
	ZIP      string
	County   string
	ResRatio float64
	BusRatio float64
	OthRatio float64
	TotRatio float64
}

// ParseHUDZipCounty reads HUD's USPS ZIP-to-County crosswalk CSV
// (downloaded from https://www.huduser.gov/portal/dataset/uspszip-api.html)
// and returns one entry per ZIP-County pair. HUD ships the file with a
// header row, quoted fields, and ratio columns as decimal strings; the
// order of fields is: ZIP, COUNTY, RES_RATIO, BUS_RATIO, OTH_RATIO,
// TOT_RATIO.
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
func ParseHUDZipCounty(r io.Reader) ([]HUDZipCounty, error) {
	cr := csv.NewReader(r)
	// Tolerate variable column counts; pick columns by index below.
	cr.FieldsPerRecord = -1

	out := make([]HUDZipCounty, 0, 50_000)
	headerSeen := false
	lineNum := 0
	for {
		fields, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse hud zip-county: read line %d: %w", lineNum+1, err)
		}
		lineNum++
		if !headerSeen {
			headerSeen = true
			continue
		}
		if isBlankRow(fields) {
			continue
		}
		if len(fields) < 6 {
			return nil, fmt.Errorf("parse hud zip-county: line %d: expected 6 columns, got %d", lineNum, len(fields))
		}
		zip := strings.TrimSpace(fields[0])
		county := strings.TrimSpace(fields[1])
		if zip == "" || county == "" {
			continue
		}
		zip = padLeadingZeros(zip, 5)
		county = padLeadingZeros(county, 5)
		res, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse hud zip-county: line %d: RES_RATIO %q: %w", lineNum, fields[2], err)
		}
		bus, err := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse hud zip-county: line %d: BUS_RATIO %q: %w", lineNum, fields[3], err)
		}
		oth, err := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse hud zip-county: line %d: OTH_RATIO %q: %w", lineNum, fields[4], err)
		}
		tot, err := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
		if err != nil {
			return nil, fmt.Errorf("parse hud zip-county: line %d: TOT_RATIO %q: %w", lineNum, fields[5], err)
		}
		out = append(out, HUDZipCounty{
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
// ParseHUDZipCounty doc for why this matters for ZIP / county FIPS.
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
