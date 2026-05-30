package us

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// Minimal, correctly-formatted public-source fixtures (no HUD) — just
// enough to parse and write the region TOML. The ZCTA files are
// pipe-delimited with an 18-column layout (header skipped); see zcta.go.
const tinyCBSA = `Banner row one
CBSA Code,CBSA Title,Metropolitan/Micropolitan Statistical Area,FIPS State Code,FIPS County Code
35620,"New York-Newark-Jersey City, NY-NJ",Metropolitan Statistical Area,36,061
`

const tinyZCTAPlace = `OID_ZCTA5_20|GEOID_ZCTA5_20|NAMELSAD_ZCTA5_20|AREALAND_ZCTA5_20|AREAWATER_ZCTA5_20|MTFCC_ZCTA5_20|CLASSFP_ZCTA5_20|FUNCSTAT_ZCTA5_20|OID_PLACE_20|GEOID_PLACE_20|NAMELSAD_PLACE_20|AREALAND_PLACE_20|AREAWATER_PLACE_20|MTFCC_PLACE_20|CLASSFP_PLACE_20|FUNCSTAT_PLACE_20|AREALAND_PART|AREAWATER_PART
1|10001|ZCTA5 10001|100|0|G6350|B5|S|2|3651000|New York city|100|0|G4110|C1|A|100|0
`

const tinyZCTACounty = `OID_ZCTA5_20|GEOID_ZCTA5_20|NAMELSAD_ZCTA5_20|AREALAND_ZCTA5_20|AREAWATER_ZCTA5_20|MTFCC_ZCTA5_20|CLASSFP_ZCTA5_20|FUNCSTAT_ZCTA5_20|OID_COUNTY_20|GEOID_COUNTY_20|NAMELSAD_COUNTY_20|AREALAND_COUNTY_20|AREAWATER_COUNTY_20|MTFCC_COUNTY_20|CLASSFP_COUNTY_20|FUNCSTAT_COUNTY_20|AREALAND_PART|AREAWATER_PART
1|10001|ZCTA5 10001|100|0|G6350|B5|S|2|36061|New York County|100|0|G4020|H1|A|100|0
`

func TestRegenerate_TargetRegionsSkipsPostal(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	mustWrite(t, filepath.Join(src, "list1_2023.csv"), tinyCBSA)
	mustWrite(t, filepath.Join(src, "tab20_zcta520_place20_natl.txt"), tinyZCTAPlace)
	mustWrite(t, filepath.Join(src, "tab20_zcta520_county20_natl.txt"), tinyZCTACounty)
	mustWrite(t, filepath.Join(out, "regions_us_msa_overrides.toml"), "")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := Regenerate(context.Background(), src, out, etl.TargetRegions, logger); err != nil {
		t.Fatalf("Regenerate(regions): %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "regions_us_msas.toml")); err != nil {
		t.Errorf("region TOML not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "postal_codes_us.csv")); !os.IsNotExist(err) {
		t.Errorf("postal CSV should NOT be written for TargetRegions (stat err=%v)", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
