package seedfiles

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// PostalRow is one parsed CSV record from postal_codes_<cc>.csv.
// Three columns: postal_code,country,leaf_region_slug. Country is
// redundant with the caller-supplied country code but kept in the
// CSV so cross-country rows are caught at parse time.
type PostalRow struct {
	PostalCode     string
	Country        atlas.Country
	LeafRegionSlug string
}

var postalHeader = []string{"postal_code", "country", "leaf_region_slug"}

// ParsePostal reads every row from r and returns them, validated.
func ParsePostal(r io.Reader, country atlas.Country) ([]PostalRow, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = len(postalHeader)
	reader.TrimLeadingSpace = true

	got, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("seedfiles: read postal header: %w", err)
	}
	for i, h := range postalHeader {
		if i >= len(got) || !strings.EqualFold(strings.TrimSpace(got[i]), h) {
			return nil, fmt.Errorf("seedfiles: unexpected postal header %v (want %v)", got, postalHeader)
		}
	}

	var out []PostalRow
	line := 1
	for {
		line++
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("seedfiles: postal line %d: %w", line, err)
		}
		row, err := parsePostalRecord(rec, country)
		if err != nil {
			return nil, fmt.Errorf("seedfiles: postal line %d: %w", line, err)
		}
		out = append(out, row)
	}
	return out, nil
}

func parsePostalRecord(rec []string, expectedCountry atlas.Country) (PostalRow, error) {
	get := func(i int) string { return strings.TrimSpace(rec[i]) }
	rowCountry := atlas.Country(get(1))
	if rowCountry != expectedCountry {
		return PostalRow{}, fmt.Errorf("country %q does not match expected %q", rowCountry, expectedCountry)
	}
	postal := atlas.NormalizePostalCode(rowCountry, get(0))
	if postal == "" {
		return PostalRow{}, errors.New("empty postal_code")
	}
	if err := atlas.ValidatePostalCode(rowCountry, postal); err != nil {
		return PostalRow{}, err
	}
	slug := get(2)
	if slug == "" {
		return PostalRow{}, errors.New("empty leaf_region_slug")
	}
	return PostalRow{PostalCode: postal, Country: rowCountry, LeafRegionSlug: slug}, nil
}
