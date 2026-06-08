package sqlite_test

import (
	"context"
	"testing"
	"time"
)

func TestCoverageGaps_RecordListPrune(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Strictly-increasing deterministic timestamps so newest-first
	// ordering is stable across the run.
	base := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	var i int
	s.SetClock(func() time.Time {
		i++
		return base.Add(time.Duration(i) * time.Second)
	})

	inputs := []struct{ kind, country, input string }{
		{"lookup", "US", "00000"},
		{"lookup", "CA", "X0X"},
		{"search", "", "no-such-place-1"},
		{"lookup", "US", "11111"},
		{"search", "", "no-such-place-2"},
	}
	for _, in := range inputs {
		if err := s.RecordCoverageGap(ctx, in.kind, in.country, in.input); err != nil {
			t.Fatalf("RecordCoverageGap(%v): %v", in, err)
		}
	}

	got, err := s.ListCoverageGaps(ctx, 10)
	if err != nil {
		t.Fatalf("ListCoverageGaps: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("len = %d, want 5", len(got))
	}
	// Newest-first: the last insert leads.
	if got[0].Kind != "search" || got[0].Input != "no-such-place-2" {
		t.Errorf("newest = %+v, want search/no-such-place-2", got[0])
	}
	if got[0].Country != "" {
		t.Errorf("search gap country = %q, want empty", got[0].Country)
	}
	// Oldest is the first insert; fields round-trip.
	if got[4].Kind != "lookup" || got[4].Country != "US" || got[4].Input != "00000" {
		t.Errorf("oldest = %+v, want lookup/US/00000", got[4])
	}

	// limit caps the result set.
	limited, err := s.ListCoverageGaps(ctx, 2)
	if err != nil {
		t.Fatalf("ListCoverageGaps(2): %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("limited len = %d, want 2", len(limited))
	}

	// Prune keeps only the newest 3.
	if err := s.PruneCoverageGaps(ctx, 3); err != nil {
		t.Fatalf("PruneCoverageGaps: %v", err)
	}
	after, err := s.ListCoverageGaps(ctx, 100)
	if err != nil {
		t.Fatalf("ListCoverageGaps after prune: %v", err)
	}
	if len(after) != 3 {
		t.Fatalf("after prune len = %d, want 3", len(after))
	}
	if after[0].Input != "no-such-place-2" || after[2].Input != "no-such-place-1" {
		t.Errorf("after prune window = %q..%q, want no-such-place-2..no-such-place-1",
			after[0].Input, after[2].Input)
	}

	// maxRows <= 0 is a no-op (unbounded).
	if err := s.PruneCoverageGaps(ctx, 0); err != nil {
		t.Fatalf("PruneCoverageGaps(0): %v", err)
	}
	noop, _ := s.ListCoverageGaps(ctx, 100)
	if len(noop) != 3 {
		t.Fatalf("after no-op prune len = %d, want 3", len(noop))
	}
}
