package us

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strings"
)

// MSA is a single auto-generated Metropolitan Statistical Area row,
// derived from one or more lines in the Census CBSA delineation file
// (one per constituent county). All counties belonging to the same
// CBSA code roll up into one MSA value.
type MSA struct {
	// CBSACode is the 5-digit Census CBSA code (e.g., "35620" for
	// New York-Newark-Jersey City).
	CBSACode string
	// Title is the canonical Census CBSA title (e.g.,
	// "New York-Newark-Jersey City, NY-NJ").
	Title string
	// StateAbbrevs holds the 2-letter postal abbreviations parsed
	// from the trailing portion of Title (e.g., ["NY","NJ"] for
	// 35620). Order preserved from the title.
	StateAbbrevs []string
	// Counties is the list of (5-digit FIPS state+county) GEOIDs
	// that belong to this MSA per the delineation file. Sorted
	// ascending for deterministic output.
	Counties []string
}

// ParseCBSA reads the Census CBSA delineation file (already converted
// from xlsx to CSV — see etl/scripts/xlsx_to_csv.py and
// etl/SOURCES.md) and returns:
//
//   - msas: one MSA per unique CBSA code, with the union of its
//     constituent counties, sorted by CBSA code ascending.
//   - countyToMSA: a lookup table from 5-digit county GEOID → CBSA
//     code. Used by the smallest-anchor crosswalk to find the MSA
//     containing a given ZCTA's primary county.
//
// Only "Metropolitan Statistical Area" rows are kept; micropolitan
// statistical areas (μSAs) and Metropolitan Division sub-rows are
// filtered out. The first two banner rows of the Census CSV are
// skipped automatically by detecting the column-header row.
func ParseCBSA(r io.Reader) (msas []MSA, countyToMSA map[string]string, err error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // banner rows are short

	headerSeen := false
	var col map[string]int

	type aggKey = string // CBSA code
	type aggValue struct {
		Title    string
		Counties map[string]bool
	}
	agg := map[aggKey]*aggValue{}

	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, nil, fmt.Errorf("parse cbsa: read: %w", readErr)
		}
		if !headerSeen {
			// Look for the column-header row, which starts with
			// "CBSA Code". Earlier rows are banners.
			if len(row) > 0 && strings.TrimSpace(row[0]) == "CBSA Code" {
				col = map[string]int{}
				for i, name := range row {
					col[strings.TrimSpace(name)] = i
				}
				if _, ok := col["CBSA Code"]; !ok {
					return nil, nil, fmt.Errorf("parse cbsa: missing CBSA Code column")
				}
				if _, ok := col["CBSA Title"]; !ok {
					return nil, nil, fmt.Errorf("parse cbsa: missing CBSA Title column")
				}
				if _, ok := col["Metropolitan/Micropolitan Statistical Area"]; !ok {
					return nil, nil, fmt.Errorf("parse cbsa: missing Metropolitan/Micropolitan Statistical Area column")
				}
				if _, ok := col["FIPS State Code"]; !ok {
					return nil, nil, fmt.Errorf("parse cbsa: missing FIPS State Code column")
				}
				if _, ok := col["FIPS County Code"]; !ok {
					return nil, nil, fmt.Errorf("parse cbsa: missing FIPS County Code column")
				}
				headerSeen = true
			}
			continue
		}
		// Data rows have ≥12 columns; banner rows already filtered.
		if len(row) <= col["FIPS County Code"] {
			continue
		}
		areaType := strings.TrimSpace(row[col["Metropolitan/Micropolitan Statistical Area"]])
		if areaType != "Metropolitan Statistical Area" {
			continue
		}
		code := strings.TrimSpace(row[col["CBSA Code"]])
		title := strings.TrimSpace(row[col["CBSA Title"]])
		stateFIPS := strings.TrimSpace(row[col["FIPS State Code"]])
		countyFIPS := strings.TrimSpace(row[col["FIPS County Code"]])
		if code == "" || title == "" || stateFIPS == "" || countyFIPS == "" {
			continue
		}
		// FIPS codes in the delineation file are zero-padded already
		// (e.g., "06" "037") but we normalise just in case.
		stateFIPS = fmt.Sprintf("%0*s", 2, stateFIPS)
		countyFIPS = fmt.Sprintf("%0*s", 3, countyFIPS)
		geoID := stateFIPS + countyFIPS

		v, ok := agg[code]
		if !ok {
			v = &aggValue{Title: title, Counties: map[string]bool{}}
			agg[code] = v
		}
		v.Counties[geoID] = true
	}
	if !headerSeen {
		return nil, nil, fmt.Errorf("parse cbsa: header row not found")
	}

	countyToMSA = map[string]string{}
	for code, v := range agg {
		for c := range v.Counties {
			countyToMSA[c] = code
		}
	}

	msas = make([]MSA, 0, len(agg))
	for code, v := range agg {
		counties := make([]string, 0, len(v.Counties))
		for c := range v.Counties {
			counties = append(counties, c)
		}
		sort.Strings(counties)
		msas = append(msas, MSA{
			CBSACode:     code,
			Title:        v.Title,
			StateAbbrevs: parseStateAbbrevs(v.Title),
			Counties:     counties,
		})
	}
	sort.Slice(msas, func(i, j int) bool { return msas[i].CBSACode < msas[j].CBSACode })
	return msas, countyToMSA, nil
}

// parseStateAbbrevs extracts the trailing ", XX[-YY-ZZ]" portion of a
// CBSA title and returns the state abbreviations in order. Returns nil
// for unexpected titles (defensive — every Census MSA title we've seen
// follows the format).
func parseStateAbbrevs(title string) []string {
	i := strings.LastIndex(title, ", ")
	if i < 0 {
		return nil
	}
	suffix := title[i+2:]
	parts := strings.Split(suffix, "-")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if len(p) == 2 && allAlphaUpper(p) {
			out = append(out, p)
		}
	}
	return out
}

func allAlphaUpper(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
