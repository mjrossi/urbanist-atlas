// Package ca implements the Canada ETL plan for the urbanist atlas
// postal-coverage pipeline (slice #7.5.4). It reads two Statistics
// Canada boundary file zips staged under etl/sources/ca/:
//
//   - lfsa000b21a_e.zip — FSA boundaries (2021 vintage). The DBF
//     attribute table inside maps each 3-character Forward Sortation
//     Area to its province code.
//   - lcma000b21a_e.zip — CMA boundaries (2021 vintage). The DBF
//     attribute table inside lists every Census Metropolitan Area
//     (population ≥100k, type 'B') with its UID, name, and
//     constituent province(s).
//
// and produces two deterministic seed files under api/seed/:
//
//   - regions_ca_cmas.toml — one [[region]] per CMA, slug from the
//     editorial override map (or auto-generated as
//     "<slugified-name>-cma") and parents derived from the CMA's
//     constituent provinces.
//   - postal_codes_ca.csv — FSA → smallest-curated-anchor slug
//     (curated city leaf → CMA via spatial join → province).
//
// Importing the package (or blank-importing it as cmd/server/etl.go
// does) registers the CA plan with etl.Plans via init().
//
// FSA → CMA assignment: each FSA is anchored to the CMA it overlaps
// most by area, computed by a max-overlap spatial join of the two
// boundary files' polygon geometry (spatial.go). This replaced an
// earlier coarse FSA-prefix → CMA table that covered only the seven
// biggest metros; the spatial join resolves all ~41 CMAs without the
// restricted-license Postal Code Conversion File (PCCF).
package ca

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

func init() {
	etl.Plans["CA"] = etl.Country{
		Code:       "CA",
		SourcesDir: "ca",
		Sources: []etl.SourceDescriptor{
			{
				Filename: "lfsa000b21a_e.zip",
				URL:      "https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lfsa000b21a_e.zip",
				SHA256:   "9fd2b6adf66e5716d06f91ebdcdb5d8a4e8b9eeb520f8b4285030d34319959db",
				Vintage:  "Statistics Canada FSA boundary file, 2021 census",
				// The 155 MB FSA source feeds only the postal pass, so a
				// --target=regions download/regenerate skips it.
				Targets: []etl.Target{etl.TargetPostal},
			},
			{
				Filename: "lcma000b21a_e.zip",
				URL:      "https://www12.statcan.gc.ca/census-recensement/2021/geo/sip-pis/boundary-limites/files-fichiers/lcma000b21a_e.zip",
				SHA256:   "a12dd39b3262edb48f9490b435d2f43b0327cc4af7d829f32aebae4d4b9f8fa0",
				Vintage:  "Statistics Canada CMA boundary file, 2021 census",
			},
		},
		Targets: []etl.OutputTarget{
			{Path: "regions_ca_cmas.toml", Format: "toml", MinRows: 35, MaxRows: 50},
			{Path: "postal_codes_ca.csv", Format: "csv", MinRows: 1500, MaxRows: 1700},
		},
		Regenerate: Regenerate,
	}
}

// Regenerate runs the full CA ETL pipeline.
func Regenerate(ctx context.Context, srcDir, outDir string, target etl.Target, logger *slog.Logger) error {
	cmaZipPath := filepath.Join(srcDir, "lcma000b21a_e.zip")

	cmas, err := parseCMAs(cmaZipPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: parsed CMA boundary", "cmas", len(cmas), "path", cmaZipPath)

	overridesPath := filepath.Join(outDir, "regions_ca_cma_overrides.toml")
	overrides, err := etl.ReadOverrides[CMAOverride](overridesPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: read overrides", "count", len(overrides), "path", overridesPath)

	assignments := assignCMAs(cmas, overrides)
	knownCMASlugs := make(map[string]bool, len(assignments))
	slugByCMAUID := make(map[string]string, len(assignments))
	for _, a := range assignments {
		knownCMASlugs[a.Slug] = true
		slugByCMAUID[a.UID] = a.Slug
	}
	// Expand multi-province CMAs (Ottawa-Gatineau) into per-province
	// portions + the portion anchor lookup the FSA crosswalk routes through.
	portions, portionByCMA := buildCMAPortions(cmas, assignments)
	allRegions := make([]etl.RegionRow, 0, len(assignments)+len(portions))
	allRegions = append(allRegions, cmaRowsToRegionRows(assignments)...)
	allRegions = append(allRegions, portions...)

	if target.Regions() {
		tomlPath := filepath.Join(outDir, "regions_ca_cmas.toml")
		writeTOML := func(w io.Writer) error {
			return etl.WriteRegionsTOML(w, cmaTOMLHeader, allRegions)
		}
		if err := etl.WriteFile(tomlPath, "etl ca", writeTOML); err != nil {
			return err
		}
		logger.Info("etl ca: wrote CMAs", "path", tomlPath, "regions", len(allRegions), "portions", len(portions))
	}

	if !target.Postal() {
		return nil
	}

	// The FSA boundary is the 155 MB source; parse it only for the postal
	// pass so a --target=regions run needs just the CMA file.
	fsaZipPath := filepath.Join(srcDir, "lfsa000b21a_e.zip")
	fsas, err := parseFSAs(fsaZipPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: parsed FSA boundary", "fsas", len(fsas), "path", fsaZipPath)

	// Max-overlap spatial join of FSA polygons against CMA polygons
	// (reads the .shp geometry the DBF parse above ignores). Resolve the
	// returned CMA UIDs to region slugs, keeping only CMAs we actually
	// generated so an unmapped UID falls through to province.
	cmaUIDByFSA, err := spatialJoinFSAToCMA(fsaZipPath, cmaZipPath)
	if err != nil {
		return err
	}
	cmaSlugByFSA := make(map[string]string, len(cmaUIDByFSA))
	for fsa, uid := range cmaUIDByFSA {
		if slug := slugByCMAUID[uid]; slug != "" && knownCMASlugs[slug] {
			cmaSlugByFSA[fsa] = slug
		}
	}
	logger.Info("etl ca: spatial join FSA→CMA", "assigned", len(cmaSlugByFSA), "of_fsas", len(fsas))

	anchors, reasons := crosswalk(fsas, cmaSlugByFSA, portionByCMA)
	csvPath := filepath.Join(outDir, "postal_codes_ca.csv")
	writePostal := func(w io.Writer) error { return etl.WritePostalCSV(w, "CA", anchors) }
	if err := etl.WriteFile(csvPath, "etl ca", writePostal); err != nil {
		return err
	}
	logger.Info("etl ca: wrote postal codes",
		"path", csvPath,
		"count", len(anchors),
		"by_reason", reasons,
	)
	return nil
}
