package atlas

import "context"

// UsageCount is one daily aggregate bucket. It is both the write unit (a
// delta) and the read unit (a running total), since a second
// near-identical type would only invite drift.
//
// Key is a public slug or a bounded enum value, never raw user input;
// see docs/superpowers/specs/2026-09-06-usage-digest-design.md §D3.
type UsageCount struct {
	// Day is the UTC day as 'YYYY-MM-DD', empty on a UsageGroupByKey read.
	Day string `json:"day"`
	// Kind is the bucket family (the Kind* constants in internal/usage).
	Kind string `json:"kind"`
	// Key is the slug or enum value within Kind.
	Key string `json:"key"`
	// Count is a delta to accumulate on write and a running total on read.
	Count int `json:"count"`
}

// MaxUsageLimit and DefaultUsageLimit bound a usage read. The handler
// and the store share them so their bounds can't drift; openapi.yaml
// repeats them on the limit param, so change the two together.
const (
	MaxUsageLimit     = 1000
	DefaultUsageLimit = 100
)

// UsageGroupBy selects the granularity of a usage read.
type UsageGroupBy string

const (
	// UsageGroupByKey sums each (kind, key) bucket across the range, one
	// row per bucket with an empty Day. It is the default because
	// ranking per-day rows lets a one-day spike outrank steady traffic.
	UsageGroupByKey UsageGroupBy = "key"
	// UsageGroupByDay returns the stored per-day rows unaggregated, for
	// charting a daily series within a range.
	UsageGroupByDay UsageGroupBy = "day"
)

// UsageQuery describes one read of the rollup table. From and To are
// inclusive 'YYYY-MM-DD' bounds; an empty Kind means all kinds; an empty
// GroupBy means UsageGroupByKey.
type UsageQuery struct {
	From    string
	To      string
	Kind    string
	GroupBy UsageGroupBy
	Limit   int
}

// UsageReader is the read seam behind GET /api/v1/admin/usage.
type UsageReader interface {
	ListUsage(ctx context.Context, q UsageQuery) ([]UsageCount, error)
}
