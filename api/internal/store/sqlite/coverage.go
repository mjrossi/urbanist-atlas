package sqlite

import (
	"context"
	"fmt"

	sqlitegen "github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// RecordCoverageGap appends one sampled empty-result lookup or search.
// kind is "lookup" or "search"; country is "" for searches.
func (s *Store) RecordCoverageGap(ctx context.Context, kind, country, input string) error {
	if err := s.q.InsertCoverageGap(ctx, sqlitegen.InsertCoverageGapParams{
		Kind:      kind,
		Country:   country,
		Input:     input,
		CreatedAt: s.nowFunc().Format(sqliteTimeFormat),
	}); err != nil {
		return fmt.Errorf("sqlite.RecordCoverageGap: %w", err)
	}
	return nil
}

// PruneCoverageGaps deletes all but the newest maxRows rows. maxRows <= 0
// is a no-op.
func (s *Store) PruneCoverageGaps(ctx context.Context, maxRows int) error {
	if maxRows <= 0 {
		return nil
	}
	if err := s.q.PruneCoverageGaps(ctx, int64(maxRows)); err != nil {
		return fmt.Errorf("sqlite.PruneCoverageGaps: %w", err)
	}
	return nil
}

// ListCoverageGaps returns the newest coverage-gap rows first, limit
// defaulting to 50 and capped at 200.
func (s *Store) ListCoverageGaps(ctx context.Context, limit int) ([]atlas.CoverageGap, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.q.ListCoverageGaps(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListCoverageGaps: %w", err)
	}
	out := make([]atlas.CoverageGap, 0, len(rows))
	for _, r := range rows {
		createdAt, err := parseSQLiteTime(r.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("sqlite: coverage_gap %d: parse created_at: %w", r.ID, err)
		}
		out = append(out, atlas.CoverageGap{
			Kind:      r.Kind,
			Country:   r.Country,
			Input:     r.Input,
			CreatedAt: createdAt,
		})
	}
	return out, nil
}
