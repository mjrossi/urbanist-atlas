package atlas

import (
	"context"
	"time"
)

// CoverageGap is one sampled empty-result lookup or search — the
// editorial "which input returned nothing?" signal that tells us where
// the directory has no coverage yet. It is the single place raw
// user-typed input is persisted (the privacy bar's "sampled empties"
// exception); non-empty traffic stays aggregate-only in Prometheus.
//
//   - Kind is "lookup" or "search".
//   - Country is the lookup country ("" for searches, which have no
//     country axis).
//   - Input is the normalized postal code (lookups) or the search query.
type CoverageGap struct {
	Kind      string
	Country   string
	Input     string
	CreatedAt time.Time
}

// CoverageGapReader is the read seam behind GET
// /api/v1/admin/coverage-gaps. limit <= 0 falls back to a default; the
// implementation caps it. Satisfied by the SQLite store.
type CoverageGapReader interface {
	ListCoverageGaps(ctx context.Context, limit int) ([]CoverageGap, error)
}
