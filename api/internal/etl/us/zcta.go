package us

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// zctaPlace is a ZCTA's primary place: of the places it straddles, the
// one with the largest AREALAND_PART.
type zctaPlace struct {
	PlaceGEOID string
	PlaceName  string
}

// zctaCounty is a ZCTA's primary county, by the same AREALAND_PART rule.
type zctaCounty struct {
	CountyGEOID string
	CountyName  string
}

// parseZCTAPlace reads the Census ZCTA-to-place relationship file
// (tab20_zcta520_place20_natl.txt) into one primary place per ZCTA. The
// column indexes are fixed by Census's pipe-delimited layout:
//
//	OID_ZCTA5_20|GEOID_ZCTA5_20|NAMELSAD_ZCTA5_20|AREALAND_ZCTA5_20|
//	AREAWATER_ZCTA5_20|MTFCC_ZCTA5_20|CLASSFP_ZCTA5_20|FUNCSTAT_ZCTA5_20|
//	OID_PLACE_20|GEOID_PLACE_20|NAMELSAD_PLACE_20|AREALAND_PLACE_20|
//	AREAWATER_PLACE_20|MTFCC_PLACE_20|CLASSFP_PLACE_20|FUNCSTAT_PLACE_20|
//	AREALAND_PART|AREAWATER_PART
//
// Rows with a blank ZCTA or place GEOID are skipped.
func parseZCTAPlace(r io.Reader) (map[string]zctaPlace, error) {
	raw, err := parseZCTARelationship(r, 1, 9, 10, 16)
	if err != nil {
		return nil, err
	}
	out := make(map[string]zctaPlace, len(raw))
	for zcta, v := range raw {
		out[zcta] = zctaPlace{PlaceGEOID: v.GEOID, PlaceName: v.Name}
	}
	return out, nil
}

// parseZCTACounty reads the Census ZCTA-to-county relationship file,
// which has the place file's layout with COUNTY columns in place of
// PLACE ones.
func parseZCTACounty(r io.Reader) (map[string]zctaCounty, error) {
	raw, err := parseZCTARelationship(r, 1, 9, 10, 16)
	if err != nil {
		return nil, err
	}
	out := make(map[string]zctaCounty, len(raw))
	for zcta, v := range raw {
		out[zcta] = zctaCounty{CountyGEOID: v.GEOID, CountyName: v.Name}
	}
	return out, nil
}

type zctaAttachment struct {
	GEOID        string
	Name         string
	AreaLandPart int64
}

// parseZCTARelationship keeps, per ZCTA, the row with the largest
// AREALAND_PART. The column arguments are indexes into the pipe-split row.
func parseZCTARelationship(r io.Reader, zctaCol, geoidCol, nameCol, areaCol int) (map[string]zctaAttachment, error) {
	out := map[string]zctaAttachment{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	headerSeen := false
	lineNum := 0
	const utf8BOM = "\uFEFF"
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		// Census ships these files with a UTF-8 BOM that bufio doesn't strip.
		if lineNum == 1 {
			line = strings.TrimPrefix(line, utf8BOM)
		}
		if !headerSeen {
			headerSeen = true
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) <= areaCol {
			continue
		}
		zcta := strings.TrimSpace(fields[zctaCol])
		geoid := strings.TrimSpace(fields[geoidCol])
		name := strings.TrimSpace(fields[nameCol])
		areaStr := strings.TrimSpace(fields[areaCol])
		if zcta == "" || geoid == "" {
			// A county no ZCTA falls in, or a ZCTA outside every place.
			continue
		}
		area, err := strconv.ParseInt(areaStr, 10, 64)
		if err != nil {
			// Coercing a malformed AREALAND_PART to 0 would silently lose the
			// max-area pick and mis-anchor the ZCTA.
			return nil, fmt.Errorf("parse zcta relationship: line %d: AREALAND_PART %q: %w", lineNum, areaStr, err)
		}
		existing, ok := out[zcta]
		if !ok || area > existing.AreaLandPart {
			out[zcta] = zctaAttachment{GEOID: geoid, Name: name, AreaLandPart: area}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse zcta relationship: scan: %w", err)
	}
	return out, nil
}
