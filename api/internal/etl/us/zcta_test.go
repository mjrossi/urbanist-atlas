package us

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// zctaRow builds one pipe-delimited relationship row in the 18-column
// shape parseZCTAPlace/parseZCTACounty read, populating only the columns
// the shared scanner looks at: ZCTA (1), target GEOID (9), target name
// (10), and AREALAND_PART (16).
func zctaRow(zcta, geoid, name, area string) string {
	f := make([]string, 18)
	f[1] = zcta
	f[9] = geoid
	f[10] = name
	f[16] = area
	return strings.Join(f, "|")
}

// TestParseZCTARelationship_MaxAreaTiebreak pins the happy path: across
// rows that straddle multiple targets, the scanner keeps the one with
// the largest AREALAND_PART, and an explicit "0" area still parses (it's
// the legitimate zero-intersection literal — #17 must reject only
// *malformed* areas, never the benign zero).
func TestParseZCTARelationship_MaxAreaTiebreak(t *testing.T) {
	input := strings.Join([]string{
		zctaRow("HEADER", "x", "x", "x"), // header line, skipped
		zctaRow("00601", "1600100", "Place Small", "100"),
		zctaRow("00601", "1600500", "Place Big", "500"), // larger area wins
		zctaRow("00602", "1600200", "Zero Place", "0"),  // benign zero, must be kept
		zctaRow("", "1600300", "No ZCTA", "999"),        // blank zcta -> skipped
		zctaRow("00603", "", "No GEOID", "999"),         // blank geoid -> skipped
	}, "\n")

	got, err := parseZCTARelationship(strings.NewReader(input), 1, 9, 10, 16)
	if err != nil {
		t.Fatalf("parseZCTARelationship: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (00601, 00602); blanks skipped: %v", len(got), got)
	}
	if a := got["00601"]; a.GEOID != "1600500" || a.AreaLandPart != 500 {
		t.Errorf("00601 = %+v, want GEOID=1600500 AreaLandPart=500 (max-area tiebreak)", a)
	}
	if a := got["00602"]; a.GEOID != "1600200" || a.AreaLandPart != 0 {
		t.Errorf("00602 = %+v, want GEOID=1600200 AreaLandPart=0 (benign zero kept)", a)
	}
}

// TestParseZCTARelationship_MalformedAreaErrors pins issue #17: a
// non-numeric AREALAND_PART is surfaced as an error with line context
// rather than silently coerced to 0 (which would lose the max-area
// tiebreak and mis-anchor the ZCTA with no signal). The error must wrap
// strconv's underlying error so callers can inspect it.
func TestParseZCTARelationship_MalformedAreaErrors(t *testing.T) {
	input := strings.Join([]string{
		zctaRow("HEADER", "x", "x", "x"),                         // line 1: header
		zctaRow("00601", "1600100", "Good Place", "100"),         // line 2: fine
		zctaRow("00602", "1600200", "Bad Place", "not-a-number"), // line 3: malformed
	}, "\n")

	_, err := parseZCTARelationship(strings.NewReader(input), 1, 9, 10, 16)
	if err == nil {
		t.Fatal("expected an error for a malformed AREALAND_PART, got nil")
	}
	if !errors.Is(err, strconv.ErrSyntax) {
		t.Errorf("error should wrap strconv.ErrSyntax, got %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "line 3") || !strings.Contains(msg, "not-a-number") {
		t.Errorf("error should carry line number and offending value, got %q", msg)
	}
}
