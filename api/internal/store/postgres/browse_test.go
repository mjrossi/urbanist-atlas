//go:build integration

// Integration tests for the Postgres-backed browse + recent Store
// methods. Uses the same testcontainers harness as store_test.go and
// the same seed loaders pipeline_test.go uses, so the regions /
// postal_codes / orgs row set is the production seed (with the PT
// addition from slice #4.6 — MUBi-nacional sits in pt-nacional and
// must NOT surface from ListRecent).

package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/loadpostal"
	"github.com/mjrossi/urbanist-atlas/api/internal/loadregions"
	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// loadAllSeeds runs the same multi-file region + postal + org loaders
// the `loaddata` orchestrator uses. State/province-tier files load
// first (their slugs are referenced as parents by leaves in the main
// file via cross-file resolution). Caller must hold a Postgres pool
// fresh from startPostgres + applyMigrations.
func loadAllSeeds(ctx context.Context, t *testing.T, store *Store) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	regionFiles := []struct {
		path    string
		country string
	}{
		{repoFile(t, "seed", "regions_us_states.toml"), "US"},
		{repoFile(t, "seed", "regions_us_multistate.toml"), "US"},
		{repoFile(t, "seed", "regions_us_msas.toml"), "US"},
		{repoFile(t, "seed", "regions_us.toml"), "US"},
		{repoFile(t, "seed", "regions_ca_provinces.toml"), "CA"},
		{repoFile(t, "seed", "regions_ca_cmas.toml"), "CA"},
		{repoFile(t, "seed", "regions_ca.toml"), "CA"},
		{repoFile(t, "seed", "regions_pt.toml"), "PT"},
	}
	for _, rf := range regionFiles {
		if _, err := loadregions.LoadFile(ctx, store.Pool(), logger, rf.path, rf.country); err != nil {
			t.Fatalf("loadregions %s (%s): %v", rf.country, rf.path, err)
		}
	}
	postalFiles := []struct {
		path    string
		country atlas.Country
	}{
		{repoFile(t, "seed", "postal_codes_us.csv"), atlas.CountryUS},
		{repoFile(t, "seed", "postal_codes_ca.csv"), atlas.CountryCA},
		{repoFile(t, "seed", "postal_codes_pt.csv"), atlas.Country("PT")},
	}
	for _, pf := range postalFiles {
		if _, err := loadpostal.LoadFile(ctx, store.Pool(), logger, pf.path, pf.country); err != nil {
			t.Fatalf("loadpostal %s: %v", pf.country, err)
		}
	}
	if _, err := seed.LoadFile(ctx, store.Pool(), logger, repoFile(t, "seed", "orgs.toml")); err != nil {
		t.Fatalf("seed orgs: %v", err)
	}
}

func TestPostgresStore_ListRegions_ShapeAndOrdering(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.ListRegions(ctx)
	if err != nil {
		t.Fatalf("ListPlaces: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want >=1 place, got 0")
	}
	// Every entry should be in the default-browse set, non-national,
	// and have at least one org.
	for _, m := range got {
		if !atlas.IsDefaultBrowseKind(m.Region.Kind) {
			t.Errorf("kind outside default-browse set in result: %q (%s)", m.Region.Kind, m.Region.Slug)
		}
		if m.Region.ScopeTier == atlas.ScopeNational {
			t.Errorf("national-tier region in result: %s", m.Region.Slug)
		}
		if m.OrgCount == 0 {
			t.Errorf("zero-org region in result: %s", m.Region.Slug)
		}
	}
	// Ordering: org_count DESC, then name ASC for ties.
	for i := 1; i < len(got); i++ {
		prev, cur := got[i-1], got[i]
		if cur.OrgCount > prev.OrgCount {
			t.Errorf("not descending by org_count: [%d]=%d, [%d]=%d",
				i-1, prev.OrgCount, i, cur.OrgCount)
		}
		if cur.OrgCount == prev.OrgCount && cur.Region.Name < prev.Region.Name {
			t.Errorf("not ascending by name within count tie: [%d]=%q, [%d]=%q",
				i-1, prev.Region.Name, i, cur.Region.Name)
		}
	}
}

func TestPostgresStore_GetRegion_HappyPath(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetRegion(ctx, "nyc-metro")
	if err != nil {
		t.Fatalf("GetPlace: %v", err)
	}
	if got == nil {
		t.Fatal("nil result for known place slug")
	}
	if got.Region.Slug != "nyc-metro" {
		t.Errorf("region slug: want nyc-metro, got %s", got.Region.Slug)
	}
	if len(got.Orgs) == 0 {
		t.Errorf("nyc-metro has no orgs; expected at least one (TransitCenter / TransAlt via nyc descendant)")
	}
	// At least one of the seeded orgs that attach to nyc-metro or its
	// descendants should be present.
	gotSlugs := make(map[string]bool, len(got.Orgs))
	for _, o := range got.Orgs {
		gotSlugs[o.Slug] = true
	}
	wantOneOf := []string{"transitcenter", "transportation-alternatives", "riders-alliance"}
	found := false
	for _, w := range wantOneOf {
		if gotSlugs[w] {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("none of %v in nyc-metro orgs; got %v", wantOneOf, sortedKeys(gotSlugs))
	}
	// Each org's Regions must be hydrated.
	for _, o := range got.Orgs {
		if len(o.Regions) == 0 {
			t.Errorf("org %s has empty Regions; hydration failed", o.Slug)
		}
	}
}

func TestPostgresStore_GetRegion_OrgsOrderedNewestFirst(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetRegion(ctx, "nyc-metro")
	if err != nil {
		t.Fatalf("GetPlace: %v", err)
	}
	if got == nil || len(got.Orgs) < 2 {
		t.Skipf("nyc-metro needs >=2 orgs to assert ordering; got %d", len(got.Orgs))
	}
	// Postgres ORDER BY o.created_at DESC, o.id DESC. Assert it holds.
	for i := 1; i < len(got.Orgs); i++ {
		prev, cur := got.Orgs[i-1], got.Orgs[i]
		if cur.CreatedAt.After(prev.CreatedAt) {
			t.Errorf("not descending by created_at: [%d]=%v (%s), [%d]=%v (%s)",
				i-1, prev.CreatedAt, prev.Slug, i, cur.CreatedAt, cur.Slug)
		}
		if cur.CreatedAt.Equal(prev.CreatedAt) && cur.ID > prev.ID {
			t.Errorf("tied created_at not descending by id: [%d]=%d, [%d]=%d",
				i-1, prev.ID, i, cur.ID)
		}
	}
}

func TestPostgresStore_GetRegion_UnknownSlug_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetRegion(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("err: want nil, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil, got %+v", got)
	}
}

// TestPostgresStore_GetRegion_StateSlugResolves pins the broadened
// detail-endpoint contract: a us:state slug (outside the default
// browse set) now resolves and returns its descendant orgs. Replaces
// the prior "non-place slug returns nil" test.
func TestPostgresStore_GetRegion_StateSlugResolves(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetRegion(ctx, "ny")
	if err != nil {
		t.Fatalf("err: want nil, got %v", err)
	}
	if got == nil {
		t.Fatal("nil result for ny (us:state) — expected the broadened detail endpoint to resolve it")
	}
	if got.Region.Slug != "ny" || got.Region.Kind != "us:state" {
		t.Errorf("region: want slug=ny kind=us:state, got slug=%s kind=%s",
			got.Region.Slug, got.Region.Kind)
	}
	if len(got.Orgs) == 0 {
		t.Errorf("orgs: want >=1 (descendant walk; NY descendants include nyc-metro + nyc + boroughs), got 0")
	}
}

// TestPostgresStore_GetRegion_MultiStateSlugResolves pins resolution
// for us:multi-state regions (Chicagoland, NYC tri-state, DMV).
func TestPostgresStore_GetRegion_MultiStateSlugResolves(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetRegion(ctx, "chicagoland")
	if err != nil {
		t.Fatalf("err: want nil, got %v", err)
	}
	if got == nil {
		t.Fatal("nil result for chicagoland (us:multi-state)")
	}
	if got.Region.Kind != "us:multi-state" {
		t.Errorf("region.kind: want us:multi-state, got %s", got.Region.Kind)
	}
	if len(got.Orgs) == 0 {
		t.Errorf("orgs: want >=1 via descendant walk into chicago-metro + Chicago, got 0")
	}
}

// TestPostgresStore_GetRegion_NationalReturnsNil pins the v1
// editorial gate: national-tier slugs (pt-nacional, future MUBi-
// equivalents) still 404 even though the kind gate is gone.
func TestPostgresStore_GetRegion_NationalReturnsNil(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetRegion(ctx, "pt-nacional")
	if err != nil {
		t.Fatalf("err: want nil, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil for national-tier slug, got %+v", got)
	}
}

func TestPostgresStore_ListRecent_ShapeAndCap(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.ListRecent(ctx)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("ListRecent returned 0 orgs")
	}
	if len(got) > 10 {
		t.Errorf("len: want <= 10, got %d", len(got))
	}
	// Newest first by CreatedAt.
	for i := 1; i < len(got); i++ {
		if got[i].CreatedAt.After(got[i-1].CreatedAt) {
			t.Errorf("not descending by created_at: [%d]=%v, [%d]=%v",
				i-1, got[i-1].CreatedAt, i, got[i].CreatedAt)
		}
	}
}

// MUBi (mubi-nacional) is the slice-#4.6 seed org attached ONLY to
// pt-nacional (scope_tier='national'). It must NOT surface in
// ListRecent's default response. A regression here is the kind of
// silent failure the test is specifically guarding against.
func TestPostgresStore_ListRecent_ExcludesNationalTier(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.ListRecent(ctx)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	for _, o := range got {
		if o.Slug == "mubi-nacional" {
			t.Errorf("national-tier org mubi-nacional leaked into ListRecent: %+v", o)
		}
	}
}

func TestPostgresStore_GetOrgBySlug_HappyPath(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	// transportation-alternatives is a known org in api/seed/orgs.toml
	// attached to nyc-metro + brooklyn descendants.
	got, err := store.GetOrgBySlug(ctx, "transportation-alternatives")
	if err != nil {
		t.Fatalf("GetOrgBySlug: %v", err)
	}
	if got == nil {
		t.Fatal("nil result for known seed slug")
	}
	if got.Slug != "transportation-alternatives" {
		t.Errorf("slug: want transportation-alternatives, got %s", got.Slug)
	}
	if got.Name == "" {
		t.Error("name: want non-empty")
	}
	if len(got.Regions) == 0 {
		t.Errorf("regions: want >= 1 (denormalized), got 0")
	}
	for _, r := range got.Regions {
		if r.Slug == "" {
			t.Error("region with empty slug — hydration failed")
		}
	}
}

func TestPostgresStore_GetOrgBySlug_UnknownSlug(t *testing.T) {
	ctx := context.Background()
	store, closeFn := startPostgres(t)
	defer closeFn()
	loadAllSeeds(ctx, t, store)

	got, err := store.GetOrgBySlug(ctx, "totally-fake-org")
	if err == nil {
		t.Fatalf("err: want ErrOrgNotFound, got nil (result=%+v)", got)
	}
	if !errors.Is(err, atlas.ErrOrgNotFound) {
		t.Errorf("err: want ErrOrgNotFound, got %v", err)
	}
	if got != nil {
		t.Errorf("result: want nil, got %+v", got)
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
