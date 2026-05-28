// Package loadpostal parses postal-code → leaf-region CSV files.
// The CSV format is three columns:
//
//	postal_code,country,leaf_region_slug
//
// One row per postal code. Country is redundant with the caller-
// supplied country and is kept in the CSV so the file is self-
// documenting and so cross-country rows are caught at parse time.
//
// Real-world Census / StatsCan / Royal Mail files don't ship in this
// shape; the expectation is that an out-of-band ETL step reshapes them
// before the binary touches them. Bundled fixtures under api/seed/
// demonstrate the schema.
package loadpostal

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Row is one parsed CSV record.
type Row struct {
	PostalCode     string
	Country        atlas.Country
	LeafRegionSlug string
}

var header = []string{"postal_code", "country", "leaf_region_slug"}

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
