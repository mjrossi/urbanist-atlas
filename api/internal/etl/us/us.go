// Package us implements the United States ETL plan. It reads Census
// sources staged under etl/sources/us/ (the CBSA delineation, converted
// from xlsx to list1_2023.csv by etl/scripts/xlsx_to_csv.py, and the
// 2020 ZCTA-to-place and ZCTA-to-county crosswalks) plus, when staged,
// HUD's USPS ZIP-County crosswalk, which covers operational ZIPs Census
// ZCTA omits. It writes regions_us_msas.toml and postal_codes_us.csv.
//
// Importing the package registers the US plan in etl.Plans. See
// docs/etl-architecture.md "US pipeline".
package us

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

func init() {
	etl.Plans["US"] = etl.Country{
		Code:       "US",
		SourcesDir: "us",
		Sources: []etl.SourceDescriptor{
			{
				// Census ships the CBSA delineation as xlsx only; regenerate reads
				// the list1_2023.csv derived from it (recipe in etl/SOURCES.md).
				Filename: "list1_2023.xlsx",
				URL:      "https://www2.census.gov/programs-surveys/metro-micro/geographies/reference-files/2023/delineation-files/list1_2023.xlsx",
				SHA256:   "952c4b1e78acbb54e6ec9412434b7602fedacbf021736351a63c181bdb753629",
				Vintage:  "Census CBSA delineation, July 2023",
			},
			{
				Filename: "tab20_zcta520_place20_natl.txt",
				URL:      "https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_place20_natl.txt",
				SHA256:   "698a5dad71ed419411677d0ffd8ecd9331067f59c472cdd239b92c12f698285d",
				Vintage:  "Census ZCTA-to-place relationship, 2020 vintage",
				// Parsed only by the postal pass.
				Targets: []etl.Target{etl.TargetPostal},
			},
			{
				Filename: "tab20_zcta520_county20_natl.txt",
				URL:      "https://www2.census.gov/geo/docs/maps-data/data/rel2020/zcta520/tab20_zcta520_county20_natl.txt",
				SHA256:   "3ed41278d637dc249e0323306f68be8a6c234e3090f4de88ef328dee71aeaaaf",
				Vintage:  "Census ZCTA-to-county relationship, 2020 vintage",
				Targets:  []etl.Target{etl.TargetPostal},
			},
			{
				// HUD's file sits behind a HUDUser account, so URL is the portal
				// landing page and the operator downloads by hand.
				Filename: "hud_zip_county_2025q4.csv",
				URL:      "https://www.huduser.gov/portal/dataset/uspszip-api.html",
				SHA256:   "2795b91c26703d1150f2545683da0b6638d006f213e48cc70318e384b3f00f8b",
				Vintage:  "HUD USPS ZIP-to-County crosswalk, 2025-Q4 (operator-downloaded; HUD account required)",
				Optional: true,
				Targets:  []etl.Target{etl.TargetPostal},
			},
		},
		Targets: []etl.OutputTarget{
			{Path: "regions_us_msas.toml"},
			{Path: "postal_codes_us.csv"},
		},
		Regenerate: Regenerate,
	}
}

// Regenerate runs the US region pass and, when target includes postal,
// the postal pass.
func Regenerate(ctx context.Context, srcDir, outDir string, target etl.Target, logger *slog.Logger) error {
	rt, err := regenerateRegions(srcDir, outDir, target, logger)
	if err != nil {
		return err
	}

	if !target.Postal() {
		return nil
	}

	return regeneratePostal(srcDir, outDir, rt, logger)
}

// regionRouting carries the region-pass lookups the postal pass threads
// into newCountyResolver.
type regionRouting struct {
	countyToMSA  map[string]string
	cbsaToSlug   map[string]string // CBSA code → umbrella slug
	portionSlugs map[string]string // "CBSAcode:stateFIPS" → portion slug
}

// regenerateRegions parses the CBSA delineation, applies overrides and
// writes regions_us_msas.toml when target includes regions. It returns
// the routing lookups even when it writes nothing.
func regenerateRegions(srcDir, outDir string, target etl.Target, logger *slog.Logger) (regionRouting, error) {
	cbsaPath := filepath.Join(srcDir, "list1_2023.csv")

	msas, countyToMSA, err := loadCBSA(cbsaPath)
	if err != nil {
		return regionRouting{}, err
	}
	logger.Info("etl us: parsed CBSA delineation", "msas", len(msas), "county_to_msa", len(countyToMSA))

	overridesPath := filepath.Join(outDir, "regions_us_msa_overrides.toml")
	overrides, err := etl.ReadOverrides[MSAOverride](overridesPath)
	if err != nil {
		return regionRouting{}, err
	}
	logger.Info("etl us: read overrides", "count", len(overrides), "path", overridesPath)

	assignments := assignMSASlugs(msas, overrides)
	cbsaToSlug := make(map[string]string, len(assignments))
	for code, a := range assignments {
		cbsaToSlug[code] = a.Slug
	}
	rows, portionSlugs := buildRegionRows(msas, assignments)

	if target.Regions() {
		msaTOMLPath := filepath.Join(outDir, "regions_us_msas.toml")
		writeTOML := func(w io.Writer) error { return etl.WriteRegionsTOML(w, msaTOMLHeader, rows) }
		if err := etl.WriteFile(msaTOMLPath, "etl us", writeTOML); err != nil {
			return regionRouting{}, err
		}
		logger.Info("etl us: wrote MSAs", "path", msaTOMLPath, "regions", len(rows), "portions", len(portionSlugs))
	}

	return regionRouting{
		countyToMSA:  countyToMSA,
		cbsaToSlug:   cbsaToSlug,
		portionSlugs: portionSlugs,
	}, nil
}

// regeneratePostal anchors every ZCTA, backfills HUD, and writes
// postal_codes_us.csv.
func regeneratePostal(srcDir, outDir string, rt regionRouting, logger *slog.Logger) error {
	zctaPlaces, err := loadZCTAPlace(filepath.Join(srcDir, "tab20_zcta520_place20_natl.txt"))
	if err != nil {
		return err
	}
	logger.Info("etl us: parsed ZCTA-to-place", "rows", len(zctaPlaces))

	zctaCounties, err := loadZCTACounty(filepath.Join(srcDir, "tab20_zcta520_county20_natl.txt"))
	if err != nil {
		return err
	}
	logger.Info("etl us: parsed ZCTA-to-county", "rows", len(zctaCounties))

	anchors, reasons := crosswalk(zctaPlaces, zctaCounties, rt.countyToMSA, rt.cbsaToSlug, rt.portionSlugs)

	hudAnchors, hudReasons, err := backfillHUD(srcDir, anchors, zctaCounties, rt, logger)
	if err != nil {
		return err
	}

	csvPath := filepath.Join(outDir, "postal_codes_us.csv")
	writePostal := func(w io.Writer) error { return writePostalCodesCSV(w, anchors, hudAnchors) }
	if err := etl.WriteFile(csvPath, "etl us", writePostal); err != nil {
		return err
	}
	logger.Info("etl us: wrote postal codes",
		"path", csvPath,
		"zcta_count", len(anchors),
		"hud_count", len(hudAnchors),
		"total", len(anchors)+len(hudAnchors),
		"by_reason", reasons,
		"by_hud_reason", hudReasons,
	)

	return nil
}

// backfillHUD runs the optional HUD pass. With no HUD CSV staged it
// returns no anchors; otherwise it first repairs CT legacy-county anchors
// in zctaAnchors, then anchors the ZIPs ZCTA lacks.
func backfillHUD(srcDir string, zctaAnchors []PostalAnchor, zctaCounties map[string]zctaCounty, rt regionRouting, logger *slog.Logger) ([]PostalAnchor, map[string]int, error) {
	hudPath := findHUDFile(srcDir)
	if hudPath == "" {
		logger.Info("etl us: no HUD ZIP-County CSV found in src dir — skipping non-ZCTA backfill",
			"src_dir", srcDir,
			"hint", "place hud_zip_county_<vintage>.csv under etl/sources/us/ to enable",
		)
		return nil, map[string]int{}, nil
	}

	huds, err := loadHUD(hudPath)
	if err != nil {
		return nil, nil, err
	}
	logger.Info("etl us: parsed HUD ZIP-County", "rows", len(huds), "path", hudPath)

	ctReasons := reconcileCTLegacyCounties(zctaAnchors, zctaCounties, huds, rt.countyToMSA, rt.cbsaToSlug, rt.portionSlugs)
	ctReconciled := 0
	for k, n := range ctReasons {
		if strings.HasPrefix(k, "ct-reconciled:") {
			ctReconciled += n
		}
	}
	logger.Info("etl us: ct legacy-county reconcile",
		"reasons", ctReasons,
		"reconciled_total", ctReconciled,
		"reconciled_msa", ctReasons["ct-reconciled:msa"],
		"skip_no_hud", ctReasons["ct-skip:no-hud"],
		"skip_hud_unresolved", ctReasons["ct-skip:hud-unresolved"],
	)

	hudAnchors, hudReasons := crosswalkHUDBackfill(huds, zctaAnchors, rt.countyToMSA, rt.cbsaToSlug, rt.portionSlugs)
	logger.Info("etl us: hud backfill",
		"reasons", hudReasons,
		"added", len(hudAnchors),
		"borough_count", hudReasons["hud:nyc-borough"],
		"county_leaf_count", hudReasons["hud:county-leaf"],
		"msa_count", hudReasons["hud:msa"],
		"state_count", hudReasons["hud:state"],
		"unknown_count", hudReasons["hud:unknown"],
	)

	return hudAnchors, hudReasons, nil
}

// findHUDFile returns the lexicographically last
// hud_zip_county_*.csv in srcDir (the newest vintage, given the naming),
// or "" when there is none.
func findHUDFile(srcDir string) string {
	matches, err := filepath.Glob(filepath.Join(srcDir, "hud_zip_county_*.csv"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return matches[len(matches)-1]
}

func loadHUD(path string) ([]hudZipCounty, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("etl us: open hud %s: %w", path, err)
	}
	defer f.Close()
	return parseHUDZipCounty(f)
}

func loadCBSA(path string) ([]msa, map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("etl us: open cbsa %s: %w", path, err)
	}
	defer f.Close()
	return parseCBSA(f)
}

func loadZCTAPlace(path string) (map[string]zctaPlace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("etl us: open zcta-place %s: %w", path, err)
	}
	defer f.Close()
	return parseZCTAPlace(f)
}

func loadZCTACounty(path string) (map[string]zctaCounty, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("etl us: open zcta-county %s: %w", path, err)
	}
	defer f.Close()
	return parseZCTACounty(f)
}
