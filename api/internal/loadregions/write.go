package loadregions

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
)

// Summary is the per-run report returned by LoadFile.
type Summary struct {
	Regions     int
	ParentEdges int
}

// LoadFile parses a TOML file at path, validates it (structural +
// cycle check), and writes the resulting regions + region_parents
// rows inside a single transaction. Country is stamped on every
// region row.
func LoadFile(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, path, country string) (Summary, error) {
	f, err := openAndParse(path)
	if err != nil {
		return Summary{}, err
	}
	if err := DetectCycles(f); err != nil {
		return Summary{}, err
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Summary{}, fmt.Errorf("loadregions: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := gen.New(tx)
	rid := map[string]int64{}
	summary := Summary{}
	for _, r := range f.Regions {
		id, err := q.UpsertRegion(ctx, gen.UpsertRegionParams{
			Country:      country,
			Kind:         r.Kind,
			Name:         r.Name,
			Slug:         r.Slug,
			ScopeTier:    r.ScopeTier,
			SortPriority: int32(r.SortPriority),
		})
		if err != nil {
			return Summary{}, fmt.Errorf("loadregions: upsert %q: %w", r.Slug, err)
		}
		rid[r.Slug] = id
		summary.Regions++
	}
	for _, r := range f.Regions {
		if err := q.DeleteRegionParents(ctx, rid[r.Slug]); err != nil {
			return Summary{}, fmt.Errorf("loadregions: clear parents for %q: %w", r.Slug, err)
		}
		for _, ps := range r.Parents {
			pid, ok := rid[ps]
			if !ok {
				return Summary{}, fmt.Errorf("loadregions: parent %q not found while wiring %q", ps, r.Slug)
			}
			if err := q.InsertRegionParent(ctx, gen.InsertRegionParentParams{
				RegionID:       rid[r.Slug],
				ParentRegionID: pid,
			}); err != nil {
				return Summary{}, fmt.Errorf("loadregions: insert edge %q→%q: %w", r.Slug, ps, err)
			}
			summary.ParentEdges++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Summary{}, fmt.Errorf("loadregions: commit: %w", err)
	}
	if logger != nil {
		logger.Info("loadregions: complete",
			"country", country,
			"regions", summary.Regions,
			"parent_edges", summary.ParentEdges,
		)
	}
	return summary, nil
}

func openAndParse(path string) (File, error) {
	r, err := openFile(path)
	if err != nil {
		return File{}, err
	}
	defer r.Close()
	return Parse(r)
}

func openFile(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loadregions: open %s: %w", path, err)
	}
	return f, nil
}
