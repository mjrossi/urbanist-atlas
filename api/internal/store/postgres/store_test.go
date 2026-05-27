//go:build integration

// Integration tests for the Postgres store. Build-tagged so the
// default `go test ./...` run stays fast and doesn't require Docker;
// run via `just api-test-integration` or
// `go test ./... -tags=integration`.
//
// Uses testcontainers-go's postgres module to spin up an ephemeral
// 17-alpine container per test binary, applies the embedded
// migrations via goose against database/sql, then exercises the
// adapter end-to-end through the same atlas.Store interface the
// production server uses.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
	"github.com/mjrossi/urbanist-atlas/api/migrations"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Shared Postgres container for the integration suite. Booted once in
// TestMain, migrated, then snapshotted; each test restores from the
// snapshot in microseconds instead of spinning up its own container
// (which costs ~1.5s of wall time). Snapshot/Restore is a
// CREATE DATABASE … TEMPLATE operation under the hood.
//
// The package-shared state is intentional: tests in this package run
// serially (no t.Parallel), and Restore is destructive — it drops the
// app database and recreates it from the template, killing any open
// connections. Each test gets a freshly-opened pgxpool, so stale
// connections from a prior test never leak into the next one.
var (
	sharedContainer *tcpostgres.PostgresContainer
	sharedDBURL     string
)

func TestMain(m *testing.M) {
	cleanup, err := setupSharedPostgres()
	if err != nil {
		log.Printf("integration test setup failed: %v", err)
		if cleanup != nil {
			cleanup()
		}
		os.Exit(1)
	}
	code := m.Run()
	if cleanup != nil {
		cleanup()
	}
	os.Exit(code)
}

// setupSharedPostgres boots the package-shared container, runs the
// embedded migrations, and saves a "migrated_template" snapshot that
// startPostgres restores per test. Returns a cleanup that terminates
// the container.
func setupSharedPostgres() (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	container, err := tcpostgres.Run(ctx,
		"postgres:17-alpine",
		tcpostgres.WithDatabase("urbanist_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}
	cleanup := func() {
		shutdown, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		_ = container.Terminate(shutdown)
	}

	// Belt-and-suspenders: wait until the DB is actually accepting
	// connections. The Run() above already waits for log readiness,
	// but on slower hosts the listener can lag by a beat.
	waitStrategy := wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second)
	if err := waitStrategy.WaitUntilReady(ctx, container); err != nil {
		return cleanup, fmt.Errorf("wait for postgres: %w", err)
	}

	dbURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return cleanup, fmt.Errorf("connection string: %w", err)
	}
	if err := applyMigrations(ctx, dbURL); err != nil {
		return cleanup, fmt.Errorf("apply migrations: %w", err)
	}
	if err := container.Snapshot(ctx); err != nil {
		return cleanup, fmt.Errorf("snapshot migrated template: %w", err)
	}

	sharedContainer = container
	sharedDBURL = dbURL
	return cleanup, nil
}

// startPostgres restores the shared container's post-migration
// snapshot and hands back a freshly-opened pool. The returned closeFn
// closes the pool; the container itself lives for the lifetime of the
// test binary (see TestMain). Restore is destructive — never call
// startPostgres concurrently with another test holding a pool against
// the same container.
func startPostgres(t *testing.T) (*Store, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := sharedContainer.Restore(ctx); err != nil {
		t.Fatalf("restore postgres snapshot: %v", err)
	}
	store, closeFn, err := Open(ctx, sharedDBURL)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store, closeFn
}

func applyMigrations(ctx context.Context, dbURL string) error {
	cfg, err := pgx.ParseConfig(dbURL)
	if err != nil {
		return err
	}
	sqlDB := stdlib.OpenDB(*cfg)
	defer func() { _ = sqlDB.Close() }()
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	goose.SetBaseFS(migrations.FS)
	return goose.UpContext(ctx, sqlDB, ".")
}

// seedFixture inserts a small NYC fixture so the adapter has something
// to find. Uses the pgx pool directly to avoid coupling the test to
// any sqlc-generated write helpers.
//
// Region graph for the NYC fixture (leaf → parents):
//
//	brooklyn-ny → kings-county-ny → nyc-metro → ny
//	                                            (also sf-bay-area for off-topic org)
func seedFixture(t *testing.T, store *Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := store.Pool()

	stmts := []string{
		// Insert every region we'll reference. All regions up front so
		// subsequent FK references don't trip the constraint.
		`INSERT INTO regions (id, kind, name, slug, country, scope_tier, sort_priority) VALUES
			(1,  'city',   'Brooklyn',         'brooklyn-ny',     'US', 'local',    10),
			(2,  'county', 'Kings County, NY', 'kings-county-ny', 'US', 'local',    20),
			(3,  'metro',  'New York Metro',   'nyc-metro',       'US', 'regional', 40),
			(4,  'state',  'NY',               'ny',              'US', 'regional', 60),
			(99, 'metro',  'SF Bay Area',      'sf-bay-area',     'US', 'regional', 40)`,
		`SELECT setval(pg_get_serial_sequence('regions','id'), 99)`,
		// Region DAG edges: brooklyn-ny → kings-county-ny → nyc-metro → ny
		`INSERT INTO region_parents (region_id, parent_region_id) VALUES
			(1, 2),
			(2, 3),
			(3, 4)`,
		// postal_codes: single leaf pointer (region-graph schema)
		`INSERT INTO postal_codes (postal_code, country, leaf_region_id) VALUES
			('11217', 'US', 1)`,
		`INSERT INTO organizations (id, slug, name, short_desc, website_url, contact_url, tags, status, approved_at) VALUES
			(1, 'transalt-brooklyn', 'Transportation Alternatives — Brooklyn',
				'The Brooklyn committee.', 'https://www.transalt.org', NULL,
				ARRAY['safe-streets','cycling']::text[], 'approved', NOW()),
			(2, 'riders-alliance', 'Riders Alliance',
				'Grassroots transit advocacy.', 'https://www.ridersny.org', NULL,
				ARRAY['transit','grassroots']::text[], 'approved', NOW()),
			(3, 'off-topic-sf', 'Off-Topic SF',
				'Should not appear for an NYC lookup.', 'https://example.org', NULL,
				ARRAY['transit']::text[], 'approved', NOW()),
			(4, 'pending-org', 'Pending Org',
				'Not yet approved.', 'https://example.org', NULL,
				ARRAY['transit']::text[], 'pending', NULL)`,
		`SELECT setval(pg_get_serial_sequence('organizations','id'), 4)`,
		// org 1 → regions 1, 2 (Brooklyn leaf + Kings county; both local)
		// org 2 → region 3 (NYC Metro; regional)
		// org 3 → region 99 (SF Bay Area; out of scope for an NYC lookup)
		// org 4 → region 1 (pending; status filter must exclude it)
		`INSERT INTO organization_regions (organization_id, region_id) VALUES
			(1, 1), (1, 2),
			(2, 3),
			(3, 99),
			(4, 1)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("seed: %v\nstmt: %s", err, s)
		}
	}
}

func TestPostgresStore_ResolveLeafRegion_NormalizesAndReturnsLeaf(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()
	seedFixture(t, store)

	// Whitespace around the code; the adapter must strip it.
	// The DB holds "11217" mapped to the brooklyn-ny leaf region.
	leaf, err := store.ResolveLeafRegion(context.Background(), atlas.CountryUS, " 11217 ")
	if err != nil {
		t.Fatalf("ResolveLeafRegion: %v", err)
	}
	if leaf.Slug != "brooklyn-ny" {
		t.Errorf("leaf slug: want %q, got %q", "brooklyn-ny", leaf.Slug)
	}
	if leaf.ScopeTier != atlas.ScopeLocal {
		t.Errorf("leaf scope_tier: want local, got %q", leaf.ScopeTier)
	}
}

func TestPostgresStore_ResolveLeafRegion_NotFound(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()
	seedFixture(t, store)

	_, err := store.ResolveLeafRegion(context.Background(), atlas.CountryUS, "99999")
	if err != atlas.ErrPostalCodeNotFound {
		t.Errorf("want ErrPostalCodeNotFound, got %v", err)
	}
}

func TestPostgresStore_ResolveLeafRegion_CanadianFSANormalization(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()
	seedFixture(t, store)

	ctx := context.Background()
	// Seed a single Canadian leaf region and postal code.
	_, err := store.Pool().Exec(ctx, `
		INSERT INTO regions (id, kind, name, slug, country, scope_tier) VALUES
			(200, 'city', 'Toronto', 'toronto-on', 'CA', 'local');
		INSERT INTO postal_codes (postal_code, country, leaf_region_id) VALUES
			('M5V', 'CA', 200);
	`)
	if err != nil {
		t.Fatalf("seed CA: %v", err)
	}

	// Full postal code with whitespace + lowercase: adapter must
	// truncate to "M5V" (3-char FSA) before querying.
	leaf, err := store.ResolveLeafRegion(ctx, atlas.CountryCA, "m5v 3a8")
	if err != nil {
		t.Fatalf("ResolveLeafRegion: %v", err)
	}
	if leaf.Slug != "toronto-on" {
		t.Errorf("leaf slug: want %q, got %q", "toronto-on", leaf.Slug)
	}
}

func TestPostgresStore_OrgsForRegions_OnlyApprovedAndPopulatesFullRegions(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()
	seedFixture(t, store)

	orgs, err := store.OrgsForRegions(context.Background(), []int64{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("OrgsForRegions: %v", err)
	}
	gotSlugs := make([]string, 0, len(orgs))
	for _, o := range orgs {
		gotSlugs = append(gotSlugs, o.Slug)
	}
	// "off-topic-sf" only serves region 99 → not in the input set.
	// "pending-org" tagged to region 1 but status=pending → filtered.
	wantSlugs := []string{"transalt-brooklyn", "riders-alliance"}
	if diff := cmp.Diff(wantSlugs, gotSlugs); diff != "" {
		t.Errorf("returned org slugs (-want +got):\n%s", diff)
	}

	// transalt-brooklyn serves regions 1 and 2; both must appear in
	// Org.Regions, in id order (the SQL ORDERS BY region_id).
	for _, o := range orgs {
		if o.Slug != "transalt-brooklyn" {
			continue
		}
		if len(o.Regions) != 2 {
			t.Errorf("transalt-brooklyn regions: want 2, got %d (%+v)", len(o.Regions), o.Regions)
		}
		if o.Regions[0].ID != 1 || o.Regions[1].ID != 2 {
			t.Errorf("transalt-brooklyn region order: want [1,2], got %v", regionIDs(o.Regions))
		}
	}
}

func TestPostgresStore_EndToEndLookup(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()
	seedFixture(t, store)

	res, err := atlas.Lookup(context.Background(), store, atlas.LookupQuery{
		PostalCode: "11217",
		Country:    atlas.CountryUS,
	})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(res.Local) != 1 || res.Local[0].Slug != "transalt-brooklyn" {
		t.Errorf("local: %v", orgSlugs(res.Local))
	}
	if len(res.Regional) != 1 || res.Regional[0].Slug != "riders-alliance" {
		t.Errorf("regional: %v", orgSlugs(res.Regional))
	}
	if res.ResolvedPlaceLabel != "Brooklyn, Kings County, NY — New York Metro" {
		t.Errorf("place label: %q", res.ResolvedPlaceLabel)
	}
}

func TestPostgresStore_OrgsForRegions_EmptyInput(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()

	orgs, err := store.OrgsForRegions(context.Background(), nil)
	if err != nil {
		t.Fatalf("OrgsForRegions(nil): %v", err)
	}
	if len(orgs) != 0 {
		t.Errorf("want empty, got %d", len(orgs))
	}
}

func orgSlugs(orgs []atlas.Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}

func regionIDs(regions []atlas.Region) []int64 {
	out := make([]int64, len(regions))
	for i, r := range regions {
		out[i] = r.ID
	}
	return out
}

// TestStore_AncestorRegions_NYC builds the post-#7.5.2 NYC subgraph
// against a real testcontainers Postgres and verifies the recursive-CTE
// walk. After the borough split, the state edge lives on the boroughs
// (not on `nyc`); `nyc` is a regional intermediate region whose only
// parent is `nyc-metro`. A Brooklyn ZIP walks:
//
//	brooklyn → {nyc, ny} → nyc-metro → nyc-tristate
//
// Depth order: brooklyn(0) → nyc(1, sort 15) → ny(1, sort 60) →
// nyc-metro(2, from nyc) → nyc-tristate(3).
func TestStore_AncestorRegions_NYC(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()

	rid := map[string]int64{}
	upsert := func(slug, name, kind, scope string, sort int32, parents ...string) {
		t.Helper()
		id, err := store.q.UpsertRegion(ctx, gen.UpsertRegionParams{
			Country: "US", Kind: kind, Name: name, Slug: slug,
			ScopeTier: scope, SortPriority: sort,
		})
		if err != nil {
			t.Fatalf("UpsertRegion %s: %v", slug, err)
		}
		rid[slug] = id
		if err := store.q.DeleteRegionParents(ctx, id); err != nil {
			t.Fatalf("DeleteRegionParents %s: %v", slug, err)
		}
		for _, ps := range parents {
			pid, ok := rid[ps]
			if !ok {
				t.Fatalf("parent %q not seeded before %q", ps, slug)
			}
			if err := store.q.InsertRegionParent(ctx, gen.InsertRegionParentParams{
				RegionID:       id,
				ParentRegionID: pid,
			}); err != nil {
				t.Fatalf("InsertRegionParent %s->%s: %v", slug, ps, err)
			}
		}
	}

	upsert("nyc-tristate", "Tri-State Region", "us:multi-state", "regional", 80)
	upsert("ny", "New York", "us:state", "regional", 60)
	upsert("nyc-metro", "New York Metro", "us:metro", "regional", 40, "nyc-tristate")
	upsert("nyc", "New York City", "us:city", "regional", 15, "nyc-metro")
	upsert("brooklyn", "Brooklyn", "us:borough", "local", 10, "nyc", "ny")

	if err := store.q.UpsertPostalCode(ctx, gen.UpsertPostalCodeParams{
		Country: "US", PostalCode: "11217", LeafRegionID: rid["brooklyn"],
	}); err != nil {
		t.Fatalf("UpsertPostalCode: %v", err)
	}

	leaf, err := store.ResolveLeafRegion(ctx, atlas.CountryUS, "11217")
	if err != nil {
		t.Fatalf("ResolveLeafRegion: %v", err)
	}
	if leaf.Slug != "brooklyn" {
		t.Fatalf("leaf = %q, want brooklyn", leaf.Slug)
	}

	ancestry, err := store.AncestorRegions(ctx, leaf.ID)
	if err != nil {
		t.Fatalf("AncestorRegions: %v", err)
	}
	got := make([]string, len(ancestry))
	for i, r := range ancestry {
		got[i] = r.Slug
	}
	want := []string{"brooklyn", "nyc", "ny", "nyc-metro", "nyc-tristate"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ancestor order (-want +got):\n%s", diff)
	}

	// Spot-check parent_slugs hydration on `nyc` — after the split its
	// only parent is `nyc-metro` (the `ny` edge migrated to the
	// boroughs).
	for _, r := range ancestry {
		if r.Slug == "nyc" {
			gotParents := append([]string(nil), r.ParentSlugs...)
			sort.Strings(gotParents)
			wantParents := []string{"nyc-metro"}
			if diff := cmp.Diff(wantParents, gotParents); diff != "" {
				t.Errorf("nyc.parent_slugs (-want +got):\n%s", diff)
			}
		}
		if r.Slug == "brooklyn" {
			gotParents := append([]string(nil), r.ParentSlugs...)
			sort.Strings(gotParents)
			wantParents := []string{"ny", "nyc"}
			if diff := cmp.Diff(wantParents, gotParents); diff != "" {
				t.Errorf("brooklyn.parent_slugs (-want +got):\n%s", diff)
			}
		}
	}
}

// Ensure sql import is exercised so the file doesn't trip an unused
// import lint when the build tag is on but the helper is stripped.
var _ = sql.ErrNoRows
