//go:build integration

// Integration test for the loadpostal → seed → /lookup pipeline. Uses
// the same testcontainers Postgres harness as store_test.go.
//
// The bundled api/seed/postal_codes_us.csv + api/seed/postal_codes_ca.csv
// + api/seed/orgs.toml are loaded end-to-end and the resulting state
// is exercised through atlas.Lookup, the same entry point the HTTP
// handler calls. The test also re-runs each loader a second time and
// asserts that no rows changed — the idempotence guarantee that
// roadmap slice #3 + #4 are required to provide.

package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/internal/loadpostal"
	"github.com/mjrossi/urbanist-atlas/api/internal/loadregions"
	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestPipeline_LoadpostalSeedLookup(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	usStates := repoFile(t, "seed", "regions_us_states.toml")
	usMultistate := repoFile(t, "seed", "regions_us_multistate.toml")
	usMSAs := repoFile(t, "seed", "regions_us_msas.toml")
	usRegions := repoFile(t, "seed", "regions_us.toml")
	caProvinces := repoFile(t, "seed", "regions_ca_provinces.toml")
	caCMAs := repoFile(t, "seed", "regions_ca_cmas.toml")
	caRegions := repoFile(t, "seed", "regions_ca.toml")
	ptRegions := repoFile(t, "seed", "regions_pt.toml")
	usCSV := repoFile(t, "seed", "postal_codes_us.csv")
	caCSV := repoFile(t, "seed", "postal_codes_ca.csv")
	ptCSV := repoFile(t, "seed", "postal_codes_pt.csv")
	orgsYAML := repoFile(t, "seed", "orgs.toml")

	// loadpostal requires regions to be present (leaf slug lookup).
	// orgs.toml has US/CA/PT entries — all three countries' regions must
	// be present before seed, or the seed loader fails on missing
	// region_slugs.
	//
	// State/province-tier files load BEFORE each country's main regions
	// file so the main file's leaves can parent under the states via
	// cross-file resolution (internal/loadregions/write.go's
	// RegionIDBySlug fallback).
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usStates, "US"); err != nil {
		t.Fatalf("loadregions US states: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usMultistate, "US"); err != nil {
		t.Fatalf("loadregions US multistate: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usMSAs, "US"); err != nil {
		t.Fatalf("loadregions US msas: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usRegions, "US"); err != nil {
		t.Fatalf("loadregions US: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, caProvinces, "CA"); err != nil {
		t.Fatalf("loadregions CA provinces: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, caCMAs, "CA"); err != nil {
		t.Fatalf("loadregions CA cmas: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, caRegions, "CA"); err != nil {
		t.Fatalf("loadregions CA: %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, ptRegions, "PT"); err != nil {
		t.Fatalf("loadregions PT: %v", err)
	}

	usSum, err := loadpostal.LoadFile(ctx, store.Pool(), nil, usCSV, atlas.CountryUS)
	if err != nil {
		t.Fatalf("loadpostal US: %v", err)
	}
	if usSum.PostalCodes == 0 {
		t.Fatal("US load wrote zero postal codes")
	}
	// Batched-load assertions. The US ETL produces ~33k rows so the
	// batched path must run multiple chunks (not a single big Exec) and
	// the in-memory slug cache must keep distinct-slug count to the
	// low hundreds (states + MSAs + curated leaves). A regression that
	// reverted to per-row Exec calls would show Batches == PostalCodes.
	if usSum.Batches <= 1 || usSum.Batches >= usSum.PostalCodes {
		t.Errorf("US batching: Batches=%d, PostalCodes=%d (expected many batches but << per-row)", usSum.Batches, usSum.PostalCodes)
	}
	if usSum.DistinctSlugs == 0 || usSum.DistinctSlugs > 1000 {
		t.Errorf("US distinct slug cache: DistinctSlugs=%d (expected low hundreds — states + MSAs + leaves)", usSum.DistinctSlugs)
	}
	caSum, err := loadpostal.LoadFile(ctx, store.Pool(), nil, caCSV, atlas.CountryCA)
	if err != nil {
		t.Fatalf("loadpostal CA: %v", err)
	}
	if caSum.PostalCodes == 0 {
		t.Fatal("CA load wrote zero postal codes")
	}
	ptSum, err := loadpostal.LoadFile(ctx, store.Pool(), nil, ptCSV, atlas.Country("PT"))
	if err != nil {
		t.Fatalf("loadpostal PT: %v", err)
	}
	if ptSum.PostalCodes == 0 {
		t.Fatal("PT load wrote zero postal codes")
	}

	seedSum, err := seed.LoadFile(ctx, store.Pool(), nil, orgsYAML)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if seedSum.OrgsUpserted < 10 {
		t.Errorf("expected >=10 orgs upserted, got %d", seedSum.OrgsUpserted)
	}

	// /lookup against a Brooklyn ZIP: must surface Transportation
	// Alternatives in Local and Tri-State / Riders Alliance in
	// Regional (Riders Alliance attaches to NYC Metro, Tri-State to
	// NY state — both regional).
	res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "11217", Country: atlas.CountryUS})
	if err != nil {
		t.Fatalf("Lookup 11217: %v", err)
	}
	// Post-#7.5.2 borough split: citywide NYC orgs (TransAlt, Riders
	// Alliance, StreetsPAC) attach to the regional `nyc` node and
	// surface in the Regional bucket for borough lookups. Borough-only
	// orgs (none in the seed currently) would be Local.
	if !containsSlug(res.Regional, "transportation-alternatives") {
		t.Errorf("11217 regional: missing transportation-alternatives; got %v", orgSlugList(res.Regional))
	}
	if containsSlug(res.Local, "transportation-alternatives") {
		t.Errorf("11217 local: transportation-alternatives must NOT appear here post-#7.5.2 split; got %v", orgSlugList(res.Local))
	}
	if len(res.Regional) == 0 {
		t.Errorf("11217 regional: want >=1 org, got 0")
	}

	// /lookup against a Toronto FSA: TTCriders is local.
	caRes, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "M5V", Country: atlas.CountryCA})
	if err != nil {
		t.Fatalf("Lookup M5V: %v", err)
	}
	if !containsSlug(caRes.Local, "ttcriders") {
		t.Errorf("M5V local: missing ttcriders; got %v", orgSlugList(caRes.Local))
	}

	// Idempotence: re-running the loaders must produce identical row
	// counts. Snapshot row counts → run again → compare.
	before := snapshotCounts(ctx, t, store)

	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usStates, "US"); err != nil {
		t.Fatalf("loadregions US states (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usMultistate, "US"); err != nil {
		t.Fatalf("loadregions US multistate (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usMSAs, "US"); err != nil {
		t.Fatalf("loadregions US msas (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usRegions, "US"); err != nil {
		t.Fatalf("loadregions US (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, caProvinces, "CA"); err != nil {
		t.Fatalf("loadregions CA provinces (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, caCMAs, "CA"); err != nil {
		t.Fatalf("loadregions CA cmas (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, caRegions, "CA"); err != nil {
		t.Fatalf("loadregions CA (2nd): %v", err)
	}
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, ptRegions, "PT"); err != nil {
		t.Fatalf("loadregions PT (2nd): %v", err)
	}
	if _, err := loadpostal.LoadFile(ctx, store.Pool(), nil, usCSV, atlas.CountryUS); err != nil {
		t.Fatalf("loadpostal US (2nd): %v", err)
	}
	if _, err := loadpostal.LoadFile(ctx, store.Pool(), nil, caCSV, atlas.CountryCA); err != nil {
		t.Fatalf("loadpostal CA (2nd): %v", err)
	}
	if _, err := loadpostal.LoadFile(ctx, store.Pool(), nil, ptCSV, atlas.Country("PT")); err != nil {
		t.Fatalf("loadpostal PT (2nd): %v", err)
	}
	if _, err := seed.LoadFile(ctx, store.Pool(), nil, orgsYAML); err != nil {
		t.Fatalf("seed (2nd): %v", err)
	}

	after := snapshotCounts(ctx, t, store)
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("row counts changed after re-run (loaders not idempotent) (-before +after):\n%s", diff)
	}

	// And the same lookup must still produce the same Local org set.
	resAfter, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "11217", Country: atlas.CountryUS})
	if err != nil {
		t.Fatalf("Lookup 11217 (after re-run): %v", err)
	}
	if diff := cmp.Diff(orgSlugList(res.Local), orgSlugList(resAfter.Local)); diff != "" {
		t.Errorf("11217 local orgs changed after re-run (-before +after):\n%s", diff)
	}
}

type tableCounts struct {
	Regions             int64
	PostalCodes         int64
	Organizations       int64
	OrganizationRegions int64
}

func snapshotCounts(ctx context.Context, t *testing.T, store *Store) tableCounts {
	t.Helper()
	pool := store.Pool()
	var c tableCounts
	queries := []struct {
		sql  string
		dest *int64
	}{
		{`SELECT COUNT(*) FROM regions`, &c.Regions},
		{`SELECT COUNT(*) FROM postal_codes`, &c.PostalCodes},
		{`SELECT COUNT(*) FROM organizations`, &c.Organizations},
		{`SELECT COUNT(*) FROM organization_regions`, &c.OrganizationRegions},
	}
	for _, q := range queries {
		if err := pool.QueryRow(ctx, q.sql).Scan(q.dest); err != nil {
			t.Fatalf("count %s: %v", q.sql, err)
		}
	}
	return c
}

func containsSlug(orgs []atlas.Org, slug string) bool {
	for _, o := range orgs {
		if o.Slug == slug {
			return true
		}
	}
	return false
}

func orgSlugList(orgs []atlas.Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}

// repoFile resolves a path under the repo's api/ tree by walking up
// from the test's cwd. pipeline_test.go runs from
// api/internal/store/postgres, so api/ is three parents up — but
// walking up to find a directory that contains a "go.mod" is more
// robust against future package moves.
func repoFile(t *testing.T, parts ...string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("repoFile cwd: %v", err)
	}
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			candidate := filepath.Join(append([]string{dir}, parts...)...)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			t.Fatalf("repoFile: found go.mod at %s but no %v", dir, parts)
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("repoFile: did not find go.mod walking up from cwd")
	return ""
}

func TestPipeline_WorkedCities(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// orgs.toml has US/CA/PT entries; PT regions must exist before seed.
	// US load order: states → multistate → msas → us (cross-file parent
	// references resolve via the loader's DB lookup fallback).
	_, err := loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_states.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_multistate.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_msas.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_ca_provinces.toml"), "CA")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_ca_cmas.toml"), "CA")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_ca.toml"), "CA")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_pt.toml"), "PT")
	must(err)
	_, err = loadpostal.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "postal_codes_us.csv"), atlas.CountryUS)
	must(err)
	_, err = loadpostal.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "postal_codes_ca.csv"), atlas.CountryCA)
	must(err)
	_, err = loadpostal.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "postal_codes_pt.csv"), atlas.Country("PT"))
	must(err)
	_, err = seed.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "orgs.toml"))
	must(err)

	cases := []struct {
		name         string
		postal       string
		country      atlas.Country
		mustLocal    []string
		mustRegional []string
		mustNotLocal []string
		mustNotAny   []string
	}{
		{
			// Post-#7.5.2 borough split: citywide NYC orgs (TransAlt,
			// Riders Alliance, StreetsPAC) attach to the regional `nyc`
			// node and bucket as Regional. There are no borough-only
			// orgs in the seed yet, so the Local bucket is currently
			// empty for borough ZIPs.
			name:         "NYC 11217 (Brooklyn)",
			postal:       "11217",
			country:      atlas.CountryUS,
			mustRegional: []string{"transportation-alternatives", "transitcenter", "tri-state-transportation-campaign"},
			mustNotLocal: []string{"transportation-alternatives", "tri-state-transportation-campaign"},
		},
		{
			name:         "Hoboken 07302",
			postal:       "07302",
			country:      atlas.CountryUS,
			mustRegional: []string{"transitcenter", "tri-state-transportation-campaign"},
			mustNotAny:   []string{"transportation-alternatives"},
		},
		{
			name:         "Vancouver V6B",
			postal:       "V6B",
			country:      atlas.CountryCA,
			mustRegional: []string{"hub-cycling", "movement-metro-vancouver"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: c.postal, Country: c.country})
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			localSlugs := slugSet(got.Local)
			regionalSlugs := slugSet(got.Regional)
			for _, s := range c.mustLocal {
				if !localSlugs[s] {
					t.Errorf("expected %q in Local; got %v", s, keysOf(localSlugs))
				}
			}
			for _, s := range c.mustRegional {
				if !regionalSlugs[s] {
					t.Errorf("expected %q in Regional; got %v", s, keysOf(regionalSlugs))
				}
			}
			for _, s := range c.mustNotLocal {
				if localSlugs[s] {
					t.Errorf("expected %q NOT in Local", s)
				}
			}
			for _, s := range c.mustNotAny {
				if localSlugs[s] || regionalSlugs[s] {
					t.Errorf("expected %q NOT in any bucket", s)
				}
			}
		})
	}

	t.Run("DC 20017 anchors at washington-dc city leaf, not at the multi-state metro", func(t *testing.T) {
		got, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "20017", Country: atlas.CountryUS})
		if err != nil {
			t.Fatalf("Lookup 20017: %v", err)
		}
		if len(got.ResolvedAncestry) == 0 || got.ResolvedAncestry[0].Slug != "washington-dc" {
			t.Errorf("DC 20017 leaf = %q, want washington-dc (anchored at the city leaf added in slice #7.5 follow-up; without it ZIP 20017 falls back to washington-dc-metro and the breadcrumb buries DC after MD/VA/WV)", firstSlug(got.ResolvedAncestry))
		}
		ancestrySlugs := make([]string, len(got.ResolvedAncestry))
		for i, r := range got.ResolvedAncestry {
			ancestrySlugs[i] = r.Slug
		}
		for _, expected := range []string{"washington-dc", "washington-dc-metro", "dc"} {
			found := false
			for _, s := range ancestrySlugs {
				if s == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("DC 20017 ancestry missing %q; got %v", expected, ancestrySlugs)
			}
		}
		// Diamond dedup: `dc` is reachable both at depth 1 (direct
		// parent of the washington-dc city leaf) and depth 2 (parent
		// of washington-dc-metro, which is itself a depth-1 parent).
		// The recursive CTE's UNION dedupes on the full tuple including
		// depth, so without the outer DISTINCT ON (id) in
		// queries/lookup.sql the same region surfaces twice. Lock that
		// behavior in.
		counts := map[string]int{}
		for _, s := range ancestrySlugs {
			counts[s]++
		}
		for slug, n := range counts {
			if n > 1 {
				t.Errorf("DC 20017 ancestry contains %q %d times; want exactly once (DAG-diamond dedup regression)", slug, n)
			}
		}
		// Place label: "Washington — Washington Metro" (broad = the
		// us:metro ancestor; inner is empty because dc is sort 60, at
		// the state-tier exclusion line in placeLabel).
		if want := "Washington — Washington Metro"; got.ResolvedPlaceLabel != want {
			t.Errorf("DC 20017 place label = %q, want %q", got.ResolvedPlaceLabel, want)
		}
	})

	t.Run("gap-state ZIPs return graceful empty (Local + Regional both empty, no error)", func(t *testing.T) {
		// Slice #7.6 documented 9 US states + 4 CA province/territory
		// blocks as state-floor gaps (no demonstrably-active statewide
		// advocacy org as of 2026-05-20). The /lookup contract for ZIPs
		// in these regions is: 200 OK with empty Local + empty Regional
		// — graceful empty, not an error and not a fallback to a
		// national umbrella. This test locks in that contract.
		//
		// 82001 (Cheyenne, WY) anchors at cheyenne-wy-metro, walks up
		// to wy. Neither node has any anchored org.
		// X0A (Iqaluit-area FSA, NU) anchors directly at nu (no CMA
		// in scope for NU). NU is a documented territory gap.
		//
		// If a future seed edit attaches an org to wy / cheyenne-wy-
		// metro / nu (closing a gap), this test will fail and the
		// orgs.toml `# gap` comment for that region needs to be
		// removed in the same change.
		cases := []struct {
			name                 string
			postal               string
			country              atlas.Country
			wantAncestryContains string
		}{
			{"WY (state-floor gap, metro→state walk)", "82001", atlas.CountryUS, "wy"},
			{"NU (CA territory gap, direct anchor)", "X0A", atlas.CountryCA, "nu"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: c.postal, Country: c.country})
				if err != nil {
					t.Fatalf("Lookup %s: %v", c.postal, err)
				}
				if len(got.Local) != 0 {
					t.Errorf("Lookup %s: Local must be empty for gap state; got %v", c.postal, orgSlugList(got.Local))
				}
				if len(got.Regional) != 0 {
					t.Errorf("Lookup %s: Regional must be empty for gap state; got %v", c.postal, orgSlugList(got.Regional))
				}
				// Distinguish "graceful empty for gap state" from "no
				// leaf found at all" — confirm the ancestor walk
				// actually reached the gap region.
				ancestrySlugs := make(map[string]bool, len(got.ResolvedAncestry))
				for _, r := range got.ResolvedAncestry {
					ancestrySlugs[r.Slug] = true
				}
				if !ancestrySlugs[c.wantAncestryContains] {
					t.Errorf("Lookup %s: ancestry must include %q to confirm gap-region walk happened; got %v",
						c.postal, c.wantAncestryContains, keysOf(ancestrySlugs))
				}
			})
		}
	})
}

func firstSlug(rs []atlas.Region) string {
	if len(rs) == 0 {
		return ""
	}
	return rs[0].Slug
}

func slugSet(orgs []atlas.Org) map[string]bool {
	out := make(map[string]bool, len(orgs))
	for _, o := range orgs {
		out[o.Slug] = true
	}
	return out
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestPipeline_PT_ValidationFixture exercises the slice #4.6 PT seed
// data end-to-end and asserts the validation invariants the spec
// promised: multi-parent walks, AML's cross-NUTS-II span, autonomous
// region as parallel hierarchy, and the national-tier filter in the
// default lookup.
//
// See docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md
// for the design and expected behavior.
func TestPipeline_PT_ValidationFixture(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Load all three countries' regions before seeding orgs — orgs.toml
	// references US/CA region slugs that must exist or the org seed
	// loader fails. The PT-specific assertions below only inspect PT
	// state, so the additional US/CA load is just a no-op precondition.
	// US load order: states → multistate → msas → us; cross-file parent
	// references resolve via the loader's DB lookup fallback.
	_, err := loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_states.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_multistate.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_msas.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_ca_provinces.toml"), "CA")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_ca_cmas.toml"), "CA")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_ca.toml"), "CA")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_pt.toml"), "PT")
	must(err)
	_, err = loadpostal.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "postal_codes_pt.csv"), atlas.Country("PT"))
	must(err)
	_, err = seed.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "orgs.toml"))
	must(err)

	t.Run("national tier filtered from default lookup", func(t *testing.T) {
		// A Lisboa postal-code lookup must NOT surface mubi-nacional
		// (attached to pt-nacional, scope_tier='national'), and the
		// resolved_ancestry must not include the pt-nacional region.
		res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "1100-001", Country: atlas.Country("PT")})
		if err != nil {
			t.Fatalf("Lookup 1100-001: %v", err)
		}
		all := slugSet(res.Local)
		for k := range slugSet(res.Regional) {
			all[k] = true
		}
		if all["mubi-nacional"] {
			t.Errorf("default Lisboa lookup surfaced mubi-nacional (should be filtered as scope_tier='national')")
		}
		for _, r := range res.ResolvedAncestry {
			if r.Slug == "pt-nacional" {
				t.Errorf("resolved_ancestry includes pt-nacional (should be filtered): %+v", res.ResolvedAncestry)
			}
			if r.ScopeTier == atlas.ScopeNational {
				t.Errorf("resolved_ancestry includes a national-tier region %q (filter failed)", r.Slug)
			}
		}
	})

	t.Run("Lisboa Baixa hits local chapters", func(t *testing.T) {
		// 1100-001 → santa-maria-maior → lisboa-municipio. Local-bucket
		// orgs attached to lisboa-municipio (mubi-lisboa,
		// lisboa-para-pessoas) must surface in Local.
		res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "1100-001", Country: atlas.Country("PT")})
		if err != nil {
			t.Fatalf("Lookup 1100-001: %v", err)
		}
		local := slugSet(res.Local)
		for _, want := range []string{"mubi-lisboa", "lisboa-para-pessoas"} {
			if !local[want] {
				t.Errorf("expected %q in Local; got %v", want, keysOf(local))
			}
		}
	})

	t.Run("Setúbal multi-parent walks both NUTS-II via AML", func(t *testing.T) {
		// 2900-001 → setubal-municipio → {distrito-setubal, aml}.
		// Through aml's multi-NUTS-II parent edges, the ancestor walk
		// must reach BOTH nuts-ii-grande-lisboa AND
		// nuts-ii-peninsula-setubal. Distrito-lisboa must NOT appear
		// (Setúbal isn't in that distrito).
		res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "2900-001", Country: atlas.Country("PT")})
		if err != nil {
			t.Fatalf("Lookup 2900-001: %v", err)
		}
		ancestrySlugs := map[string]bool{}
		for _, r := range res.ResolvedAncestry {
			ancestrySlugs[r.Slug] = true
		}
		for _, want := range []string{"setubal-municipio", "aml", "distrito-setubal", "nuts-ii-grande-lisboa", "nuts-ii-peninsula-setubal"} {
			if !ancestrySlugs[want] {
				t.Errorf("Setúbal ancestry missing %q; got %v", want, keysOf(ancestrySlugs))
			}
		}
		if ancestrySlugs["distrito-lisboa"] {
			t.Errorf("Setúbal ancestry incorrectly includes distrito-lisboa")
		}
	})

	t.Run("Funchal autonomous region stops at Madeira", func(t *testing.T) {
		// 9000-001 → funchal-concelho → regiao-autonoma-madeira and
		// stops. No NUTS-II parent. No mainland regions.
		res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "9000-001", Country: atlas.Country("PT")})
		if err != nil {
			t.Fatalf("Lookup 9000-001: %v", err)
		}
		ancestrySlugs := map[string]bool{}
		for _, r := range res.ResolvedAncestry {
			ancestrySlugs[r.Slug] = true
		}
		for _, want := range []string{"funchal-concelho", "regiao-autonoma-madeira"} {
			if !ancestrySlugs[want] {
				t.Errorf("Funchal ancestry missing %q; got %v", want, keysOf(ancestrySlugs))
			}
		}
		for _, mainland := range []string{"distrito-lisboa", "distrito-porto", "nuts-ii-norte", "nuts-ii-centro", "aml", "amp"} {
			if ancestrySlugs[mainland] {
				t.Errorf("Funchal ancestry incorrectly includes mainland region %q", mainland)
			}
		}
	})

	t.Run("hyphen-stripped postal code normalization round-trips", func(t *testing.T) {
		// Both "1100-001" and "1100001" must resolve to the same leaf.
		withHyphen, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "1100-001", Country: atlas.Country("PT")})
		if err != nil {
			t.Fatalf("Lookup 1100-001: %v", err)
		}
		withoutHyphen, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "1100001", Country: atlas.Country("PT")})
		if err != nil {
			t.Fatalf("Lookup 1100001: %v", err)
		}
		if diff := cmp.Diff(orgSlugList(withHyphen.Local), orgSlugList(withoutHyphen.Local)); diff != "" {
			t.Errorf("hyphen vs no-hyphen produced different Local orgs (-with +without):\n%s", diff)
		}
	})
}

// TestPipeline_NationalTierAncestor_FilteredByCTE is the safety-net
// companion to TestLookup_NationalTierOrg_ExcludedFromDefaultLookup in
// api/internal/httpapi/lookup_test.go. That test uses a sibling-
// attachment fixture (national region NOT in the ancestor chain) and
// works against MemStore + Postgres equivalently — but it does not
// exercise the recursive-CTE filter at queries/lookup.sql:23,29
// (`WHERE r.scope_tier <> 'national'`).
//
// This test puts a national region directly in the ancestor chain:
//
//	leaf-city → parent-region → NATIONAL-region
//
// Without the CTE filter, the recursive walk would surface the national
// region (and orgs attached only to it). With the filter, the walk
// stops at parent-region. This is the defense-in-depth that protects
// against an editorial mistake (a parent edge from the leaf chain into
// a national region) — the data shape intentionally avoids such edges,
// but the filter ensures national-tier orgs stay hidden if one slips
// in. See queries/lookup.sql:14-19 and the slice #4.6 spec for the
// rationale.
//
// MemStore does NOT filter at this seam (see the comment on
// TestLookup_NationalTierOrg_ExcludedFromDefaultLookup), so this test
// is meaningful only against Postgres — hence its placement here under
// the integration build tag.
func TestPipeline_NationalTierAncestor_FilteredByCTE(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()

	// Build the topology directly via SQL: a leaf city, a local parent,
	// and a national region as the parent of the local parent.
	stmts := []string{
		`INSERT INTO regions (id, kind, name, slug, country, scope_tier, sort_priority) VALUES
			(7001, 'city',   'Leaf City',     'leaf-city',     'XX', 'local',    10),
			(7002, 'region', 'Parent Region', 'parent-region', 'XX', 'local',    20),
			(7003, 'nation', 'Nation X',      'nation-x',      'XX', 'national', 90)`,
		`SELECT setval(pg_get_serial_sequence('regions','id'), 7003)`,
		// Edges: leaf-city → parent-region → nation-x.
		// nation-x is intentionally IN the ancestor chain so the CTE's
		// scope_tier filter is the only thing that stops it from
		// surfacing.
		`INSERT INTO region_parents (region_id, parent_region_id) VALUES
			(7001, 7002),
			(7002, 7003)`,
		`INSERT INTO postal_codes (postal_code, country, leaf_region_id) VALUES
			('99999', 'XX', 7001)`,
		`INSERT INTO organizations (id, slug, name, short_desc, website_url, contact_url, tags, status, approved_at) VALUES
			(7001, 'leaf-spoke', 'Leaf Spoke', 'Local advocacy.',
				'https://example.org/leaf', NULL,
				ARRAY['transit']::text[], 'approved', NOW()),
			(7002, 'national-only', 'National Only', 'National umbrella.',
				'https://example.org/national', NULL,
				ARRAY['transit']::text[], 'approved', NOW())`,
		`SELECT setval(pg_get_serial_sequence('organizations','id'), 7002)`,
		// leaf-spoke attaches to leaf-city (local).
		// national-only attaches ONLY to nation-x. Without the CTE
		// filter, the ancestor walk would yield {leaf-city, parent-
		// region, nation-x} and national-only would land in Regional.
		// With the filter, the walk stops at parent-region and
		// national-only is invisible to atlas.Lookup.
		`INSERT INTO organization_regions (organization_id, region_id) VALUES
			(7001, 7001),
			(7002, 7003)`,
	}
	for _, s := range stmts {
		if _, err := store.Pool().Exec(ctx, s); err != nil {
			t.Fatalf("seed: %v\nstmt: %s", err, s)
		}
	}

	// First assert the AncestorRegions seam directly — this is where
	// the CTE filter applies, and the most narrowly-targeted way to
	// detect a filter regression.
	ancestry, err := store.AncestorRegions(ctx, 7001)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	for _, r := range ancestry {
		if r.ScopeTier == atlas.ScopeNational {
			t.Errorf("AncestorRegions returned a national-tier region %q (CTE filter regression)", r.Slug)
		}
		if r.Slug == "nation-x" {
			t.Errorf("AncestorRegions returned nation-x — the CTE filter at queries/lookup.sql:23,29 must drop it")
		}
	}

	// Then assert the end-to-end Lookup surface: national-only must
	// not appear in either bucket; leaf-spoke must appear in Local.
	res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "99999", Country: atlas.Country("XX")})
	if err != nil {
		t.Fatalf("Lookup 99999: %v", err)
	}
	all := slugSet(res.Local)
	for k := range slugSet(res.Regional) {
		all[k] = true
	}
	if all["national-only"] {
		t.Errorf("national-only org surfaced in default lookup (CTE filter regression); got Local=%v Regional=%v",
			orgSlugList(res.Local), orgSlugList(res.Regional))
	}
	if !all["leaf-spoke"] {
		t.Errorf("leaf-spoke missing from default lookup; got Local=%v Regional=%v",
			orgSlugList(res.Local), orgSlugList(res.Regional))
	}
}

// TestPipeline_HUDBackfill_ZIP20811 is the integration regression for
// the slice #7.5.5 HUD backfill. ZIP 20811 (a Bethesda P.O. Box
// covering NIH/Walter Reed) is excluded from Census ZCTA, so a
// ZCTA-only postal pipeline returns postal-code-not-found. The HUD
// backfill anchors it at washington-dc-metro via Montgomery County,
// MD (FIPS 24031 → CBSA 47900 → slug washington-dc-metro).
//
// This test loads the real committed seed (which now includes HUD
// backfill rows) and verifies 20811 resolves to washington-dc-metro
// end-to-end. If an operator regenerates `postal_codes_us.csv`
// without the HUD source file staged (silent-skip path in
// internal/etl/us/us.go), 20811 disappears from the seed and this
// test fails — turning the silent ETL skip into a loud test failure.
// Unit-level proof that the HUD parser + crosswalk produce the right
// anchor lives in api/internal/etl/us/{hud,crosswalk}_test.go.
func TestPipeline_HUDBackfill_ZIP20811(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err := loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_states.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_multistate.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us_msas.toml"), "US")
	must(err)
	_, err = loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us.toml"), "US")
	must(err)
	_, err = loadpostal.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "postal_codes_us.csv"), atlas.CountryUS)
	must(err)

	var loaded int
	if err := store.Pool().QueryRow(ctx, `
		SELECT COUNT(*) FROM postal_codes WHERE postal_code = '20811' AND country = 'US'
	`).Scan(&loaded); err != nil {
		t.Fatalf("count 20811 in loaded seed: %v", err)
	}
	if loaded == 0 {
		t.Fatal("20811 missing from postal_codes after seed load; HUD backfill dropped out of api/seed/postal_codes_us.csv (operator regenerated without etl/sources/us/hud_zip_county_*.csv staged?)")
	}

	res, err := atlas.Lookup(ctx, store, atlas.LookupQuery{PostalCode: "20811", Country: atlas.CountryUS})
	if err != nil {
		t.Fatalf("Lookup 20811: %v", err)
	}
	ancestrySlugs := make(map[string]bool, len(res.ResolvedAncestry))
	for _, r := range res.ResolvedAncestry {
		ancestrySlugs[r.Slug] = true
	}
	if !ancestrySlugs["washington-dc-metro"] {
		t.Errorf("20811 ancestry missing washington-dc-metro; got %v", keysOf(ancestrySlugs))
	}
}
