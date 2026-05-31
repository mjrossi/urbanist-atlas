package ca

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

var update = flag.Bool("update", false, "regenerate golden files")

// TestRegenerate_CAGoldenDeterminism runs the full CA Regenerate
// pipeline over synthetic CMA + FSA boundary DBFs (built in-memory and
// zipped the way StatsCan ships them) and asserts byte-for-byte output
// against committed goldens. This locks the CA generator logic — the
// part the real-data seed-check gate doesn't cover (it gates only the
// region TOML, and the 155 MB FSA source is never fetched in CI).
//
// The fixtures exercise the CMA region paths and every FSA anchor path:
//
//   - CMA 535 → toronto-cma         (override slug + name)
//   - CMA 933 → metro-vancouver     (override slug + kind)
//   - CMA 421 → auto-slug           (no override; region writer path)
//   - FSA M5V → toronto             (city-leaf)
//   - FSA M1B → toronto-cma         (cma, via "M" prefix)
//   - FSA V5Z → metro-vancouver     (cma, via "V5" prefix)
//   - FSA V6X → richmond            (city-leaf)
//   - FSA X0A → nu                  (province fallback, PRUID 62)
func TestRegenerate_CAGoldenDeterminism(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	goldenDir := "testdata/golden/expected"

	cmaDBF := buildDBF(t,
		[]dbfFieldDef{{"CMAUID", 3}, {"CMATYPE", 1}, {"CMANAME", 30}, {"PRUID", 2}},
		[][]string{
			{"535", "B", "Toronto", "35"},
			{"933", "B", "Vancouver", "59"},
			{"421", "B", "Sherbrooke", "24"},
		},
	)
	fsaDBF := buildDBF(t,
		[]dbfFieldDef{{"CFSAUID", 3}, {"PRUID", 2}},
		[][]string{
			{"M5V", "35"},
			{"M1B", "35"},
			{"V5Z", "59"},
			{"V6X", "59"},
			{"X0A", "62"},
		},
	)
	writeZipWithDBF(t, filepath.Join(srcDir, "lcma000b21a_e.zip"), "lcma000b21a_e.dbf", cmaDBF)
	writeZipWithDBF(t, filepath.Join(srcDir, "lfsa000b21a_e.zip"), "lfsa000b21a_e.dbf", fsaDBF)

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
		if string(got) != string(want) {
			t.Errorf("%s drifted from golden. Run `go test ./internal/etl/ca -run CAGoldenDeterminism -update` and review the diff.", name)
		}
	}
}
