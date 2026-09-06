package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestStore_UpsertUsageCounts_Accumulates(t *testing.T) {
	// Two flushes on the same day must SUM, not replace — the recorder
	// flushes deltas, so a replace would silently discard traffic.
	s := newTestStore(t)
	ctx := context.Background()

	first := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 3},
		{Day: "2026-09-01", Kind: "org_view", Key: "active-trans", Count: 1},
	}
	if err := s.UpsertUsageCounts(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 2},
	}
	if err := s.UpsertUsageCounts(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.ListUsage(ctx, "2026-09-01", "2026-09-01", "region_view", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 5},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListUsage mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_ListUsage_FiltersRangeAndKind(t *testing.T) {
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

	got, err := s.ListUsage(ctx, "2026-09-01", "2026-09-02", "region_view", 10)
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
	all, err := s.ListUsage(ctx, "2026-09-01", "2026-09-02", "", 10)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("kind='' should return all 3 rows in range, got %d", len(all))
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

	got, err := s.ListUsage(ctx, "2020-01-01", "2030-01-01", "", 10)
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

func TestStore_ListUsage_RejectsUnknownKind(t *testing.T) {
	// A kind outside the migration's CHECK set can never match a row;
	// returning empty (not an error) keeps the admin endpoint simple.
	s := newTestStore(t)
	got, err := s.ListUsage(context.Background(), "2026-09-01", "2026-09-30", "not_a_kind", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unknown kind should return no rows, got %+v", got)
	}
}
