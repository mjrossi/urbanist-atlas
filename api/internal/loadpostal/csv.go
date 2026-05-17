// Package loadpostal ingests postal-code → leaf-region mappings into
// the postal_codes table. The CSV format is three columns:
//
//	postal_code,country,leaf_region_slug
//
// One row per postal code. Country is redundant with the --country
// CLI flag but kept in the CSV so the file is self-documenting and so
// cross-country rows are caught at parse time.
//
// Real-world Census / StatsCan / Royal Mail files don't ship in this
// shape; the expectation is that an out-of-band ETL step reshapes them
// before the binary touches them. Bundled fixtures under api/seed/
// demonstrate the schema.
package loadpostal

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Row is one parsed CSV record.
type Row struct {
	PostalCode     string
	Country        atlas.Country
	LeafRegionSlug string
}

// Summary is the per-run report returned by LoadFile.
type Summary struct {
	PostalCodes int
}

var header = []string{"postal_code", "country", "leaf_region_slug"}

// LoadFile parses a CSV at path and upserts its contents inside a
// single transaction. Unknown slugs are a hard error — silently
// skipping would leave postal codes pointing at nothing.
func LoadFile(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, path string, country atlas.Country) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("loadpostal: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	rows, err := ParseCSV(f, country)
	if err != nil {
		return Summary{}, err
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Summary{}, fmt.Errorf("loadpostal: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := gen.New(tx)
	slugCache := map[string]int64{}
	summary := Summary{}
	for _, row := range rows {
		leafID, ok := slugCache[row.LeafRegionSlug]
		if !ok {
			id, err := q.RegionIDBySlug(ctx, row.LeafRegionSlug)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return Summary{}, fmt.Errorf("loadpostal: postal_code %s/%s: leaf_region_slug %q not found (run loadregions first?)", row.Country, row.PostalCode, row.LeafRegionSlug)
				}
				return Summary{}, fmt.Errorf("loadpostal: resolve slug %q: %w", row.LeafRegionSlug, err)
			}
			slugCache[row.LeafRegionSlug] = id
			leafID = id
		}
		if err := q.UpsertPostalCode(ctx, gen.UpsertPostalCodeParams{
			Country:      string(row.Country),
			PostalCode:   row.PostalCode,
			LeafRegionID: leafID,
		}); err != nil {
			return Summary{}, fmt.Errorf("loadpostal: upsert %s/%s: %w", row.Country, row.PostalCode, err)
		}
		summary.PostalCodes++
		if logger != nil && summary.PostalCodes%500 == 0 {
			logger.Info("loadpostal: progress", "rows", summary.PostalCodes, "total", len(rows))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Summary{}, fmt.Errorf("loadpostal: commit: %w", err)
	}
	if logger != nil {
		logger.Info("loadpostal: complete",
			"country", country,
			"postal_codes", summary.PostalCodes,
			"distinct_leaf_slugs", len(slugCache),
		)
	}
	return summary, nil
}

// ParseCSV reads every row from r and returns them, validated.
func ParseCSV(r io.Reader, country atlas.Country) ([]Row, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = len(header)
	reader.TrimLeadingSpace = true

	got, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("loadpostal: read header: %w", err)
	}
	for i, h := range header {
		if i >= len(got) || !strings.EqualFold(strings.TrimSpace(got[i]), h) {
			return nil, fmt.Errorf("loadpostal: unexpected header %v (want %v)", got, header)
		}
	}

	var out []Row
	line := 1
	for {
		line++
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("loadpostal: line %d: %w", line, err)
		}
		row, err := parseRecord(rec, country)
		if err != nil {
			return nil, fmt.Errorf("loadpostal: line %d: %w", line, err)
		}
		out = append(out, row)
	}
	return out, nil
}

func parseRecord(rec []string, expectedCountry atlas.Country) (Row, error) {
	get := func(i int) string { return strings.TrimSpace(rec[i]) }
	rowCountry := atlas.Country(get(1))
	if rowCountry != expectedCountry {
		return Row{}, fmt.Errorf("country %q does not match --country %q", rowCountry, expectedCountry)
	}
	postal := atlas.NormalizePostalCode(rowCountry, get(0))
	if postal == "" {
		return Row{}, errors.New("empty postal_code")
	}
	if err := atlas.ValidatePostalCode(rowCountry, postal); err != nil {
		return Row{}, err
	}
	slug := get(2)
	if slug == "" {
		return Row{}, errors.New("empty leaf_region_slug")
	}
	return Row{PostalCode: postal, Country: rowCountry, LeafRegionSlug: slug}, nil
}
