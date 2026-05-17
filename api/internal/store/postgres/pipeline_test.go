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

	usRegions := repoFile(t, "seed", "regions_us.toml")
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
	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usRegions, "US"); err != nil {
		t.Fatalf("loadregions US: %v", err)
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
	if !containsSlug(res.Local, "transportation-alternatives") {
		t.Errorf("11217 local: missing transportation-alternatives; got %v", orgSlugList(res.Local))
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

	if _, err := loadregions.LoadFile(ctx, store.Pool(), nil, usRegions, "US"); err != nil {
		t.Fatalf("loadregions US (2nd): %v", err)
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
	_, err := loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us.toml"), "US")
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
			name:         "NYC 11217 (Brooklyn)",
			postal:       "11217",
			country:      atlas.CountryUS,
			mustLocal:    []string{"transportation-alternatives"},
			mustRegional: []string{"transitcenter", "tri-state-transportation-campaign"},
			mustNotLocal: []string{"tri-state-transportation-campaign"},
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
	_, err := loadregions.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "regions_us.toml"), "US")
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
