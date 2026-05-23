package seed

import (
	"os"
	"strings"
	"testing"
)

func TestParse_Minimal(t *testing.T) {
	src := `
[[org]]
slug = "transalt"
name = "Transportation Alternatives"
short_desc = "NYC streets."
website_url = "https://transalt.org"
tags = ["advocacy", "safe-streets"]
region_slugs = ["nyc"]

[[org]]
slug = "tri-state"
name = "Tri-State Transportation Campaign"
short_desc = "Tri-state policy."
website_url = "https://tstc.org"
contact_url = "https://tstc.org/contact"
tags = ["transit", "policy"]
region_slugs = ["nyc-tristate"]
`
	f, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(f.Orgs) != 2 {
		t.Fatalf("want 2 orgs, got %d", len(f.Orgs))
	}
	if f.Orgs[1].ContactURL != "https://tstc.org/contact" {
		t.Errorf("contact_url: %q", f.Orgs[1].ContactURL)
	}
	if len(f.Orgs[0].RegionSlugs) != 1 || f.Orgs[0].RegionSlugs[0] != "nyc" {
		t.Errorf("region_slugs: %v", f.Orgs[0].RegionSlugs)
	}
}

func TestParse_RejectsUnknownField(t *testing.T) {
	src := `
[[org]]
slug = "transalt"
name = "Transportation Alternatives"
short_desc = "NYC streets."
website_url = "https://transalt.org"
tags = []
region_slugs = ["nyc"]
ghost_field = "boom"
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected unknown-field error")
	}
}

func TestParse_RejectsDuplicateSlug(t *testing.T) {
	src := `
[[org]]
slug = "a"
name = "A"
short_desc = "x"
website_url = "https://a.example"
tags = []
region_slugs = ["x"]

[[org]]
slug = "a"
name = "B"
short_desc = "x"
website_url = "https://b.example"
tags = []
region_slugs = ["y"]
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected duplicate-slug error")
	}
}

func TestParse_RejectsEmptyRegionSlugs(t *testing.T) {
	src := `
[[org]]
slug = "a"
name = "A"
short_desc = "x"
website_url = "https://a.example"
tags = []
region_slugs = []
`
	_, err := Parse(strings.NewReader(src))
	if err == nil {
		t.Fatal("expected empty region_slugs error")
	}
}

// TestParse_LiveSeedFile parses api/seed/orgs.toml itself and runs the
// same structural validation the loader runs. Catches duplicate slugs,
// missing required fields, and empty region_slugs in the live seed
// without requiring a Postgres testcontainer — so this class of bug
// surfaces in `just api-test` (sub-second) instead of in
// api-test-integration (~1m30s + Docker).
func TestParse_LiveSeedFile(t *testing.T) {
	const path = "../../seed/orgs.toml"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	parsed, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse %s: %v", path, err)
	}
	if len(parsed.Orgs) == 0 {
		t.Fatalf("Parse %s: expected at least one org", path)
	}
}
