// Package us implements the United States ETL plan for the urbanist
// atlas postal-coverage pipeline (slice #7.5.3). It reads three Census
// Bureau reference files staged under etl/sources/us/:
//
//   - list1_2023.csv             — CBSA delineation (xlsx → CSV via
//     etl/scripts/xlsx_to_csv.py;
//     see etl/SOURCES.md)
//   - tab20_zcta520_place20_natl.txt  — ZCTA-to-place crosswalk
//   - tab20_zcta520_county20_natl.txt — ZCTA-to-county crosswalk
//
// and produces two deterministic seed files under api/seed/:
//
//   - regions_us_msas.toml       — one [[region]] per Metropolitan
//     Statistical Area
//   - postal_codes_us.csv        — ZIP → smallest-curated-anchor slug
//
// Importing the package (or blank-importing it, as cmd/server/etl.go
// does) registers the US plan with etl.Plans via init().
package us

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

func init() {
	etl.Plans["US"] = etl.Country{
		Code:       "US",
		SourcesDir: "us",
		Sources: []etl.SourceDescriptor{
			{
				Filename: "list1_2023.csv",
				URL:      "https://www2.census.gov/programs-surveys/metro-micro/geographies/reference-files/2023/delineation-files/list1_2023.xlsx",
				Vintage:  "Census CBSA delineation, July 2023",
			},
			{
				Filename: "tab20_zcta520_place20_natl.txt",
				URL:      "https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_place20_natl.txt",
				Vintage:  "Census ZCTA-to-place relationship, 2020 vintage",
			},
			{
				Filename: "tab20_zcta520_county20_natl.txt",
				URL:      "https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_county20_natl.txt",
				Vintage:  "Census ZCTA-to-county relationship, 2020 vintage",
			},
		},
		Targets: []etl.OutputTarget{
			{Path: "regions_us_msas.toml", Format: "toml", MinRows: 380, MaxRows: 400},
			{Path: "postal_codes_us.csv", Format: "csv", MinRows: 30000, MaxRows: 35000},
		},
		Regenerate: Regenerate,
	}
}

// Regenerate runs the full US ETL pipeline:
//
//  1. Parse the CBSA delineation CSV → list of MSAs, county-to-MSA
//     lookup.
//  2. Parse the ZCTA-to-place + ZCTA-to-county relationship files →
//     per-ZCTA primary place + county.
//  3. Read editorial overrides from api/seed/regions_us_msa_overrides.toml
//     (relative to outDir).
//  4. Assign slugs/names/parents to every MSA (overrides win).
//  5. Run the smallest-anchor crosswalk over every ZCTA → anchor slug.
//  6. Write regions_us_msas.toml + postal_codes_us.csv under outDir.
//
// Logs row counts and reason-bucket counts (city-leaf, nyc-borough,
// county-leaf, msa, state) so the maintainer can spot coverage gaps.
func Regenerate(ctx context.Context, srcDir, outDir string, logger *slog.Logger) error {
	cbsaPath := filepath.Join(srcDir, "list1_2023.csv")
	zctaPlacePath := filepath.Join(srcDir, "tab20_zcta520_place20_natl.txt")
	zctaCountyPath := filepath.Join(srcDir, "tab20_zcta520_county20_natl.txt")

	msas, countyToMSA, err := loadCBSA(cbsaPath)
	if err != nil {
		return err
	}
	logger.Info("etl us: parsed CBSA delineation", "msas", len(msas), "county_to_msa", len(countyToMSA))

	zctaPlace, err := loadZCTAPlace(zctaPlacePath)
	if err != nil {
		return err
	}
	logger.Info("etl us: parsed ZCTA-to-place", "rows", len(zctaPlace))

	zctaCounty, err := loadZCTACounty(zctaCountyPath)
	if err != nil {
		return err
	}
	logger.Info("etl us: parsed ZCTA-to-county", "rows", len(zctaCounty))

	overridesPath := filepath.Join(outDir, "regions_us_msa_overrides.toml")
	overrides, err := ReadMSAOverrides(overridesPath)
	if err != nil {
		return err
	}
	logger.Info("etl us: read overrides", "count", len(overrides), "path", overridesPath)

	assignments := AssignMSASlugs(msas, overrides)
	cbsaToSlug := make(map[string]string, len(assignments))
	for code, a := range assignments {
		cbsaToSlug[code] = a.Slug
	}

	msaTOMLPath := filepath.Join(outDir, "regions_us_msas.toml")
	if err := writeMSAs(msaTOMLPath, msas, assignments); err != nil {
		return err
	}
	logger.Info("etl us: wrote MSAs", "path", msaTOMLPath, "count", len(msas))

	anchors, reasons := Crosswalk(zctaPlace, zctaCounty, countyToMSA, cbsaToSlug)
	csvPath := filepath.Join(outDir, "postal_codes_us.csv")
	if err := writeCSV(csvPath, anchors); err != nil {
		return err
	}
	logger.Info("etl us: wrote postal codes", "path", csvPath, "count", len(anchors), "by_reason", fmt.Sprintf("%+v", reasons))

	return nil
}

func loadCBSA(path string) ([]MSA, map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("etl us: open cbsa %s: %w", path, err)
	}
	defer f.Close()
	return ParseCBSA(f)
}

func loadZCTAPlace(path string) (map[string]ZCTAPlace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("etl us: open zcta-place %s: %w", path, err)
	}
	defer f.Close()
	return ParseZCTAPlace(f)
}

func loadZCTACounty(path string) (map[string]ZCTACounty, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("etl us: open zcta-county %s: %w", path, err)
	}
	defer f.Close()
	return ParseZCTACounty(f)
}

func writeMSAs(path string, msas []MSA, assignments map[string]MSAOverride) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("etl us: create %s: %w", path, err)
	}
	defer f.Close()
	return WriteMSAsTOML(f, msas, assignments)
}

func writeCSV(path string, anchors []PostalAnchor) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("etl us: create %s: %w", path, err)
	}
	defer f.Close()
	return WritePostalCodesCSV(f, anchors)
}
