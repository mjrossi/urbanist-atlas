package etl

import (
	"encoding/csv"
	"io"
	"slices"
	"sort"
)

// PostalAnchor is the smallest-anchor decision for one postal code
// after a country's crosswalk runs. PostalCode is the unit the country
// keys postal lookups on (US ZIP/ZCTA, CA FSA, …); AnchorSlug is the
// region slug the postal_codes row points at; Reason is a
// debug-friendly label of which fallback won.
type PostalAnchor struct {
	PostalCode string
	AnchorSlug string
	Reason     string
}

// WritePostalCSV emits a postal_codes_<cc>.csv file deterministically:
// header postal_code,country,leaf_region_slug, then one row per anchor
// sorted by postal code ASC, LF line endings, trailing newline. The
// input slice is cloned before sorting so callers can keep using their
// copy. Per-country merge logic (e.g. the US ZCTA+HUD dedup) happens in
// the country plan, which hands the merged slice here.
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
