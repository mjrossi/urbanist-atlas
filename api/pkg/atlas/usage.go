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
	// Day is the UTC calendar day, formatted 'YYYY-MM-DD'.
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

// UsageReader is the read seam behind GET /api/v1/admin/usage,
// satisfied by *sqlite.Store. from and to are inclusive 'YYYY-MM-DD'
// bounds; an empty kind means all kinds.
type UsageReader interface {
	ListUsage(ctx context.Context, from, to, kind string, limit int) ([]UsageCount, error)
}
