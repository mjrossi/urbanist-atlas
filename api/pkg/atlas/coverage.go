package atlas

import (
	"context"
	"time"
)

// CoverageGap is one sampled empty-result lookup or search: where the
// directory has no coverage yet. It is the one place raw lookup or
// search input is persisted; see
// docs/superpowers/specs/2026-06-08-observability-design.md D4.
//
//   - Kind is "lookup" or "search".
//   - Country is the lookup country ("" for searches).
//   - Input is the normalized postal code (lookups) or the search query.
type CoverageGap struct {
	Kind      string
	Country   string
	Input     string
	CreatedAt time.Time
}

// CoverageGapReader is the read seam behind GET
// /api/v1/admin/coverage-gaps. limit <= 0 selects a default, and
// implementations cap it.
type CoverageGapReader interface {
	ListCoverageGaps(ctx context.Context, limit int) ([]CoverageGap, error)
}
