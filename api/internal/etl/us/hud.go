// HUD's ZIP-County crosswalk is the backfill source for operational ZIPs
// Census ZCTA omits; see docs/superpowers/specs/2026-05-19-postal-coverage-design.md
// "Two-source pipeline".

package us

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// hudZipCounty is one row of HUD's quarterly USPS ZIP-County crosswalk,
// which has a row per (ZIP, county) pair. For each ZIP, each ratio
// column sums to about 1.0 across its rows.
type hudZipCounty struct {
	ZIP      string
	County   string
	ResRatio float64
	BusRatio float64
	OthRatio float64
	TotRatio float64
}

// parseHUDZipCounty reads HUD's ZIP-County crosswalk CSV into one entry
// per ZIP-county pair. Columns are found by header name so a reordered
// or trimmed HUD layout can't silently mis-anchor ZIPs. ZIP and COUNTY
// are zero-padded to 5 characters: HUD quotes them to keep the leading
// zero, but a round trip through Excel turns "00601" into "601". A
// malformed ratio is an error, since dropping the row would leave a
// silent coverage hole.
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

// padLeadingZeros left-pads s with '0' to width n.
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
