package seedfiles_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

// region returns a minimal one-row regions_<cc>.toml document. The
// added_at loader test only needs the bundle to be structurally valid
// enough for BuildMemStore to reach the org stage.
func region(slug, kind, tier string) *fstest.MapFile {
	doc := "[[region]]\nslug = \"" + slug + "\"\nkind = \"" + kind +
		"\"\nname = \"" + slug + "\"\nscope_tier = \"" + tier + "\"\n"
	return &fstest.MapFile{Data: []byte(doc)}
}

// minimalSeedFS builds the smallest bundle BuildMemStore will accept:
// one region per file it reads for US + CA, postal CSVs anchoring the
// local-tier leaves (so the RGN-02b orphan/zero-anchor check passes),
// and a single org (in regions_us.toml's "testville") carrying the
// added_at under test.
func minimalSeedFS(orgsTOML string) fstest.MapFS {
	const csvHeader = "postal_code,country,leaf_region_slug\n"
	// Anchor the two local-tier leaves (testville, xt) with a postal
	// row each so the local-leaf reachability invariant is satisfied.
	usPostal := csvHeader + "10001,US,testville\n"
	caPostal := csvHeader + "H3A 0G4,CA,xt\n"
	return fstest.MapFS{
		"regions_us_states.toml":     region("xs", "us:state", "regional"),
		"regions_us_multistate.toml": region("xm", "us:multistate", "regional"),
		"regions_us_msas.toml":       region("xa", "us:msa", "regional"),
		"regions_us.toml":            region("testville", "us:city", "local"),
		"regions_ca_provinces.toml":  region("xp", "ca:province", "regional"),
		"regions_ca_cmas.toml":       region("xc", "ca:cma", "regional"),
		"regions_ca.toml":            region("xt", "ca:city", "local"),
		"postal_codes_us.csv":        {Data: []byte(usPostal)},
		"postal_codes_ca.csv":        {Data: []byte(caPostal)},
		"orgs.toml":                  {Data: []byte(orgsTOML)},
	}
}

// TestBuildMemStore_ParsesAddedAt asserts the loader reads a date-only
// added_at from org TOML and lands it on Org.AddedAt at midnight UTC.
func TestBuildMemStore_ParsesAddedAt(t *testing.T) {
	const orgs = `[[org]]
slug = "org-x"
name = "Org X"
short_desc = "d"
website_url = "https://x.org"
region_slugs = ["testville"]
added_at = 2026-05-21
`
	store, err := seedfiles.BuildMemStore(nil, minimalSeedFS(orgs))
	if err != nil {
		t.Fatalf("BuildMemStore: %v", err)
	}
	org, err := store.GetOrgBySlug(context.Background(), "org-x")
	if err != nil {
		t.Fatalf("GetOrgBySlug: %v", err)
	}
	want := time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC)
	if !org.AddedAt.Equal(want) {
		t.Errorf("Org.AddedAt = %v, want %v", org.AddedAt, want)
	}
}

// TestBuildMemStore_RequiresAddedAt asserts the loader rejects any org
// that omits added_at, and that the error names the offending slug so
// an operator can fix the seed file without grep gymnastics. This is
// the regression guard the whole feature exists to provide: a future
// editor who forgets the field gets a loud boot failure, not a silent
// near-year-zero AddedAt that breaks the recent strip.
func TestBuildMemStore_RequiresAddedAt(t *testing.T) {
	const orgs = `[[org]]
slug = "org-x"
name = "Org X"
short_desc = "d"
website_url = "https://x.org"
region_slugs = ["testville"]
`
	_, err := seedfiles.BuildMemStore(nil, minimalSeedFS(orgs))
	if err == nil {
		t.Fatal("BuildMemStore: want error for missing added_at, got nil")
	}
	if !strings.Contains(err.Error(), "org-x") {
		t.Errorf("error must name the offending slug; got %q", err)
	}
	if !strings.Contains(err.Error(), "added_at") {
		t.Errorf("error must name the missing field; got %q", err)
	}
}

// TestBuildMemStore_EmbeddedSeedHasAddedAtEverywhere is the invariant
// guard on the production bundle: every org in orgs.toml carries a
// populated added_at (non-zero year). The point of the whole feature
// is that the loader rejects a missing date; this test is the matching
// proof that the committed seed satisfies that contract today.
func TestBuildMemStore_EmbeddedSeedHasAddedAtEverywhere(t *testing.T) {
	if _, err := seedfiles.BuildMemStore(nil, seedfs.FS); err != nil {
		t.Fatalf("BuildMemStore embed: %v", err)
	}
	f, err := seedfs.FS.Open("orgs.toml")
	if err != nil {
		t.Fatalf("open embedded orgs.toml: %v", err)
	}
	defer f.Close()
	entries, err := seedfiles.ParseOrgs(f)
	if err != nil {
		t.Fatalf("ParseOrgs: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded orgs.toml is empty?")
	}
	for _, e := range entries {
		if e.AddedAt.Year == 0 {
			t.Errorf("org %q has zero added_at — Phase 3 backfill missed it", e.Slug)
		}
	}
}
