package us

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// Minimal CBSA fixture — a --target=regions run needs only this plus the
// overrides file; it must not require (or read) the ZCTA crosswalks,
// which feed the postal pass.
const tinyCBSA = `Banner row one
CBSA Code,CBSA Title,Metropolitan/Micropolitan Statistical Area,FIPS State Code,FIPS County Code
35620,"New York-Newark-Jersey City, NY-NJ",Metropolitan Statistical Area,36,061
`

func TestRegenerate_TargetRegionsSkipsPostal(t *testing.T) {
	src := t.TempDir()
	out := t.TempDir()
	// Deliberately NO ZCTA files staged: a regions-only run must succeed
	// without them (proves the ZCTA loads live in the postal branch).
	mustWrite(t, filepath.Join(src, "list1_2023.csv"), tinyCBSA)
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
