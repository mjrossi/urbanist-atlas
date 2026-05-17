// Package loadpostal ingests postal-code crosswalks (US ZIP and
// Canadian FSA) into the regions + postal_codes tables. It is the
// driver behind the `loadpostal` subcommand.
//
// The CSV format is uniform across countries: ten columns describing a
// single postal code and up to four tiers of geography it belongs to.
// Real-world Census / StatsCan files don't ship in this shape; the
// expectation is that a separate ETL step (a one-off script, a
// notebook, an LLM) normalizes the source data into this format before
// the binary touches it. Bundled fixtures under api/seed/ demonstrate
// the schema.
//
// The package is library-first: a caller passes a *pgxpool.Pool and a
// path; everything inside one transaction. The cmd/server wrapper is a
// ~10-line cli.ActionFunc.
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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Row is one parsed CSV record: a single postal code and the regions
// it belongs to at each tier. Any tier whose Slug is empty is
// considered absent for this row.
type Row struct {
	PostalCode string
	Country    atlas.Country
	City       RegionRef
	County     RegionRef
	Metro      RegionRef
	State      RegionRef // also holds province for CA
}

// RegionRef is the (name, slug) pair for one region tier on one row.
// An empty Slug means "no region at this tier for this postal code".
type RegionRef struct {
	Name string
	Slug string
}

// expected CSV header in canonical order.
var header = []string{
	"postal_code", "country",
	"city_name", "city_slug",
	"county_name", "county_slug",
	"metro_name", "metro_slug",
	"state_name", "state_slug",
}

// LoadFile parses a CSV at path and upserts its contents inside a
// single transaction. On any error the transaction rolls back so the
// table state is unchanged.
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

	summary, err := writeRows(ctx, tx, logger, rows)
	if err != nil {
		return Summary{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Summary{}, fmt.Errorf("loadpostal: commit: %w", err)
	}
	return summary, nil
}

// Summary is the per-run report returned by LoadFile.
type Summary struct {
	RowsParsed     int
	PostalCodes    int
	RegionsTouched int
}

// ParseCSV reads every row from r and returns them, validated.
// Header must match the canonical column order exactly so we never
// confuse columns when a source schema drifts.
func ParseCSV(r io.Reader, country atlas.Country) ([]Row, error) {
	if country != atlas.CountryUS && country != atlas.CountryCA {
		return nil, fmt.Errorf("loadpostal: unsupported country %q (want US or CA)", country)
	}
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = len(header)
	reader.TrimLeadingSpace = true

	got, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("loadpostal: read header: %w", err)
	}
	if !headerEquals(got, header) {
		return nil, fmt.Errorf("loadpostal: unexpected header %v (want %v)", got, header)
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
	postal := normalizePostalCode(rowCountry, get(0))
	if postal == "" {
		return Row{}, errors.New("empty postal_code")
	}
	if err := validatePostalCode(rowCountry, postal); err != nil {
		return Row{}, err
	}

	row := Row{
		PostalCode: postal,
		Country:    rowCountry,
		City:       RegionRef{Name: get(2), Slug: get(3)},
		County:     RegionRef{Name: get(4), Slug: get(5)},
		Metro:      RegionRef{Name: get(6), Slug: get(7)},
		State:      RegionRef{Name: get(8), Slug: get(9)},
	}
	for _, ref := range []RegionRef{row.City, row.County, row.Metro, row.State} {
		if (ref.Name == "") != (ref.Slug == "") {
			return Row{}, fmt.Errorf("region pair (name=%q slug=%q): both or neither must be set", ref.Name, ref.Slug)
		}
	}
	if row.State.Slug == "" {
		return Row{}, errors.New("state/province (state_slug, state_name) is required")
	}
	return row, nil
}

// writeRows applies the parsed rows to the DB inside an open
// transaction. Region IDs are cached per slug so we don't issue one
// upsert per row per tier — typical crosswalk files have a handful of
// distinct metros across thousands of postal codes.
func writeRows(ctx context.Context, tx pgx.Tx, logger *slog.Logger, rows []Row) (Summary, error) {
	q := gen.New(tx)
	regionIDs := map[string]int64{}
	summary := Summary{RowsParsed: len(rows)}

	upsertRegion := func(ref RegionRef, kind atlas.RegionKind, country atlas.Country) (pgtype.Int8, error) {
		if ref.Slug == "" {
			return pgtype.Int8{}, nil
		}
		if id, ok := regionIDs[ref.Slug]; ok {
			return pgtype.Int8{Int64: id, Valid: true}, nil
		}
		id, err := q.UpsertRegion(ctx, gen.UpsertRegionParams{
			Kind:      string(kind),
			Name:      ref.Name,
			Slug:      ref.Slug,
			Country:   string(country),
			ScopeTier: string(scopeTierFor(kind)),
		})
		if err != nil {
			return pgtype.Int8{}, fmt.Errorf("upsert region %q: %w", ref.Slug, err)
		}
		regionIDs[ref.Slug] = id
		summary.RegionsTouched++
		return pgtype.Int8{Int64: id, Valid: true}, nil
	}

	for i, row := range rows {
		countyKind := atlas.RegionCounty
		stateKind := atlas.RegionState
		if row.Country == atlas.CountryCA {
			stateKind = atlas.RegionProvince
		}

		cityID, err := upsertRegion(row.City, atlas.RegionCity, row.Country)
		if err != nil {
			return Summary{}, err
		}
		countyID, err := upsertRegion(row.County, countyKind, row.Country)
		if err != nil {
			return Summary{}, err
		}
		metroID, err := upsertRegion(row.Metro, atlas.RegionMetro, row.Country)
		if err != nil {
			return Summary{}, err
		}
		stateID, err := upsertRegion(row.State, stateKind, row.Country)
		if err != nil {
			return Summary{}, err
		}

		if err := q.UpsertPostalCode(ctx, gen.UpsertPostalCodeParams{
			PostalCode:     row.PostalCode,
			Country:        string(row.Country),
			CityRegionID:   cityID,
			CountyRegionID: countyID,
			MetroRegionID:  metroID,
			StateRegionID:  stateID,
		}); err != nil {
			return Summary{}, fmt.Errorf("upsert postal_code %q: %w", row.PostalCode, err)
		}
		summary.PostalCodes++

		if logger != nil && (i+1)%500 == 0 {
			logger.Info("loadpostal progress", "rows", i+1, "total", len(rows))
		}
	}
	return summary, nil
}

// scopeTierFor maps a region kind to the scope_tier the lookup
// algorithm uses for bucketing. Cities and counties are "local"; the
// broader containers (metro, state, province, multi-state) are
// "regional". Country never appears in v1 lookup results, but it maps
// to "regional" for forward compatibility.
func scopeTierFor(k atlas.RegionKind) atlas.ScopeTier {
	switch k {
	case atlas.RegionCity, atlas.RegionCounty:
		return atlas.ScopeLocal
	default:
		return atlas.ScopeRegional
	}
}

// normalizePostalCode mirrors pkg/atlas's rules: uppercase, strip
// whitespace, truncate Canadian codes to the first three chars (FSA).
// Duplicated rather than imported to avoid pulling the memstore into
// the loadpostal package; the rule is short and stable.
func normalizePostalCode(country atlas.Country, code string) string {
	c := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if country == atlas.CountryCA && len(c) > 3 {
		c = c[:3]
	}
	return c
}

func validatePostalCode(country atlas.Country, code string) error {
	switch country {
	case atlas.CountryUS:
		if len(code) != 5 {
			return fmt.Errorf("US ZIP %q: want 5 digits", code)
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				return fmt.Errorf("US ZIP %q: non-digit character", code)
			}
		}
	case atlas.CountryCA:
		if len(code) != 3 {
			return fmt.Errorf("CA FSA %q: want 3 chars", code)
		}
		// Canada FSA: letter, digit, letter.
		if !isLetter(code[0]) || !isDigit(code[1]) || !isLetter(code[2]) {
			return fmt.Errorf("CA FSA %q: must be letter-digit-letter", code)
		}
	}
	return nil
}

func isLetter(b byte) bool { return (b >= 'A' && b <= 'Z') }
func isDigit(b byte) bool  { return (b >= '0' && b <= '9') }

func headerEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), b[i]) {
			return false
		}
	}
	return true
}
