package ca

import (
	"bytes"
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	shp "github.com/jonas-p/go-shp"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

var update = flag.Bool("update", false, "regenerate golden files")

// TestRegenerate_CAGoldenDeterminism runs the full CA Regenerate
// pipeline over synthetic CMA + FSA boundary shapefiles (polygon
// geometry + attribute tables, zipped the way StatsCan ships them) and
// asserts byte-for-byte output against committed goldens. This locks the
// CA generator logic — the part the real-data seed-check gate doesn't
// cover (it gates only the region TOML, and the 155 MB FSA source is
// never fetched in CI).
//
// The fixtures exercise the CMA region paths and every FSA anchor path
// (the CMA each FSA box sits inside drives the max-overlap spatial join):
//
//   - CMA 535 → toronto-cma         (override slug + name)
//   - CMA 933 → metro-vancouver     (override slug + kind)
//   - CMA 421 → auto-slug           (no override; region writer path)
//   - CMA 505 → ottawa-gatineau-cma (stateless multi-province umbrella +
//     rollup_states + on/qc portions)
//   - FSA K1A → ottawa-gatineau-cma-on (cma-portion, Ontario side)
//   - FSA J8X → ottawa-gatineau-cma-qc (cma-portion, Quebec side)
//   - FSA M5V → toronto             (city-leaf, outranks its CMA)
//   - FSA M1B → toronto-cma         (cma, via spatial join)
//   - FSA V5Z → metro-vancouver     (cma, via spatial join)
//   - FSA V6X → richmond            (city-leaf, outranks its CMA)
//   - FSA X0A → nu                  (province fallback, no CMA overlap)
func TestRegenerate_CAGoldenDeterminism(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	goldenDir := "testdata/golden/expected"

	// Each CMA is a well-separated 100×100 box; Ottawa-Gatineau is two
	// boxes (one per province row) sharing UID 505. Each FSA is a small
	// box placed inside its target CMA so the max-overlap spatial join is
	// unambiguous (X0A sits outside every CMA → province fallback). The
	// FSA→CMA results match the prior prefix-table fixtures, so the golden
	// outputs are unchanged.
	cmaFields := []dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}, {"CMANAME", 30}, {"PRUID", 2}}
	cmaRows := [][]string{
		{"535", "B", "Toronto", "35"},
		{"933", "B", "Vancouver", "59"},
		{"421", "B", "Sherbrooke", "24"},
		// Ottawa-Gatineau is multi-province (one row per province).
		{"505", "B", "Ottawa - Gatineau", "35"},
		{"505", "B", "Ottawa - Gatineau", "24"},
	}
	cmaRings := [][]shp.Point{
		square(0, 0, 100, 100),   // 535 Toronto
		square(200, 0, 300, 100), // 933 Vancouver
		square(400, 0, 500, 100), // 421 Sherbrooke
		square(600, 0, 700, 100), // 505 Ottawa-Gatineau (ON part)
		square(700, 0, 800, 100), // 505 Ottawa-Gatineau (QC part)
	}
	fsaFields := []dbfFieldDef{{"CFSAUID", 3}, {"PRUID", 2}}
	fsaRows := [][]string{
		{"M5V", "35"},
		{"M1B", "35"},
		{"V5Z", "59"},
		{"V6X", "59"},
		{"X0A", "62"},
		// Ottawa-Gatineau portions: K1A is Ontario, J8X is Quebec.
		{"K1A", "35"},
		{"J8X", "24"},
	}
	fsaRings := [][]shp.Point{
		square(10, 10, 20, 20),         // M5V → Toronto box (city-leaf wins)
		square(30, 30, 40, 40),         // M1B → Toronto box → toronto-cma
		square(210, 10, 220, 20),       // V5Z → Vancouver box → metro-vancouver
		square(230, 30, 240, 40),       // V6X → Vancouver box (city-leaf wins)
		square(1000, 1000, 1010, 1010), // X0A → outside all CMAs → province
		// Both K1A and J8X resolve to Ottawa-Gatineau (CMA UID 505) by the
		// spatial join; the ON/QC portion split is then driven by each FSA's
		// PRUID (35 vs 24) in crosswalk, not by which box the polygon sits in.
		square(610, 10, 620, 20), // K1A (PRUID 35) → on-portion
		square(710, 10, 720, 20), // J8X (PRUID 24) → qc-portion
	}
	writeShapefileZip(t, filepath.Join(srcDir, "lcma000b21a_e.zip"), "lcma000b21a_e", cmaFields, cmaRows, cmaRings)
	writeShapefileZip(t, filepath.Join(srcDir, "lfsa000b21a_e.zip"), "lfsa000b21a_e", fsaFields, fsaRows, fsaRings)

	// Overrides are read from outDir (regions_ca_cma_overrides.toml), now
	// data rather than the compiled cmaOverrides map. Stage the canonical
	// overrides the fixture exercises so the golden output is unchanged:
	// CMA 535 → toronto-cma override slug+name; 933 → metro-vancouver
	// override slug+kind. CMA 421 (Sherbrooke) has no override and
	// auto-generates, exercising the no-override region writer path.
	overridesTOML := `[[override]]
cma_uid = "535"
slug = "toronto-cma"
name = "Greater Toronto Area"

[[override]]
cma_uid = "933"
slug = "metro-vancouver"
name = "Metro Vancouver"
kind = "ca:regional-district"

[[override]]
cma_uid = "505"
slug = "ottawa-gatineau-cma"
name = "Ottawa-Gatineau"
`
	if err := os.WriteFile(filepath.Join(outDir, "regions_ca_cma_overrides.toml"), []byte(overridesTOML), 0o644); err != nil {
		t.Fatalf("stage overrides: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := Regenerate(context.Background(), srcDir, outDir, etl.TargetAll, logger); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	for _, name := range []string{"regions_ca_cmas.toml", "postal_codes_ca.csv"} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read output %s: %v", name, err)
		}
		goldenPath := filepath.Join(goldenDir, name)
		if *update {
			if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
				t.Fatalf("update golden %s: %v", name, err)
			}
			continue
		}
		want, err := os.ReadFile(goldenPath)
		if err != nil {
			t.Fatalf("read golden %s (run with -update first): %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s drifted from golden. Run `go test ./internal/etl/ca -run CAGoldenDeterminism -update` and review the diff.", name)
		}
	}
}
