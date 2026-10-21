package etl

import (
	"encoding/csv"
	"io"
	"slices"
	"sort"
)

// PostalAnchor is the smallest-anchor decision for one postal code (US
// ZIP, CA FSA). Reason labels which fallback tier won, for the summary
// log.
type PostalAnchor struct {
	PostalCode string
	AnchorSlug string
	Reason     string
}

// WritePostalCSV writes a postal_codes_<cc>.csv: header, then one row per
// anchor sorted by postal code. It sorts a copy, leaving anchors as is.
func WritePostalCSV(w io.Writer, country string, anchors []PostalAnchor) error {
	sorted := slices.Clone(anchors)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].PostalCode < sorted[j].PostalCode })

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"postal_code", "country", "leaf_region_slug"}); err != nil {
		return err
	}
	for _, a := range sorted {
		if err := cw.Write([]string{a.PostalCode, country, a.AnchorSlug}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
