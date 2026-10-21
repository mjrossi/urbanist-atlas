package sqlite

import (
	"context"
	"fmt"

	sqlitegen "github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// UpsertUsageCounts adds a batch of daily usage deltas, in one
// transaction, to any existing (day, kind, key) rows. All or nothing,
// because a partial flush would double-count on retry. An empty batch
// is a no-op.
func (s *Store) UpsertUsageCounts(ctx context.Context, counts []atlas.UsageCount) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite.UpsertUsageCounts: begin: %w", err)
	}
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

// ListUsage returns buckets between the inclusive day bounds, highest
// count first; an empty Kind means every kind. atlas.UsageGroupByKey
// (the default) sums each bucket over the range, leaving Day empty;
// atlas.UsageGroupByDay returns the stored per-day rows. The grouping
// matters: a capped per-day read ranks by single-day count, so a slug
// with steady traffic would sort below one that spiked once. Each
// grouping needs two queries because sqlc's SQLite parser rejects a
// named argument used twice (queries/usage_daily.sql).
func (s *Store) ListUsage(ctx context.Context, q atlas.UsageQuery) ([]atlas.UsageCount, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = atlas.DefaultUsageLimit
	}
	if limit > atlas.MaxUsageLimit {
		limit = atlas.MaxUsageLimit
	}

	if q.GroupBy == atlas.UsageGroupByDay {
		return s.listUsageByDay(ctx, q, limit)
	}
	return s.listUsageTotals(ctx, q, limit)
}

// listUsageTotals sums each (kind, key) across the range.
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
		out = append(out, atlas.UsageCount{Kind: r.kind, Key: r.key, Count: int(r.count)})
	}
	return out, nil
}

// listUsageByDay returns the stored rows unaggregated.
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

// PruneUsage deletes buckets older than cutoffDay ('YYYY-MM-DD'). A
// blank cutoff is a no-op.
func (s *Store) PruneUsage(ctx context.Context, cutoffDay string) error {
	if cutoffDay == "" {
		return nil
	}
	if err := s.q.PruneUsage(ctx, cutoffDay); err != nil {
		return fmt.Errorf("sqlite.PruneUsage: %w", err)
	}
	return nil
}
