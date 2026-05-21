package us

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseHUDZipCounty_GoldenFixture(t *testing.T) {
	// Small inline fixture mirroring HUD's published format. HUD ships
	// the CSV with quoted fields and a header row; ZIPs appear as
	// 5-digit strings (so quoted to preserve leading zeros), COUNTY as
	// the 5-digit FIPS string.
	fixture := `"ZIP","COUNTY","RES_RATIO","BUS_RATIO","OTH_RATIO","TOT_RATIO"
"20811","24031","0.000000","0.999000","0.001000","0.999000"
"00601","72001","0.964360","0.030590","0.005050","0.949690"
"00601","72141","0.035640","0.069410","0.994950","0.050310"
"10001","36061","1.000000","1.000000","1.000000","1.000000"
"99999","99999","0.000000","0.000000","0.000000","0.000000"
`
	got, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseHUDZipCounty: %v", err)
	}
	want := []HUDZipCounty{
		{ZIP: "20811", County: "24031", ResRatio: 0.0, BusRatio: 0.999, OthRatio: 0.001, TotRatio: 0.999},
		{ZIP: "00601", County: "72001", ResRatio: 0.96436, BusRatio: 0.03059, OthRatio: 0.00505, TotRatio: 0.94969},
		{ZIP: "00601", County: "72141", ResRatio: 0.03564, BusRatio: 0.06941, OthRatio: 0.99495, TotRatio: 0.05031},
		{ZIP: "10001", County: "36061", ResRatio: 1.0, BusRatio: 1.0, OthRatio: 1.0, TotRatio: 1.0},
		{ZIP: "99999", County: "99999", ResRatio: 0.0, BusRatio: 0.0, OthRatio: 0.0, TotRatio: 0.0},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateApprox(0, 1e-6)); diff != "" {
		t.Errorf("ParseHUDZipCounty (-want +got):\n%s", diff)
	}
}

func TestParseHUDZipCounty_SkipsBlankLines(t *testing.T) {
	// Trailing newlines, blank middle lines, and rows that are
	// nothing but whitespace must be skipped without error.
	fixture := "\"ZIP\",\"COUNTY\",\"RES_RATIO\",\"BUS_RATIO\",\"OTH_RATIO\",\"TOT_RATIO\"\n" +
		"\"20811\",\"24031\",\"0.000\",\"0.999\",\"0.001\",\"0.999\"\n" +
		"\n" +
		"\"10001\",\"36061\",\"1.000\",\"1.000\",\"1.000\",\"1.000\"\n" +
		"\n"
	got, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseHUDZipCounty: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2 (blank lines should be skipped); got %+v", len(got), got)
	}
}

func TestParseHUDZipCounty_MalformedNumericRejected(t *testing.T) {
	fixture := `"ZIP","COUNTY","RES_RATIO","BUS_RATIO","OTH_RATIO","TOT_RATIO"
"20811","24031","not-a-number","0.999","0.001","0.999"
`
	_, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err == nil {
		t.Fatal("expected error on malformed numeric, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Errorf("error must include line number; got: %v", err)
	}
}

func TestParseHUDZipCounty_LeftPadsLeadingZeros(t *testing.T) {
	// Defends against an operator round-tripping the HUD CSV through
	// Excel, which silently coerces ZIP / COUNTY strings to ints and
	// drops leading zeros ("00601" → "601", "01001" → "1001"). The
	// parser must restore them so downstream lookups don't mis-anchor
	// Puerto Rico (state FIPS "72") and New England (state FIPS "01"
	// through "09") ZIPs.
	fixture := `"ZIP","COUNTY","RES_RATIO","BUS_RATIO","OTH_RATIO","TOT_RATIO"
"601","72001","0.95","0.03","0.02","0.95"
"1001","25013","1.00","1.00","1.00","1.00"
`
	got, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseHUDZipCounty: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ZIP != "00601" || got[0].County != "72001" {
		t.Errorf("row 0 = (%q,%q), want (\"00601\",\"72001\")", got[0].ZIP, got[0].County)
	}
	if got[1].ZIP != "01001" || got[1].County != "25013" {
		t.Errorf("row 1 = (%q,%q), want (\"01001\",\"25013\")", got[1].ZIP, got[1].County)
	}
}

func TestParseHUDZipCounty_RealWorldEightColumnLayout(t *testing.T) {
	// HUD's actual published CSV has 8 columns with USPS_ZIP_PREF_CITY
	// and USPS_ZIP_PREF_STATE wedged between COUNTY and the ratio
	// columns. The parser resolves columns by header name, so this
	// layout (and any future HUD column reordering) must work without
	// the parser silently anchoring ZIPs by the wrong column.
	fixture := `ZIP,COUNTY,USPS_ZIP_PREF_CITY,USPS_ZIP_PREF_STATE,RES_RATIO,BUS_RATIO,OTH_RATIO,TOT_RATIO
00501,36103,HOLTSVILLE,NY,0.000000000,1.000000000,0.000000000,1.000000000
20811,24031,BETHESDA,MD,0.000000000,0.999000000,0.001000000,0.999000000
`
	got, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseHUDZipCounty: %v", err)
	}
	want := []HUDZipCounty{
		{ZIP: "00501", County: "36103", ResRatio: 0.0, BusRatio: 1.0, OthRatio: 0.0, TotRatio: 1.0},
		{ZIP: "20811", County: "24031", ResRatio: 0.0, BusRatio: 0.999, OthRatio: 0.001, TotRatio: 0.999},
	}
	if diff := cmp.Diff(want, got, cmpopts.EquateApprox(0, 1e-6)); diff != "" {
		t.Errorf("ParseHUDZipCounty (-want +got):\n%s", diff)
	}
}

func TestParseHUDZipCounty_MissingRequiredHeader(t *testing.T) {
	// A HUD vintage that drops or renames a required column must fail
	// loudly, not silently produce zero rows.
	fixture := `ZIP,COUNTY,RES_RATIO,BUS_RATIO,OTH_RATIO
"20811","24031","0.000","0.999","0.001"
`
	_, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err == nil {
		t.Fatal("expected error on missing TOT_RATIO header, got nil")
	}
	if !strings.Contains(err.Error(), "TOT_RATIO") {
		t.Errorf("error should name missing column; got: %v", err)
	}
}

func TestParseHUDZipCounty_TrimsTrailingWhitespace(t *testing.T) {
	fixture := "\"ZIP\",\"COUNTY\",\"RES_RATIO\",\"BUS_RATIO\",\"OTH_RATIO\",\"TOT_RATIO\"\n" +
		"\"20811 \",\"24031 \",\"0.000\",\"0.999\",\"0.001\",\"0.999\"\n"
	got, err := ParseHUDZipCounty(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("ParseHUDZipCounty: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].ZIP != "20811" || got[0].County != "24031" {
		t.Errorf("trailing whitespace not trimmed: %+v", got[0])
	}
}
