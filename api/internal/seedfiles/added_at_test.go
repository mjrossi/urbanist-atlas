package seedfiles_test

import (
	"context"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
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
// one region per file it reads for US + CA, header-only postal CSVs,
// and a single org (in regions_us.toml's "testville") carrying the
// added_at under test.
func minimalSeedFS(orgsTOML string) fstest.MapFS {
	const csvHeader = "postal_code,country,leaf_region_slug\n"
	return fstest.MapFS{
		"regions_us_states.toml":     region("xs", "us:state", "regional"),
		"regions_us_multistate.toml": region("xm", "us:multistate", "regional"),
		"regions_us_msas.toml":       region("xa", "us:msa", "regional"),
		"regions_us.toml":            region("testville", "us:city", "local"),
		"regions_ca_provinces.toml":  region("xp", "ca:province", "regional"),
		"regions_ca_cmas.toml":       region("xc", "ca:cma", "regional"),
		"regions_ca.toml":            region("xt", "ca:city", "local"),
		"postal_codes_us.csv":        {Data: []byte(csvHeader)},
		"postal_codes_ca.csv":        {Data: []byte(csvHeader)},
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
