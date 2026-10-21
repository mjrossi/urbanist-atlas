package us

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// msa is one Metropolitan Statistical Area, rolled up from the CBSA
// delineation file's one-row-per-county layout.
type msa struct {
	// CBSACode is the 5-digit Census CBSA code (e.g. "35620").
	CBSACode string
	// Title is the Census CBSA title ("New York-Newark-Jersey City, NY-NJ").
	Title string
	// StateAbbrevs are the postal abbreviations at the end of Title, in
	// title order.
	StateAbbrevs []string
	// Counties are the MSA's 5-digit county GEOIDs, sorted.
	Counties []string
}

// parseCBSA reads the CBSA delineation CSV (converted from xlsx; see
// etl/SOURCES.md) into MSAs sorted by CBSA code plus a county GEOID →
// CBSA code lookup. Micropolitan rows are dropped. Metropolitan Division
// columns are ignored, so division counties roll up into their MSA.
// Census puts banner rows above the header, so rows before the
// "CBSA Code" header row are skipped.
func parseCBSA(r io.Reader) (msas []msa, countyToMSA map[string]string, err error) {
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
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, nil, fmt.Errorf("parse cbsa: read: %w", readErr)
		}
		if !headerSeen {
			if len(row) > 0 && strings.TrimSpace(row[0]) == "CBSA Code" {
				col = map[string]int{}
				for i, name := range row {
					col[strings.TrimSpace(name)] = i
				}
				required := []string{
					"CBSA Code",
					"CBSA Title",
					"Metropolitan/Micropolitan Statistical Area",
					"FIPS State Code",
					"FIPS County Code",
				}
				for _, name := range required {
					if _, ok := col[name]; !ok {
						return nil, nil, fmt.Errorf("parse cbsa: missing %s column", name)
					}
				}
				headerSeen = true
			}
			continue
		}
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
		// The delineation file zero-pads FIPS already; pad anyway in case a
		// spreadsheet round trip stripped it.
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
		return nil, nil, errors.New("parse cbsa: header row not found")
	}

	countyToMSA = map[string]string{}
	for code, v := range agg {
		for c := range v.Counties {
			countyToMSA[c] = code
		}
	}

	msas = make([]msa, 0, len(agg))
	for code, v := range agg {
		counties := make([]string, 0, len(v.Counties))
		for c := range v.Counties {
			counties = append(counties, c)
		}
		sort.Strings(counties)
		msas = append(msas, msa{
			CBSACode:     code,
			Title:        v.Title,
			StateAbbrevs: parseStateAbbrevs(v.Title),
			Counties:     counties,
		})
	}
	sort.Slice(msas, func(i, j int) bool { return msas[i].CBSACode < msas[j].CBSACode })
	return msas, countyToMSA, nil
}

// parseStateAbbrevs returns the state abbreviations in a CBSA title's
// trailing ", XX[-YY-ZZ]", or nil when the title doesn't end that way.
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
