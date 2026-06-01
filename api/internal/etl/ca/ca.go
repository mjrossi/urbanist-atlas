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
//     (curated city leaf → CMA via prefix rule → province).
//
// Importing the package (or blank-importing it as cmd/server/etl.go
// does) registers the CA plan with etl.Plans via init().
//
// FSA → CMA mapping caveat: without StatsCan's restricted-licence
// Postal Code Conversion File (PCCF) we can't do per-FSA spatial join.
// Slice #7.5.4 uses a coarse FSA prefix → CMA hand-mapping in
// mappings.go covering Toronto, Montréal, Vancouver, Ottawa-Gatineau,
// Calgary, and Edmonton; future slices can replace this with a
// per-FSA mapping when PCCF or spatial-join data becomes available.
package ca

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
			{Path: "regions_ca_cmas.toml", Format: "toml", MinRows: 35, MaxRows: 45},
			{Path: "postal_codes_ca.csv", Format: "csv", MinRows: 1500, MaxRows: 1700},
		},
		Regenerate: Regenerate,
	}
}

// Regenerate runs the full CA ETL pipeline.
func Regenerate(ctx context.Context, srcDir, outDir string, target etl.Target, logger *slog.Logger) error {
	cmaZipPath := filepath.Join(srcDir, "lcma000b21a_e.zip")

	cmas, err := ParseCMAs(cmaZipPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: parsed CMA boundary", "cmas", len(cmas), "path", cmaZipPath)

	overridesPath := filepath.Join(outDir, "regions_ca_cma_overrides.toml")
	overrides, err := ReadCMAOverrides(overridesPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: read overrides", "count", len(overrides), "path", overridesPath)

	assignments := assignCMAs(cmas, overrides)
	knownCMASlugs := make(map[string]bool, len(assignments))
	for _, a := range assignments {
		knownCMASlugs[a.Slug] = true
	}

	if target.Regions() {
		tomlPath := filepath.Join(outDir, "regions_ca_cmas.toml")
		if err := writeCMAsToFile(tomlPath, assignments); err != nil {
			return err
		}
		logger.Info("etl ca: wrote CMAs", "path", tomlPath, "count", len(assignments))
	}

	if !target.Postal() {
		return nil
	}

	// The FSA boundary is the 155 MB source; parse it only for the postal
	// pass so a --target=regions run needs just the CMA file.
	fsaZipPath := filepath.Join(srcDir, "lfsa000b21a_e.zip")
	fsas, err := ParseFSAs(fsaZipPath)
	if err != nil {
		return err
	}
	logger.Info("etl ca: parsed FSA boundary", "fsas", len(fsas), "path", fsaZipPath)

	anchors, reasons := Crosswalk(fsas, knownCMASlugs)
	csvPath := filepath.Join(outDir, "postal_codes_ca.csv")
	if err := writeCSVToFile(csvPath, anchors); err != nil {
		return err
	}
	logger.Info("etl ca: wrote postal codes",
		"path", csvPath,
		"count", len(anchors),
		"by_reason", fmt.Sprintf("%+v", reasons),
	)
	return nil
}

func writeCMAsToFile(path string, assignments []CMAAssignment) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("etl ca: create %s: %w", path, err)
	}
	defer f.Close()
	return WriteCMAsTOML(f, assignments)
}

func writeCSVToFile(path string, anchors []PostalAnchor) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("etl ca: create %s: %w", path, err)
	}
	defer f.Close()
	return WritePostalCodesCSV(f, anchors)
}
