package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// byDay is the per-day (unaggregated) read; the default grouping sums
// across the range, so tests that assert on Day must ask for this.
func byDay(from, to, kind string, limit int) atlas.UsageQuery {
	return atlas.UsageQuery{From: from, To: to, Kind: kind, GroupBy: atlas.UsageGroupByDay, Limit: limit}
}

func byKey(from, to, kind string, limit int) atlas.UsageQuery {
	return atlas.UsageQuery{From: from, To: to, Kind: kind, Limit: limit}
}

func TestStore_UpsertUsageCounts_Accumulates(t *testing.T) {
	// Two flushes on the same day must SUM, not replace — the recorder
	// flushes deltas, so a replace would silently discard traffic.
	s := newTestStore(t)
	ctx := context.Background()

	first := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago", Count: 3},
		{Day: "2026-09-01", Kind: "org_view", Key: "active-trans", Count: 1},
	}
	if err := s.UpsertUsageCounts(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago", Count: 2},
	}
	if err := s.UpsertUsageCounts(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.ListUsage(ctx, byDay("2026-09-01", "2026-09-01", "region_view", 10))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago", Count: 5},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListUsage mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_ListUsage_ByDayFiltersRangeAndKind(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed := []atlas.UsageCount{
		{Day: "2026-08-31", Kind: "region_view", Key: "out-of-range", Count: 99},
		{Day: "2026-09-01", Kind: "region_view", Key: "in-range", Count: 5},
		{Day: "2026-09-01", Kind: "org_view", Key: "wrong-kind", Count: 7},
		{Day: "2026-09-02", Kind: "region_view", Key: "also-in-range", Count: 9},
	}
	if err := s.UpsertUsageCounts(ctx, seed); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.ListUsage(ctx, byDay("2026-09-01", "2026-09-02", "region_view", 10))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Ordered by count DESC.
	want := []atlas.UsageCount{
		{Day: "2026-09-02", Kind: "region_view", Key: "also-in-range", Count: 9},
		{Day: "2026-09-01", Kind: "region_view", Key: "in-range", Count: 5},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListUsage mismatch (-want +got):\n%s", diff)
	}

	// Empty kind means "all kinds".
	all, err := s.ListUsage(ctx, byDay("2026-09-01", "2026-09-02", "", 10))
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("kind='' should return all 3 rows in range, got %d", len(all))
	}
}

func TestStore_ListUsage_GroupsByKeyAcrossRange(t *testing.T) {
	// The default grouping sums a bucket over the whole range and leaves
	// Day empty, so one month is one row per key rather than ~31.
	s := newTestStore(t)
	ctx := context.Background()

	seed := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago", Count: 4},
		{Day: "2026-09-02", Kind: "region_view", Key: "chicago", Count: 6},
		{Day: "2026-09-02", Kind: "region_view", Key: "boston", Count: 3},
		{Day: "2026-09-03", Kind: "org_view", Key: "active-trans", Count: 2},
		{Day: "2026-10-01", Kind: "region_view", Key: "chicago", Count: 100},
	}
	if err := s.UpsertUsageCounts(ctx, seed); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.ListUsage(ctx, byKey("2026-09-01", "2026-09-30", "region_view", 10))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// The out-of-range October row must not leak into the September sum.
	want := []atlas.UsageCount{
		{Kind: "region_view", Key: "chicago", Count: 10},
		{Kind: "region_view", Key: "boston", Count: 3},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("grouped ListUsage mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_ListUsage_GroupByKeyRanksByRangeTotal(t *testing.T) {
	// The reason grouping is not just a row-count optimisation: a slug
	// with steady daily traffic must outrank one that spiked once, and a
	// per-day read with a small limit gets that backwards.
	s := newTestStore(t)
	ctx := context.Background()

	var seed []atlas.UsageCount
	for _, day := range []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04"} {
		seed = append(seed, atlas.UsageCount{Day: day, Kind: "region_view", Key: "steady", Count: 10})
	}
	seed = append(seed, atlas.UsageCount{Day: "2026-09-01", Kind: "region_view", Key: "spiky", Count: 25})
	if err := s.UpsertUsageCounts(ctx, seed); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// steady totals 40 over the range; spiky totals 25.
	grouped, err := s.ListUsage(ctx, byKey("2026-09-01", "2026-09-30", "region_view", 1))
	if err != nil {
		t.Fatalf("grouped list: %v", err)
	}
	if len(grouped) != 1 || grouped[0].Key != "steady" || grouped[0].Count != 40 {
		t.Errorf("top bucket should be steady/40, got %+v", grouped)
	}

	// The per-day read ranks by single-day count, so it picks spiky.
	// Asserted so the difference between the two groupings stays visible.
	perDay, err := s.ListUsage(ctx, byDay("2026-09-01", "2026-09-30", "region_view", 1))
	if err != nil {
		t.Fatalf("per-day list: %v", err)
	}
	if len(perDay) != 1 || perDay[0].Key != "spiky" {
		t.Errorf("per-day top row should be spiky, got %+v", perDay)
	}
}

func TestStore_PruneUsage_DropsOlderThanCutoff(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed := []atlas.UsageCount{
		{Day: "2026-01-01", Kind: "region_view", Key: "ancient", Count: 1},
		{Day: "2026-09-01", Kind: "region_view", Key: "recent", Count: 1},
	}
	if err := s.UpsertUsageCounts(ctx, seed); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.PruneUsage(ctx, "2026-06-01"); err != nil {
		t.Fatalf("prune: %v", err)
	}

	got, err := s.ListUsage(ctx, byDay("2020-01-01", "2030-01-01", "", 10))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Key != "recent" {
		t.Errorf("prune should keep only 'recent', got %+v", got)
	}
}

func TestStore_UpsertUsageCounts_EmptyIsNoOp(t *testing.T) {
	// The recorder flushes unconditionally on its interval; an empty
	// buffer must not open a transaction or error.
	s := newTestStore(t)
	if err := s.UpsertUsageCounts(context.Background(), nil); err != nil {
		t.Fatalf("empty upsert should be a no-op, got: %v", err)
	}
}

func TestStore_ListUsage_UnknownKindMatchesNothing(t *testing.T) {
	// A kind outside the migration's CHECK set can never match a row.
	// The store returns empty rather than erroring; rejecting the typo
	// with a 400 is the handler's job (see parseUsageKindParam), because
	// an empty 200 here would otherwise read as "no traffic".
	s := newTestStore(t)
	for _, q := range []atlas.UsageQuery{
		byKey("2026-09-01", "2026-09-30", "not_a_kind", 10),
		byDay("2026-09-01", "2026-09-30", "not_a_kind", 10),
	} {
		got, err := s.ListUsage(context.Background(), q)
		if err != nil {
			t.Fatalf("list (%s): %v", q.GroupBy, err)
		}
		if len(got) != 0 {
			t.Errorf("unknown kind should return no rows, got %+v", got)
		}
	}
}

func TestStore_UsageKindConstantsMatchCheckConstraint(t *testing.T) {
	// internal/usage documents that its Kind* constants "must match the
	// CHECK constraint in migration 0003". Nothing enforced that, so a
	// new kind added on one side only would fail at runtime on the first
	// flush that used it. This pins both directions.
	s := newTestStore(t)
	ctx := context.Background()

	for _, kind := range []string{
		usage.KindRegionView,
		usage.KindOrgView,
		usage.KindLookup,
		usage.KindLookupTier,
		usage.KindLookupResult,
		usage.KindLookupCountry,
	} {
		err := s.UpsertUsageCounts(ctx, []atlas.UsageCount{
			{Day: "2026-09-01", Kind: kind, Key: "k", Count: 1},
		})
		if err != nil {
			t.Errorf("kind %q is declared in Go but rejected by the CHECK constraint: %v", kind, err)
		}
	}

	err := s.UpsertUsageCounts(ctx, []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "not_a_kind", Key: "k", Count: 1},
	})
	if err == nil {
		t.Error("the CHECK constraint should reject a kind outside the declared set")
	}
}
