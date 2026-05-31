package us

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

// TestRegenerate_GoldenDeterminism runs the full US Regenerate pipeline
// over tiny synthetic sources (committed under testdata/golden/sources)
// and asserts byte-for-byte output against committed goldens. This is
// the gate that protects the HUD-dependent postal logic the real-data
// seed-check gate can't run in CI. The fixtures deliberately exercise
// every anchor path:
//
//   - 10001 → manhattan      (nyc-borough, via county 36061)
//   - 06604 → bridgeport     (city-leaf, via place 0908000)
//   - 06110 → hartford metro (msa, via county 09110 in countyToMSA)
//   - 06010 → hartford metro (ct-reconciled:msa — legacy county 09003
//     strands it at bare `ct`; HUD's current county 09110 repairs it)
//   - 05001 → vt             (state fallback; reconcile must leave it)
//   - 06888 → hartford metro (hud:msa backfill — not in the ZCTA files)
//
// Regenerate with `-update` to refresh the goldens after an intentional
// generator change, then review the diff.
func TestRegenerate_GoldenDeterminism(t *testing.T) {
	srcDir := "testdata/golden/sources"
	goldenDir := "testdata/golden/expected"
	outDir := t.TempDir()

	// Overrides are read from outDir; this fixture needs none.
	mustWrite(t, filepath.Join(outDir, "regions_us_msa_overrides.toml"), "")

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := Regenerate(context.Background(), srcDir, outDir, etl.TargetAll, logger); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	for _, name := range []string{"regions_us_msas.toml", "postal_codes_us.csv"} {
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
			t.Errorf("%s drifted from golden. Run `go test ./internal/etl/us -run GoldenDeterminism -update` and review the diff.", name)
		}
	}
}
