package loadpostal

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestParseCSV_HappyPath(t *testing.T) {
	src := `postal_code,country,leaf_region_slug
11217,US,brooklyn
11215,US,brooklyn
10001,US,manhattan
`
	rows, err := ParseCSV(strings.NewReader(src), atlas.CountryUS)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	want := []Row{
		{PostalCode: "11217", Country: atlas.CountryUS, LeafRegionSlug: "brooklyn"},
		{PostalCode: "11215", Country: atlas.CountryUS, LeafRegionSlug: "brooklyn"},
		{PostalCode: "10001", Country: atlas.CountryUS, LeafRegionSlug: "manhattan"},
	}
	if diff := cmp.Diff(want, rows); diff != "" {
		t.Errorf("rows mismatch (-want +got):\n%s", diff)
	}
}

func TestParseCSV_NormalizesCanadianFSA(t *testing.T) {
	src := `postal_code,country,leaf_region_slug
M5V 3A8,CA,vancouver
m5v,CA,vancouver
`
	rows, err := ParseCSV(strings.NewReader(src), atlas.CountryCA)
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if rows[0].PostalCode != "M5V" || rows[1].PostalCode != "M5V" {
		t.Errorf("CA normalization failed: %+v", rows)
	}
}

func TestParseCSV_RejectsCrossCountryRow(t *testing.T) {
	src := `postal_code,country,leaf_region_slug
11217,US,brooklyn
V6B,CA,vancouver
`
	_, err := ParseCSV(strings.NewReader(src), atlas.CountryUS)
	if err == nil {
		t.Fatal("expected cross-country error")
	}
}

func TestParseCSV_RejectsBadHeader(t *testing.T) {
	src := `postal,country,slug
11217,US,brooklyn
`
	_, err := ParseCSV(strings.NewReader(src), atlas.CountryUS)
	if err == nil {
		t.Fatal("expected header error")
	}
}

func TestParseCSV_RejectsInvalidPostalCode(t *testing.T) {
	src := `postal_code,country,leaf_region_slug
1121,US,brooklyn
`
	_, err := ParseCSV(strings.NewReader(src), atlas.CountryUS)
	if err == nil {
		t.Fatal("expected postal validation error")
	}
}

func TestParseCSV_RejectsEmptySlug(t *testing.T) {
	src := `postal_code,country,leaf_region_slug
11217,US,
`
	_, err := ParseCSV(strings.NewReader(src), atlas.CountryUS)
	if err == nil {
		t.Fatal("expected empty-slug error")
	}
}
