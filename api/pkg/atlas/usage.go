package atlas

import "context"

// UsageCount is one daily aggregate bucket — the durable record behind
// the monthly usage digest. It serves as both the write unit (the
// recorder flushes a slice of deltas) and the read unit (the admin
// endpoint returns accumulated totals), because the shape is identical
// and a second near-duplicate type would only invite drift.
//
// Key holds a public content identifier (region or org slug) or a
// bounded enum value — never raw user input. Raw postal codes and
// search queries are persisted only in coverage_gaps, sampled. See
// docs/superpowers/specs/2026-09-06-usage-digest-design.md §D3.
type UsageCount struct {
	// Day is the UTC calendar day, formatted 'YYYY-MM-DD'. It is empty
	// on a UsageGroupByKey read, where the row is a total spanning the
	// whole requested range rather than a single day.
	Day string `json:"day"`
	// Kind is the bucket family — see the Kind* constants in
	// internal/usage.
	Kind string `json:"kind"`
	// Key is the slug or enum value within Kind.
	Key string `json:"key"`
	// Count is the number of events in this bucket. On write it is a
	// delta to accumulate; on read it is the running total.
	Count int `json:"count"`
}

// UsageGroupBy selects the granularity of a usage read.
type UsageGroupBy string

const (
	// UsageGroupByKey sums each (kind, key) bucket across the whole
	// requested range, returning one row per bucket with an empty Day.
	// This is the default and what the monthly digest wants: a month of
	// per-day rows both overruns any sane limit and ranks by single-day
	// count, so a slug with steady daily traffic loses to one that
	// spiked once.
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

// UsageReader is the read seam behind GET /api/v1/admin/usage,
// satisfied by *sqlite.Store.
type UsageReader interface {
	ListUsage(ctx context.Context, q UsageQuery) ([]UsageCount, error)
}
