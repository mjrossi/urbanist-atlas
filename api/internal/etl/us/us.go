// Package us implements the United States ETL plan for the urbanist
// atlas postal-coverage pipeline (slices #7.5.3 + #7.5.5). It reads
// upstream reference files staged under etl/sources/us/ from two
// complementary sources:
//
// Census Bureau (primary — slice #7.5.3):
//
//   - list1_2023.csv             — CBSA delineation (xlsx → CSV via
//     etl/scripts/xlsx_to_csv.py;
//     see etl/SOURCES.md)
//   - tab20_zcta520_place20_natl.txt  — ZCTA-to-place crosswalk
//   - tab20_zcta520_county20_natl.txt — ZCTA-to-county crosswalk
//
// HUD (additive backfill — slice #7.5.5):
//
//   - hud_zip_county_<vintage>.csv — USPS ZIP-to-County crosswalk
//     covering operational ZIPs Census ZCTA omits (P.O. Box-only,
//     single-building, APO/FPO). Optional — the orchestrator
//     gracefully degrades to ZCTA-only when the file is absent.
//
// It produces two deterministic seed files under api/seed/:
//
//   - regions_us_msas.toml       — one [[region]] per Metropolitan
//     Statistical Area
//   - postal_codes_us.csv        — ZIP → smallest-curated-anchor slug,
//     merged from ZCTA + HUD passes
//
// Importing the package (or blank-importing it, as cmd/server/etl.go
// does) registers the US plan with etl.Plans via init().
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
				// CBSA delineation ships as xlsx only; the Filename
				// here is the downloaded xlsx. Regenerate reads the
				// derived CSV (list1_2023.csv) produced by the
				// manual `etl/scripts/xlsx_to_csv.py` step — see
				// etl/SOURCES.md for the recipe.
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
				// ZCTA crosswalks feed only the postal pass (Regenerate
				// parses them after the regions early-return), so a
				// --target=regions download skips them.
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
				// HUD USPS ZIP-to-County crosswalk (slice #7.5.5). The
				// canonical download URL is account-scoped behind the
				// HUDUser portal; the operator pins the sha256 below
				// on first download. URL points at the portal landing
				// page so the operator has a one-click entry point;
				// the actual download is manual. The downloader's
				// sha256 check is skipped when SHA256 is empty, so
				// pre-pinning the file is the operator's only required
				// step before `etl regenerate`.
				Filename: "hud_zip_county_2025q4.csv",
				URL:      "https://www.huduser.gov/portal/dataset/uspszip-api.html",
				SHA256:   "2795b91c26703d1150f2545683da0b6638d006f213e48cc70318e384b3f00f8b",
				Vintage:  "HUD USPS ZIP-to-County crosswalk, 2025-Q4 (operator-downloaded; HUD account required)",
				// Account-gated landing page, not a direct file: download
				// must skip it (operator fetches by hand). Feeds the
				// postal pass only.
				Optional: true,
				Targets:  []etl.Target{etl.TargetPostal},
			},
		},
		Targets: []etl.OutputTarget{
			{Path: "regions_us_msas.toml", Format: "toml", MinRows: 380, MaxRows: 500},
			// Row band widened from 30000-35000 in slice #7.5.5 to
			// accommodate the additive HUD backfill (~5-10k net-new
			// rows; ZCTA-source rows unchanged).
			{Path: "postal_codes_us.csv", Format: "csv", MinRows: 30000, MaxRows: 45000},
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
//  6. (Slice #7.5.5) If a HUD ZIP-County CSV is staged under srcDir,
//     parse it and run crosswalkHUDBackfill against the post-ZCTA
//     anchor set to produce additional anchors for ZIPs Census ZCTA
//     omits (P.O. Box-only, single-building, APO/FPO). The HUD CSV is
//     account-gated so its absence is not an error — the flow
//     degrades to ZCTA-only.
//  7. Write regions_us_msas.toml + postal_codes_us.csv under outDir.
//     The CSV writer merges ZCTA + HUD anchor slices with ZCTA
//     winning any (country, postal_code) tie.
//
// Logs row counts and reason-bucket counts (city-leaf, nyc-borough,
// county-leaf, msa, state) so the maintainer can spot coverage gaps.
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

// regionRouting carries the region-pass outputs the postal pass needs:
// the county→MSA lookup and the two slug routing maps every crosswalk
// call threads into newCountyResolver. The region TOML is written inside
// regenerateRegions; only these lookups flow forward.
type regionRouting struct {
	countyToMSA  map[string]string
	cbsaToSlug   map[string]string // CBSA code → umbrella slug
	portionSlugs map[string]string // "CBSAcode:stateFIPS" → portion slug
}

// regenerateRegions runs the region pass: parse the CBSA delineation,
// read editorial overrides, assign slugs/names/parents, expand to the
// full emitted region set, and (when the target includes regions) write
// regions_us_msas.toml. It always returns the routing lookups the postal
// pass needs, even on a --target=postal run where no TOML is written.
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
	// Expand to the full emitted region set (umbrellas + per-state
	// portions) and the portion anchor lookup the crosswalk routes through.
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

// regeneratePostal runs the postal pass: parse the ZCTA crosswalks, run
// the smallest-anchor crosswalk to produce ZCTA anchors, additively
// backfill HUD anchors for the operational ZIPs Census omits, then write
// postal_codes_us.csv (ZCTA winning any (country, postal_code) tie).
func regeneratePostal(srcDir, outDir string, rt regionRouting, logger *slog.Logger) error {
	// ZCTA crosswalks feed only the postal pass, so they're parsed here
	// rather than above the region write — a --target=regions run needs
	// just CBSA + overrides and skips ~20 MB of ZCTA download/parse.
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

// backfillHUD runs the optional HUD ZIP-County backfill against the
// post-ZCTA anchor set. When no HUD CSV is staged the flow degrades to
// ZCTA-only (nil anchors, empty reasons). When one is present it first
// reconciles the CT legacy-county gap (mutating zctaAnchors in place) and
// then produces additional anchors for ZIPs Census ZCTA omits (P.O.
// Box-only, single-building, APO/FPO).
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

	// Repair the CT county-vintage gap before backfill: re-anchor CT
	// ZCTA ZIPs stranded at the bare state (their 2020 legacy county
	// isn't in the 2023 planning-region countyToMSA) using HUD's
	// current-vintage county. Mutates `zctaAnchors` in place.
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

// findHUDFile scans srcDir for any file matching the HUD ZIP-County
// naming convention "hud_zip_county_*.csv" and returns the path to
// the latest match (sorted ASC by name, so the lexicographically
// last entry — typically the most recent vintage — wins when
// multiple exist). Returns "" when no match is found — the caller
// treats that as "no HUD backfill" and emits a ZCTA-only CSV.
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
