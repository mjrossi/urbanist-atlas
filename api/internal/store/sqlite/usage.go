package sqlite

import (
	"context"
	"fmt"

	sqlitegen "github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// maxUsageListLimit caps a single admin usage read. Generous because
// the digest legitimately wants a few hundred buckets per month, but
// bounded so a malformed query can't stream the whole table.
const maxUsageListLimit = 1000

// defaultUsageListLimit is applied when the caller passes limit <= 0.
const defaultUsageListLimit = 100

// UpsertUsageCounts accumulates a batch of daily usage deltas in one
// transaction. Counts SUM into any existing (day, kind, key) row, so
// repeated flushes within a day compose correctly.
//
// The whole batch is one transaction: a partial flush would double-count
// on retry, and the recorder has already cleared its buffer by the time
// this runs. An empty batch is a no-op — the recorder flushes on a timer
// whether or not traffic arrived.
func (s *Store) UpsertUsageCounts(ctx context.Context, counts []atlas.UsageCount) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite.UpsertUsageCounts: begin: %w", err)
	}
	// Rollback is a no-op once Commit succeeds; safe to always defer.
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	for _, c := range counts {
		if err := qtx.UpsertUsageCount(ctx, sqlitegen.UpsertUsageCountParams{
			Day:       c.Day,
			Kind:      c.Kind,
			BucketKey: c.Key,
			Count:     int64(c.Count),
		}); err != nil {
			return fmt.Errorf("sqlite.UpsertUsageCounts: %s/%s/%s: %w", c.Day, c.Kind, c.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite.UpsertUsageCounts: commit: %w", err)
	}
	return nil
}

// ListUsage returns accumulated buckets between the inclusive day
// bounds, highest-count first. An empty Kind returns every kind.
//
// The default grouping (atlas.UsageGroupByKey) sums each bucket over the
// whole range and leaves Day empty. atlas.UsageGroupByDay returns the
// stored per-day rows instead. Grouping matters for correctness, not
// just row count: a capped per-day read ranks by single-day count, so a
// slug with steady daily traffic sorts below one that spiked once.
//
// The two-query split per grouping (rather than one query with an
// optional filter) is forced by sqlc's SQLite parser, which rejects a
// named argument referenced twice — see the note in
// queries/usage_daily.sql.
func (s *Store) ListUsage(ctx context.Context, q atlas.UsageQuery) ([]atlas.UsageCount, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = defaultUsageListLimit
	}
	if limit > maxUsageListLimit {
		limit = maxUsageListLimit
	}

	if q.GroupBy == atlas.UsageGroupByDay {
		return s.listUsageByDay(ctx, q, limit)
	}
	return s.listUsageTotals(ctx, q, limit)
}

// listUsageTotals answers a UsageGroupByKey read: one row per
// (kind, key), summed across the range, with an empty Day.
func (s *Store) listUsageTotals(ctx context.Context, q atlas.UsageQuery, limit int) ([]atlas.UsageCount, error) {
	type row struct {
		kind, key string
		count     int64
	}
	var rows []row

	if q.Kind == "" {
		got, err := s.q.ListUsageTotals(ctx, sqlitegen.ListUsageTotalsParams{
			FromDay:  q.From,
			ToDay:    q.To,
			RowLimit: int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("sqlite.ListUsage: totals: %w", err)
		}
		for _, r := range got {
			rows = append(rows, row{kind: r.Kind, key: r.BucketKey, count: r.Total})
		}
	} else {
		got, err := s.q.ListUsageTotalsByKind(ctx, sqlitegen.ListUsageTotalsByKindParams{
			FromDay:  q.From,
			ToDay:    q.To,
			Kind:     q.Kind,
			RowLimit: int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("sqlite.ListUsage: totals by kind: %w", err)
		}
		for _, r := range got {
			rows = append(rows, row{kind: r.Kind, key: r.BucketKey, count: r.Total})
		}
	}

	out := make([]atlas.UsageCount, 0, len(rows))
	for _, r := range rows {
		// Day stays empty: the row spans the whole requested range.
		out = append(out, atlas.UsageCount{Kind: r.kind, Key: r.key, Count: int(r.count)})
	}
	return out, nil
}

// listUsageByDay answers a UsageGroupByDay read: the stored rows,
// unaggregated.
func (s *Store) listUsageByDay(ctx context.Context, q atlas.UsageQuery, limit int) ([]atlas.UsageCount, error) {
	var rows []sqlitegen.UsageDaily
	var err error
	if q.Kind == "" {
		rows, err = s.q.ListUsage(ctx, sqlitegen.ListUsageParams{
			FromDay:  q.From,
			ToDay:    q.To,
			RowLimit: int64(limit),
		})
	} else {
		rows, err = s.q.ListUsageByKind(ctx, sqlitegen.ListUsageByKindParams{
			FromDay:  q.From,
			ToDay:    q.To,
			Kind:     q.Kind,
			RowLimit: int64(limit),
		})
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListUsage: by day: %w", err)
	}

	out := make([]atlas.UsageCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, atlas.UsageCount{
			Day:   r.Day,
			Kind:  r.Kind,
			Key:   r.BucketKey,
			Count: int(r.Count),
		})
	}
	return out, nil
}

// PruneUsage deletes buckets strictly older than cutoffDay
// ('YYYY-MM-DD'), keeping the table bounded. A blank cutoff is a no-op.
func (s *Store) PruneUsage(ctx context.Context, cutoffDay string) error {
	if cutoffDay == "" {
		return nil
	}
	if err := s.q.PruneUsage(ctx, cutoffDay); err != nil {
		return fmt.Errorf("sqlite.PruneUsage: %w", err)
	}
	return nil
}
