//go:build integration

// Integration test for the loaddata orchestrator (internal/loaddata).
// Lives in the postgres package so it can share the testcontainers
// harness with the rest of the pipeline suite. Asserts that LoadAll
// populates every table the launch needs (regions, postal_codes,
// organizations) and stays idempotent across re-runs — the same
// invariants pipeline_test.go enforces for the manual chain that
// loaddata.LoadAll wraps.

package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/internal/loaddata"
)

func TestPipeline_LoaddataLoadAll(t *testing.T) {
	store, closeFn := startPostgres(t)
	defer closeFn()

	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	seedDir := repoFile(t, "seed")

	if err := loaddata.LoadAll(ctx, store.Pool(), logger, seedDir); err != nil {
		t.Fatalf("loaddata.LoadAll: %v", err)
	}

	before := snapshotCounts(ctx, t, store)
	if before.Regions == 0 {
		t.Error("regions empty after LoadAll")
	}
	if before.PostalCodes == 0 {
		t.Error("postal_codes empty after LoadAll")
	}
	if before.Organizations == 0 {
		t.Error("organizations empty after LoadAll")
	}

	// Each country in the bundle must have ≥1 region loaded. Catches a
	// missing-country bug where the orchestrator silently drops a code
	// from its list.
	for _, cc := range []string{"US", "CA", "PT"} {
		var n int64
		if err := store.Pool().QueryRow(ctx,
			`SELECT COUNT(*) FROM regions WHERE country = $1`, cc,
		).Scan(&n); err != nil {
			t.Fatalf("count regions for %s: %v", cc, err)
		}
		if n == 0 {
			t.Errorf("expected ≥1 region for %s after LoadAll; got 0", cc)
		}
	}

	// Idempotence: a second LoadAll must produce identical row counts.
	if err := loaddata.LoadAll(ctx, store.Pool(), logger, seedDir); err != nil {
		t.Fatalf("loaddata.LoadAll (2nd): %v", err)
	}
	after := snapshotCounts(ctx, t, store)
	if diff := cmp.Diff(before, after); diff != "" {
		t.Errorf("row counts changed after re-run (-before +after):\n%s", diff)
	}
}
