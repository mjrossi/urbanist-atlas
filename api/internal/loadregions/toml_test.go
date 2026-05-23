package loadregions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse_Minimal(t *testing.T) {
	src := `
[[region]]
slug = "ny"
kind = "us:state"
name = "New York"
scope_tier = "regional"
sort_priority = 60
parents = []

[[region]]
slug = "brooklyn"
kind = "us:borough"
name = "Brooklyn"
scope_tier = "local"
sort_priority = 10
parents = ["nyc"]
`
	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Regions) != 2 {
		t.Fatalf("want 2 regions, got %d", len(f.Regions))
	}
	if f.Regions[1].Slug != "brooklyn" || f.Regions[1].SortPriority != 10 {
		t.Errorf("brooklyn region: %+v", f.Regions[1])
	}
	if got := f.Regions[1].Parents; len(got) != 1 || got[0] != "nyc" {
		t.Errorf("brooklyn parents: %v", got)
	}
}

func TestParse_RejectsUnknownField(t *testing.T) {
	src := `
[[region]]
slug = "ny"
kind = "us:state"
name = "New York"
scope_tier = "regional"
sort_priority = 60
parents = []
mystery_field = "boom"
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParse_RejectsInvalidScopeTier(t *testing.T) {
	src := `
[[region]]
slug = "ny"
kind = "us:state"
name = "New York"
scope_tier = "global"
sort_priority = 60
parents = []
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected error for invalid scope_tier")
	}
}

func TestParse_RejectsEmpty(t *testing.T) {
	_, err := Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestParse_AcceptsNationalScopeTier(t *testing.T) {
	src := `
[[region]]
slug = "pt-nacional"
kind = "pt:nacional"
name = "Portugal"
scope_tier = "national"
sort_priority = 90
parents = []
`
	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Regions) != 1 || f.Regions[0].ScopeTier != "national" {
		t.Errorf("national scope_tier not parsed: %+v", f.Regions)
	}
}

// TestParse_LiveSeedFiles parses every api/seed/regions_*.toml file
// loadregions consumes and runs the same structural validation the
// loader runs. Catches duplicate slugs, missing required fields, and
// bad scope_tier values in the live seed without requiring a Postgres
// testcontainer — so this class of bug surfaces in `just api-test`
// (sub-second) instead of in api-test-integration (~1m30s + Docker).
//
// Cross-file slug resolution (e.g. that every `parents = [...]` entry
// resolves to a region defined in some file) is still deferred to the
// integration suite; this guard is just the per-file structural pass.
//
// `regions_us_msa_overrides.toml` is intentionally skipped: it lives
// in api/seed/ alongside the taxonomy files but is consumed by
// api/internal/etl/us, not by loadregions, and uses a different
// schema (CBSA-keyed editorial pins).
func TestParse_LiveSeedFiles(t *testing.T) {
	matches, err := filepath.Glob("../../seed/regions_*.toml")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no regions_*.toml files found under ../../seed/")
	}
	for _, path := range matches {
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_overrides.toml") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", path, err)
			}
			defer f.Close()
			parsed, err := Parse(f)
			if err != nil {
				t.Fatalf("Parse %s: %v", path, err)
			}
			if len(parsed.Regions) == 0 {
				t.Fatalf("Parse %s: expected at least one region", path)
			}
		})
	}
}
