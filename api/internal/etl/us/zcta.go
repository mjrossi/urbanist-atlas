package us

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ZCTAPlace is the primary place assignment for one ZCTA. A ZCTA may
// straddle multiple Census places; we keep only the place with the
// largest land area within the ZCTA (the AREALAND_PART column).
type ZCTAPlace struct {
	PlaceGEOID string
	PlaceName  string
}

// ZCTACounty is the primary county assignment for one ZCTA. ZCTAs can
// straddle counties too, but it's rare for the primary county to be
// ambiguous — same largest-AREALAND_PART tiebreak applies.
type ZCTACounty struct {
	CountyGEOID string
	CountyName  string
}

// ParseZCTAPlace reads the Census ZCTA-to-place relationship file
// (tab20_zcta520_place20_natl.txt, pipe-delimited with a BOM) and
// returns one entry per ZCTA: the primary place assignment.
//
// File format (pipe-delimited):
//
//	OID_ZCTA5_20|GEOID_ZCTA5_20|NAMELSAD_ZCTA5_20|AREALAND_ZCTA5_20|
//	AREAWATER_ZCTA5_20|MTFCC_ZCTA5_20|CLASSFP_ZCTA5_20|FUNCSTAT_ZCTA5_20|
//	OID_PLACE_20|GEOID_PLACE_20|NAMELSAD_PLACE_20|AREALAND_PLACE_20|
//	AREAWATER_PLACE_20|MTFCC_PLACE_20|CLASSFP_PLACE_20|FUNCSTAT_PLACE_20|
//	AREALAND_PART|AREAWATER_PART
//
// Rows where GEOID_ZCTA5_20 or GEOID_PLACE_20 is blank are skipped (no
// ZCTA-place mapping to record). ZCTAs that straddle multiple places
// keep only the row with the largest AREALAND_PART.
func ParseZCTAPlace(r io.Reader) (map[string]ZCTAPlace, error) {
	raw, err := parseZCTARelationship(r, 1, 9, 10, 16)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ZCTAPlace, len(raw))
	for zcta, v := range raw {
		out[zcta] = ZCTAPlace{PlaceGEOID: v.GEOID, PlaceName: v.Name}
	}
	return out, nil
}

// ParseZCTACounty reads the Census ZCTA-to-county relationship file
// (tab20_zcta520_county20_natl.txt). Same pipe-delimited shape as the
// place file, with COUNTY columns substituted for PLACE columns.
// Returns one entry per ZCTA: the primary county assignment.
func ParseZCTACounty(r io.Reader) (map[string]ZCTACounty, error) {
	raw, err := parseZCTARelationship(r, 1, 9, 10, 16)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ZCTACounty, len(raw))
	for zcta, v := range raw {
		out[zcta] = ZCTACounty{CountyGEOID: v.GEOID, CountyName: v.Name}
	}
	return out, nil
}

type zctaAttachment struct {
	GEOID        string
	Name         string
	AreaLandPart int64
}

// parseZCTARelationship is the shared scanner for both place and
// county crosswalks. zctaCol / geoidCol / nameCol / areaCol are the
// column indexes inside the pipe-delimited row.
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
		// Strip UTF-8 BOM on the first line. Census ships these files
		// with a leading EF BB BF marker that bufio doesn't peel off.
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
			// Rows where the ZCTA or the target geometry is null
			// (e.g., a county that no ZCTA falls inside, or a ZCTA
			// entirely outside any place).
			continue
		}
		area, err := strconv.ParseInt(areaStr, 10, 64)
		if err != nil {
			// A malformed AREALAND_PART can't be silently coerced to 0:
			// 0 would lose the max-area tiebreak and mis-anchor the ZCTA
			// with no signal. Error with line context, mirroring the HUD
			// reader's bad-ratio handling (hud.go).
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
