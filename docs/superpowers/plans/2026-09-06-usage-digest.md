# Usage Rollups & Monthly Digest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Record daily per-slug usage rollups in the existing SQLite DB and push a monthly digest issue that merges Cloudflare pageviews, content popularity, coverage gaps, and Fly Prometheus health.

**Architecture:** A new `internal/usage` package buffers counts in memory and flushes them to a `usage_daily` table on an interval — mirroring `internal/coverage`'s fire-and-forget posture, but batched so SQLite sees one transaction per minute regardless of traffic. Handlers call a nil-safe `Increment`. A bearer-gated `GET /api/v1/admin/usage` reads the rollups back, and a monthly GitHub Actions workflow renders them into an issue.

**Tech Stack:** Go 1.x (stdlib + chi + urfave/cli), goose migrations, sqlc, modernc SQLite, oapi-codegen, openapi-typescript, GitHub Actions.

**Spec:** [`../specs/2026-09-06-usage-digest-design.md`](../specs/2026-09-06-usage-digest-design.md)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `api/migrations-sqlite/0003_usage_daily.sql` | Create: the `usage_daily` table + index |
| `api/internal/store/sqlite/queries/usage_daily.sql` | Create: sqlc queries (upsert, list, prune) |
| `api/internal/store/sqlite/usage.go` | Create: `Store` methods wrapping the generated queries |
| `api/internal/store/sqlite/usage_test.go` | Create: store-layer tests |
| `api/pkg/atlas/usage.go` | Create: `UsageCount` type + `UsageReader` seam |
| `api/internal/usage/recorder.go` | Create: buffered recorder (Increment/Flush/Run/Wait) |
| `api/internal/usage/recorder_test.go` | Create: recorder tests |
| `api/internal/httpapi/usage.go` | Create: admin read handler + wire adapter |
| `api/internal/httpapi/usage_test.go` | Create: handler tests |
| `api/internal/httpapi/router.go` | Modify: `Config.Usage`, `Config.UsageCounts`, route registration |
| `api/internal/httpapi/lookup.go` | Modify: record lookup rollups |
| `api/internal/httpapi/regions.go` | Modify: record region-view rollups |
| `api/internal/httpapi/orgs.go` | Modify: record org-view rollups |
| `api/openapi.yaml` | Modify: `/api/v1/admin/usage` path + `UsageCount` schema |
| `api/cmd/server/serve.go` | Modify: flags, construction, shutdown drain |
| `.github/workflows/usage-digest.yml` | Create: the monthly digest workflow |
| `docs/deploy.md` | Modify: §Usage digest runbook subsection |
| `CLAUDE.md` | Modify: add sqlc to the approved-dependency list |

**Ordering note:** Tasks 1–2 are pure data layer, 3 is the recorder, 4–6 wire it up, 7 is the workflow, 8 is docs. Tasks 1, 2, and 3 have no dependency on each other beyond types and could be parallelised, but the sequence below is the simplest to review.

---

### Task 1: `usage_daily` table + sqlc queries

**Files:**
- Create: `api/migrations-sqlite/0003_usage_daily.sql`
- Create: `api/internal/store/sqlite/queries/usage_daily.sql`

Note the column is `bucket_key`, not `key`: `KEY` is a keyword in several SQL dialects and reads ambiguously in `ON CONFLICT` clauses. The Go-side field stays `Key`.

- [ ] **Step 1: Write the migration**

```sql
-- +goose Up
-- +goose StatementBegin

-- usage_daily holds per-day aggregate counts of content popularity and
-- lookup outcomes — the durable record behind the monthly usage digest.
--
-- Aggregating by day is what makes per-slug popularity affordable here
-- when it was rejected as a Prometheus label (unbounded cardinality;
-- see the 2026-06-08 observability spec's D3). One row per
-- (day, kind, key) caps growth at roughly the slug count per day.
--
-- PRIVACY: bucket_key holds only public content identifiers (region and
-- org slugs) or bounded enum values — never raw postal codes or search
-- queries. Raw user input is persisted ONLY in coverage_gaps, sampled,
-- per the 2026-06-08 D4 privacy bar.
CREATE TABLE usage_daily (
    day        TEXT    NOT NULL,           -- 'YYYY-MM-DD', UTC
    kind       TEXT    NOT NULL CHECK (kind IN (
                   'region_view','org_view','lookup',
                   'lookup_tier','lookup_result','lookup_country')),
    bucket_key TEXT    NOT NULL,           -- region/org slug, tier, result, or country
    count      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, kind, bucket_key)
) WITHOUT ROWID;

-- Serves the digest's range scan (day BETWEEN ? AND ?, optional kind).
CREATE INDEX usage_daily_day_kind ON usage_daily(day, kind);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS usage_daily_day_kind;
DROP TABLE IF EXISTS usage_daily;
-- +goose StatementEnd
```

- [ ] **Step 2: Write the sqlc queries**

```sql
-- name: UpsertUsageCount :exec
-- Accumulates rather than replaces: the recorder flushes deltas
-- accrued since the last flush, so repeated flushes on the same day
-- must sum.
INSERT INTO usage_daily (day, kind, bucket_key, count)
VALUES (sqlc.arg(day), sqlc.arg(kind), sqlc.arg(bucket_key), sqlc.arg(count))
ON CONFLICT (day, kind, bucket_key)
DO UPDATE SET count = count + excluded.count;

-- name: ListUsage :many
-- Ordered by count DESC so a limited read returns the top buckets —
-- the digest's "top regions / top orgs" question — rather than an
-- arbitrary slice. Empty kind_filter means "all kinds".
SELECT day, kind, bucket_key, count
FROM usage_daily
WHERE day >= sqlc.arg(from_day)
  AND day <= sqlc.arg(to_day)
  AND (sqlc.arg(kind_filter) = '' OR kind = sqlc.arg(kind_filter))
ORDER BY count DESC, day DESC, kind ASC, bucket_key ASC
LIMIT sqlc.arg(row_limit);

-- name: PruneUsage :exec
-- Drops buckets older than the cutoff day. Called opportunistically
-- after a flush so the table stays bounded at ~400 days.
DELETE FROM usage_daily WHERE day < sqlc.arg(cutoff_day);
```

- [ ] **Step 3: Regenerate sqlc bindings**

Run: `just api-gen`
Expected: `api/internal/store/sqlite/gen/usage_daily.sql.go` appears, `gen/models.go` gains a `UsageDaily` struct. No errors.

- [ ] **Step 4: Verify the migration applies**

Run: `cd api && mise exec -- go test ./internal/store/sqlite/... -run TestStore_Ping -v`
Expected: PASS. (`newTestStore` calls `Migrate`, so a broken migration fails here.)

- [ ] **Step 5: Commit**

```bash
git add api/migrations-sqlite/0003_usage_daily.sql \
        api/internal/store/sqlite/queries/usage_daily.sql \
        api/internal/store/sqlite/gen/
git commit -m "feat(api): add usage_daily rollup table and sqlc queries"
```

---

### Task 2: `atlas.UsageCount` + store methods

**Files:**
- Create: `api/pkg/atlas/usage.go`
- Create: `api/internal/store/sqlite/usage.go`
- Create: `api/internal/store/sqlite/usage_test.go`

- [ ] **Step 1: Write the failing store test**

Create `api/internal/store/sqlite/usage_test.go`:

```go
package sqlite_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestStore_UpsertUsageCounts_Accumulates(t *testing.T) {
	// Two flushes on the same day must SUM, not replace — the recorder
	// flushes deltas, so a replace would silently discard traffic.
	s := newTestStore(t)
	ctx := context.Background()

	first := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 3},
		{Day: "2026-09-01", Kind: "org_view", Key: "active-trans", Count: 1},
	}
	if err := s.UpsertUsageCounts(ctx, first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	second := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 2},
	}
	if err := s.UpsertUsageCounts(ctx, second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := s.ListUsage(ctx, "2026-09-01", "2026-09-01", "region_view", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 5},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListUsage mismatch (-want +got):\n%s", diff)
	}
}

func TestStore_ListUsage_FiltersRangeAndKind(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed := []atlas.UsageCount{
		{Day: "2026-08-31", Kind: "region_view", Key: "out-of-range", Count: 99},
		{Day: "2026-09-01", Kind: "region_view", Key: "in-range", Count: 5},
		{Day: "2026-09-01", Kind: "org_view", Key: "wrong-kind", Count: 7},
		{Day: "2026-09-02", Kind: "region_view", Key: "also-in-range", Count: 9},
	}
	if err := s.UpsertUsageCounts(ctx, seed); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.ListUsage(ctx, "2026-09-01", "2026-09-02", "region_view", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Ordered by count DESC.
	want := []atlas.UsageCount{
		{Day: "2026-09-02", Kind: "region_view", Key: "also-in-range", Count: 9},
		{Day: "2026-09-01", Kind: "region_view", Key: "in-range", Count: 5},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("ListUsage mismatch (-want +got):\n%s", diff)
	}

	// Empty kind means "all kinds".
	all, err := s.ListUsage(ctx, "2026-09-01", "2026-09-02", "", 10)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("kind='' should return all 3 rows in range, got %d", len(all))
	}
}

func TestStore_PruneUsage_DropsOlderThanCutoff(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seed := []atlas.UsageCount{
		{Day: "2026-01-01", Kind: "region_view", Key: "ancient", Count: 1},
		{Day: "2026-09-01", Kind: "region_view", Key: "recent", Count: 1},
	}
	if err := s.UpsertUsageCounts(ctx, seed); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := s.PruneUsage(ctx, "2026-06-01"); err != nil {
		t.Fatalf("prune: %v", err)
	}

	got, err := s.ListUsage(ctx, "2020-01-01", "2030-01-01", "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].Key != "recent" {
		t.Errorf("prune should keep only 'recent', got %+v", got)
	}
}

func TestStore_UpsertUsageCounts_EmptyIsNoOp(t *testing.T) {
	// The recorder flushes unconditionally on its interval; an empty
	// buffer must not open a transaction or error.
	s := newTestStore(t)
	if err := s.UpsertUsageCounts(context.Background(), nil); err != nil {
		t.Fatalf("empty upsert should be a no-op, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd api && mise exec -- go test ./internal/store/sqlite/... -run TestStore_.*Usage -v`
Expected: FAIL — compile error, `undefined: atlas.UsageCount` and `s.UpsertUsageCounts undefined`.

- [ ] **Step 3: Write the atlas type and read seam**

Create `api/pkg/atlas/usage.go`:

```go
package atlas

import "context"

// UsageCount is one daily aggregate bucket — the durable record behind
// the monthly usage digest. It serves as both the write unit (the
// recorder flushes a slice of deltas) and the read unit (the admin
// endpoint returns accumulated totals), because the shape is identical
// and a second near-duplicate type would only invite drift.
//
// Key holds a public content identifier (region or org slug) or a
// bounded enum value — never raw user input. See the 2026-09-06
// usage-digest spec's D3.
type UsageCount struct {
	// Day is the UTC calendar day, formatted 'YYYY-MM-DD'.
	Day string `json:"day"`
	// Kind is the bucket family — see the UsageKind* constants in
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
```

- [ ] **Step 4: Write the store methods**

Create `api/internal/store/sqlite/usage.go`:

```go
package sqlite

import (
	"context"
	"fmt"

	sqlitegen "github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite/gen"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// maxUsageListLimit caps a single admin usage read. Generous because
// the digest legitimately wants a few hundred buckets per month, but
// bounded so a malformed query can't stream the whole table.
const maxUsageListLimit = 1000

// defaultUsageListLimit is applied when the caller passes limit <= 0.
const defaultUsageListLimit = 100

// UpsertUsageCounts accumulates a batch of daily usage deltas in one
// transaction. Counts SUM into any existing (day, kind, key) row, so
// repeated flushes within a day compose correctly.
//
// The whole batch is one transaction: a partial flush would double-count
// on retry, and the recorder has already cleared its buffer by the time
// this runs. An empty batch is a no-op — the recorder flushes on a timer
// whether or not traffic arrived.
func (s *Store) UpsertUsageCounts(ctx context.Context, counts []atlas.UsageCount) error {
	if len(counts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite.UpsertUsageCounts: begin: %w", err)
	}
	// Rollback is a no-op once Commit succeeds; safe to always defer.
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	for _, c := range counts {
		if err := qtx.UpsertUsageCount(ctx, sqlitegen.UpsertUsageCountParams{
			Day:       c.Day,
			Kind:      c.Kind,
			BucketKey: c.Key,
			Count:     int64(c.Count),
		}); err != nil {
			return fmt.Errorf("sqlite.UpsertUsageCounts: %s/%s/%s: %w", c.Day, c.Kind, c.Key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite.UpsertUsageCounts: commit: %w", err)
	}
	return nil
}

// ListUsage returns accumulated buckets between the inclusive day
// bounds, highest-count first. An empty kind returns every kind.
func (s *Store) ListUsage(ctx context.Context, from, to, kind string, limit int) ([]atlas.UsageCount, error) {
	if limit <= 0 {
		limit = defaultUsageListLimit
	}
	if limit > maxUsageListLimit {
		limit = maxUsageListLimit
	}
	rows, err := s.q.ListUsage(ctx, sqlitegen.ListUsageParams{
		FromDay:    from,
		ToDay:      to,
		KindFilter: kind,
		RowLimit:   int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite.ListUsage: %w", err)
	}
	out := make([]atlas.UsageCount, 0, len(rows))
	for _, r := range rows {
		out = append(out, atlas.UsageCount{
			Day:   r.Day,
			Kind:  r.Kind,
			Key:   r.BucketKey,
			Count: int(r.Count),
		})
	}
	return out, nil
}

// PruneUsage deletes buckets strictly older than cutoffDay
// ('YYYY-MM-DD'), keeping the table bounded. A blank cutoff is a no-op.
func (s *Store) PruneUsage(ctx context.Context, cutoffDay string) error {
	if cutoffDay == "" {
		return nil
	}
	if err := s.q.PruneUsage(ctx, cutoffDay); err != nil {
		return fmt.Errorf("sqlite.PruneUsage: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd api && mise exec -- go test ./internal/store/sqlite/... -run TestStore_.*Usage -v`
Expected: PASS — all four tests.

> If sqlc named a generated param field differently (e.g. `Count` vs `Count_`), fix the call site to match `gen/usage_daily.sql.go` rather than editing generated code.

- [ ] **Step 6: Commit**

```bash
git add api/pkg/atlas/usage.go api/internal/store/sqlite/usage.go api/internal/store/sqlite/usage_test.go
git commit -m "feat(api): add UsageCount type and SQLite rollup store methods"
```

---

### Task 3: The `internal/usage` recorder

**Files:**
- Create: `api/internal/usage/recorder.go`
- Create: `api/internal/usage/recorder_test.go`

- [ ] **Step 1: Write the failing recorder test**

Create `api/internal/usage/recorder_test.go`:

```go
package usage_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// fakeStore captures flushed batches so tests can assert on them
// without a real DB.
type fakeStore struct {
	mu      sync.Mutex
	batches [][]atlas.UsageCount
	pruned  []string
	err     error
}

func (f *fakeStore) UpsertUsageCounts(_ context.Context, counts []atlas.UsageCount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	cp := make([]atlas.UsageCount, len(counts))
	copy(cp, counts)
	f.batches = append(f.batches, cp)
	return nil
}

func (f *fakeStore) PruneUsage(_ context.Context, cutoffDay string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, cutoffDay)
	return nil
}

func (f *fakeStore) flushed() []atlas.UsageCount {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []atlas.UsageCount
	for _, b := range f.batches {
		out = append(out, b...)
	}
	return out
}

// fixedClock pins "now" so day bucketing is deterministic.
func fixedClock(day string) func() time.Time {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		panic(err)
	}
	return func() time.Time { return t }
}

func TestRecorder_AccumulatesAndFlushes(t *testing.T) {
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	r.Increment(usage.KindRegionView, "chicago-il")
	r.Increment(usage.KindRegionView, "chicago-il")
	r.Increment(usage.KindOrgView, "active-trans")

	if got := store.flushed(); len(got) != 0 {
		t.Fatalf("nothing should be written before Flush, got %+v", got)
	}
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	want := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: "org_view", Key: "active-trans", Count: 1},
		{Day: "2026-09-01", Kind: "region_view", Key: "chicago-il", Count: 2},
	}
	got := store.flushed()
	sortCounts := cmpopts.SortSlices(func(a, b atlas.UsageCount) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Key < b.Key
	})
	if diff := cmp.Diff(want, got, sortCounts); diff != "" {
		t.Errorf("flushed mismatch (-want +got):\n%s", diff)
	}
}

func TestRecorder_FlushClearsBuffer(t *testing.T) {
	// A second flush must not re-send counts already written, or the
	// accumulating upsert would double-count them.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	r.Increment(usage.KindRegionView, "chicago-il")
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("first flush: %v", err)
	}
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("second flush: %v", err)
	}

	if got := store.flushed(); len(got) != 1 || got[0].Count != 1 {
		t.Errorf("second flush should send nothing; total flushed = %+v", got)
	}
}

func TestRecorder_EmptyFlushIsNoOp(t *testing.T) {
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("empty flush: %v", err)
	}
	if len(store.batches) != 0 {
		t.Errorf("empty flush should not call the store, got %d batches", len(store.batches))
	}
}

func TestRecorder_NilIsNoOp(t *testing.T) {
	// Handlers call Increment unconditionally, mirroring *Metrics.
	var r *usage.Recorder
	r.Increment(usage.KindRegionView, "chicago-il")
	if err := r.Flush(context.Background()); err != nil {
		t.Errorf("nil Flush should be a no-op, got: %v", err)
	}
	if err := r.Wait(context.Background()); err != nil {
		t.Errorf("nil Wait should be a no-op, got: %v", err)
	}
}

func TestRecorder_IgnoresBlankKey(t *testing.T) {
	// A blank slug would create a meaningless bucket that pollutes the
	// top-N digest sections.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	r.Increment(usage.KindRegionView, "")
	r.Increment("", "chicago-il")
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := store.flushed(); len(got) != 0 {
		t.Errorf("blank kind/key should be dropped, got %+v", got)
	}
}

func TestRecorder_PrunesOnFlush(t *testing.T) {
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 30, nil)
	r.SetClock(fixedClock("2026-09-01"))

	r.Increment(usage.KindRegionView, "chicago-il")
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// 2026-09-01 minus 30 days.
	want := "2026-08-02"
	if len(store.pruned) != 1 || store.pruned[0] != want {
		t.Errorf("prune cutoff = %v, want [%s]", store.pruned, want)
	}
}

func TestRecorder_FlushErrorDropsBatch(t *testing.T) {
	// On a write error the batch is dropped, not restored: the store
	// upsert accumulates, so re-queuing a batch that may have partially
	// committed would double-count. Losing an interval is the cheaper
	// failure, and this pins that choice.
	store := &fakeStore{err: errors.New("db down")}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	r.Increment(usage.KindRegionView, "chicago-il")
	if err := r.Flush(context.Background()); err == nil {
		t.Fatal("Flush should surface the store error")
	}

	// Buffer was cleared despite the failure; a healthy retry sends nothing.
	store.mu.Lock()
	store.err = nil
	store.mu.Unlock()
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	if got := store.flushed(); len(got) != 0 {
		t.Errorf("failed batch must not be retried, got %+v", got)
	}
}

func TestRecorder_ConcurrentIncrements(t *testing.T) {
	// Run with -race. The buffer is shared across every request
	// goroutine, so this is the contract that matters most.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r.Increment(usage.KindRegionView, "chicago-il")
			}
		}()
	}
	wg.Wait()
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := store.flushed()
	if len(got) != 1 || got[0].Count != 1000 {
		t.Errorf("want a single bucket of 1000, got %+v", got)
	}
}

func TestRecorder_RunFlushesOnContextCancel(t *testing.T) {
	// Shutdown path: Run must drain what's buffered when its context
	// is cancelled, so the last interval's counts aren't lost.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))
	r.Increment(usage.KindRegionView, "chicago-il")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	if got := store.flushed(); len(got) != 1 {
		t.Errorf("Run should flush on shutdown, got %+v", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd api && mise exec -- go test ./internal/usage/... -v`
Expected: FAIL — `no Go files in .../internal/usage` or `undefined: usage.New`.

- [ ] **Step 3: Write the recorder**

Create `api/internal/usage/recorder.go`:

```go
// Package usage records daily aggregate usage counts — content
// popularity and lookup outcomes — as the durable record behind the
// monthly usage digest.
//
// It is the batched sibling of internal/coverage. Where coverage writes
// each sampled row in its own goroutine (rare events, one row each),
// usage counts arrive on every read request, so they accumulate in
// memory and flush on an interval. SQLite then sees one small
// transaction per interval regardless of traffic, and the usage path
// can never contend with the submission write path over the shared
// single connection.
//
// Buffering in RAM is safe because fly.toml pins the app to one machine
// with auto_stop_machines=false — the same assumption the in-process
// rate limiter and the GitHub PR worker queue already document. Worst
// case on an ungraceful kill is losing one interval of counts, which is
// acceptable for usage analytics and is deliberately NOT the tradeoff
// made for submissions.
package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Bucket kinds. These are the only values written to usage_daily.kind,
// and they must match the CHECK constraint in migration 0003.
const (
	KindRegionView    = "region_view"
	KindOrgView       = "org_view"
	KindLookup        = "lookup"
	KindLookupTier    = "lookup_tier"
	KindLookupResult  = "lookup_result"
	KindLookupCountry = "lookup_country"
)

// dayFormat is the usage_daily.day column format (UTC calendar day).
const dayFormat = "2006-01-02"

// flushTimeout bounds a single flush so a wedged DB can't stall the
// ticker loop or the shutdown drain.
const flushTimeout = 5 * time.Second

// CountStore is the persistence seam the Recorder writes through —
// satisfied by *sqlite.Store. Mirrors coverage.GapStore.
type CountStore interface {
	UpsertUsageCounts(ctx context.Context, counts []atlas.UsageCount) error
	PruneUsage(ctx context.Context, cutoffDay string) error
}

// bucketKey identifies one accumulator slot. Day is captured at
// Increment time, not at flush time, so an interval that spans midnight
// still attributes each event to the day it happened.
type bucketKey struct {
	day  string
	kind string
	key  string
}

// Recorder accumulates usage counts in memory and flushes them on an
// interval. A nil *Recorder is a valid no-op so handlers can call
// Increment unconditionally — the same convention as *httpapi.Metrics
// and *coverage.Recorder.
type Recorder struct {
	store    CountStore
	interval time.Duration
	keepDays int
	logger   *slog.Logger

	mu  sync.Mutex
	buf map[bucketKey]int
	now func() time.Time
}

// New builds a Recorder. interval is the flush cadence; keepDays bounds
// retention (<= 0 disables pruning).
func New(store CountStore, interval time.Duration, keepDays int, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	if interval <= 0 {
		interval = time.Minute
	}
	return &Recorder{
		store:    store,
		interval: interval,
		keepDays: keepDays,
		logger:   logger,
		buf:      make(map[bucketKey]int),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// SetClock overrides the time source. Tests pin it so day bucketing and
// prune cutoffs are deterministic; production uses time.Now().UTC().
// Mirrors coverage.Recorder.SetRNG.
func (r *Recorder) SetClock(fn func() time.Time) {
	if r != nil && fn != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.now = fn
	}
}

// Increment buckets one event. It returns immediately — nothing touches
// the database until the next flush. Blank kind or key is dropped: a
// blank slug would create a meaningless bucket that pollutes the digest's
// top-N sections.
func (r *Recorder) Increment(kind, key string) {
	if r == nil || r.store == nil || kind == "" || key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[bucketKey{day: r.now().Format(dayFormat), kind: kind, key: key}]++
}

// Run drives the flush ticker until ctx is cancelled, then performs one
// final flush so the last interval's counts survive shutdown. Intended
// to be called in its own goroutine.
func (r *Recorder) Run(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := r.Flush(ctx); err != nil {
				r.logger.Warn("usage: periodic flush failed", "err", err)
			}
		case <-ctx.Done():
			// Detached context: the caller's ctx is already cancelled,
			// so reusing it would abort the very write we need.
			flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
			defer cancel()
			if err := r.Flush(flushCtx); err != nil {
				r.logger.Warn("usage: final flush failed", "err", err)
			}
			return
		}
	}
}

// Flush writes the buffered deltas and clears the buffer. The buffer is
// snapshotted and cleared under the lock, then written outside it, so
// request goroutines never block on the database.
//
// On a write error the batch is dropped rather than restored: the store
// upsert accumulates, so re-queuing a batch that may have partially
// committed would double-count. Losing an interval of usage counts is
// the cheaper failure.
func (r *Recorder) Flush(ctx context.Context) error {
	if r == nil || r.store == nil {
		return nil
	}
	r.mu.Lock()
	if len(r.buf) == 0 {
		r.mu.Unlock()
		return nil
	}
	counts := make([]atlas.UsageCount, 0, len(r.buf))
	for k, n := range r.buf {
		counts = append(counts, atlas.UsageCount{Day: k.day, Kind: k.kind, Key: k.key, Count: n})
	}
	r.buf = make(map[bucketKey]int)
	cutoff := ""
	if r.keepDays > 0 {
		cutoff = r.now().AddDate(0, 0, -r.keepDays).Format(dayFormat)
	}
	r.mu.Unlock()

	if err := r.store.UpsertUsageCounts(ctx, counts); err != nil {
		return err
	}
	// Pruning is opportunistic: a failure here costs disk, not data.
	if cutoff != "" {
		if err := r.store.PruneUsage(ctx, cutoff); err != nil {
			r.logger.Warn("usage: prune failed", "err", err, "cutoff", cutoff)
		}
	}
	return nil
}

// Wait performs a final flush, bounded by ctx. Called during graceful
// shutdown alongside coverage.Recorder.Wait. Nil-safe.
func (r *Recorder) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	return r.Flush(ctx)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd api && mise exec -- go test -race ./internal/usage/... -v`
Expected: PASS — all nine tests, no race warnings.

- [ ] **Step 5: Commit**

```bash
git add api/internal/usage/
git commit -m "feat(api): add buffered usage recorder"
```

---

### Task 4: Record rollups from the read handlers

**Files:**
- Modify: `api/internal/httpapi/router.go` (add `Config.Usage`)
- Modify: `api/internal/httpapi/lookup.go:93-96`
- Modify: `api/internal/httpapi/regions.go:88-100`
- Modify: `api/internal/httpapi/orgs.go:26-42`

- [ ] **Step 1: Add the Usage field to Config**

In `api/internal/httpapi/router.go`, after the `Coverage` field, add:

```go
	// Usage, when non-nil, accumulates daily aggregate usage counts
	// (content popularity + lookup outcomes) for the monthly digest.
	// Nil disables recording; handlers call it unconditionally (it is
	// nil-safe). See internal/usage.
	Usage *usage.Recorder
```

Add the import `"github.com/mjrossi/urbanist-atlas/api/internal/usage"`.

- [ ] **Step 2: Thread the recorder into the three handler constructors**

In `router.go`, update the four call sites to pass `cfg.Usage`:

```go
r.Get("/orgs/{slug}", getOrgHandler(cfg.Store, logger, cfg.Metrics, cfg.Usage))
r.Get("/regions/{slug}", getRegionHandler(cfg.Store, logger, cfg.Metrics, cfg.Usage))
```

and for the lookup and region-search handlers, append `cfg.Usage` to their argument lists in the same way. Match each handler's existing registration line — do not reorder the other arguments.

- [ ] **Step 3: Record org views**

In `api/internal/httpapi/orgs.go`, change the signature to
`func getOrgHandler(store atlas.Store, logger *slog.Logger, m *Metrics, u *usage.Recorder) http.HandlerFunc`
and add a rollup call beside each existing `m.incOrgView` call:

```go
// not-found branch, beside m.incOrgView(false):
u.Increment(usage.KindOrgView, slug)
```

```go
// found branch, beside m.incOrgView(true):
u.Increment(usage.KindOrgView, org.Slug)
```

The found branch uses `org.Slug` rather than the raw path param so the
bucket is the canonical slug, not whatever casing the caller sent.

- [ ] **Step 4: Record region views and lookups**

In `api/internal/httpapi/regions.go`, change `getRegionHandler`'s signature to accept `u *usage.Recorder` and add beside each `m.incRegionView` call:

```go
// not-found branch:
u.Increment(usage.KindRegionView, slug)
```

```go
// found branch:
u.Increment(usage.KindRegionView, detail.Region.Slug)
```

> Confirm the field path for the resolved region on `atlas.RegionDetail` before writing this line — run
> `grep -n "type RegionDetail" -A 10 api/pkg/atlas/atlas.go`
> and use whatever field holds the canonical `Region`. If the detail
> exposes the slug directly, use that.

In `api/internal/httpapi/lookup.go`, change the handler signature to accept `u *usage.Recorder`. In the miss branches, beside `m.incLookup(string(country), "military")` and `m.incLookup(string(country), "miss")` respectively:

```go
u.Increment(usage.KindLookupResult, "military")
u.Increment(usage.KindLookupCountry, metricCountry(string(country)))
```

```go
u.Increment(usage.KindLookupResult, "miss")
u.Increment(usage.KindLookupCountry, metricCountry(string(country)))
```

In the hit branch, after `m.incLookupTier(string(country), tier)`:

```go
u.Increment(usage.KindLookupResult, "hit")
u.Increment(usage.KindLookupTier, tier)
u.Increment(usage.KindLookupCountry, metricCountry(string(country)))
// ResolvedAncestry is BFS-ordered from the matched leaf outward
// (see MemStore.bfsCollectIDs), so element 0 is the smallest curated
// anchor — the region slug this postal code resolved to. Recording
// the slug rather than the postal code is the privacy bar: a slug is
// a public content identifier, a full ZIP is semi-identifying at low
// volume. See the 2026-09-06 usage-digest spec's D3.
if len(result.ResolvedAncestry) > 0 {
	u.Increment(usage.KindLookup, result.ResolvedAncestry[0].Slug)
}
```

`metricCountry` is reused so the country bucket is clamped to the same bounded set as the Prometheus label — an arbitrary `?country=` value must not create unbounded rows.

- [ ] **Step 5: Verify it compiles and existing tests still pass**

Run: `cd api && mise exec -- go build ./... && mise exec -- go test ./internal/httpapi/... 2>&1 | tail -20`
Expected: build succeeds; httpapi tests PASS. Existing tests construct `httpapi.Config` without `Usage`, which is nil and therefore a no-op — that is the nil-safety contract, and their passing proves it.

- [ ] **Step 6: Commit**

```bash
git add api/internal/httpapi/router.go api/internal/httpapi/lookup.go \
        api/internal/httpapi/regions.go api/internal/httpapi/orgs.go
git commit -m "feat(api): record daily usage rollups from read handlers"
```

---

### Task 5: `GET /api/v1/admin/usage`

**Files:**
- Modify: `api/openapi.yaml`
- Create: `api/internal/httpapi/usage.go`
- Create: `api/internal/httpapi/usage_test.go`
- Modify: `api/internal/httpapi/router.go`

- [ ] **Step 1: Add the path to openapi.yaml**

Insert immediately after the `/api/v1/admin/coverage-gaps` block, at the same indentation, before the `components:` key:

```yaml
  /api/v1/admin/usage:
    get:
      tags: [admin]
      summary: List daily aggregate usage counts.
      description: |
        Returns accumulated daily usage buckets — content popularity
        (region and org views), the region a postal code resolved to,
        and lookup outcome/tier/country totals. This is the durable
        record behind the monthly usage digest, and it outlives the
        ~30-day Prometheus retention window.

        Buckets hold only public content identifiers (slugs) and bounded
        enum values; raw postal codes and search queries are never
        recorded here. Results are highest-count first.
      operationId: listUsage
      security:
        - BearerAuth: []
      parameters:
        - in: query
          name: from
          required: true
          schema:
            type: string
            format: date
          description: Inclusive start day (YYYY-MM-DD, UTC).
          example: '2026-08-01'
        - in: query
          name: to
          required: true
          schema:
            type: string
            format: date
          description: Inclusive end day (YYYY-MM-DD, UTC).
          example: '2026-08-31'
        - in: query
          name: kind
          required: false
          schema:
            type: string
            enum: [region_view, org_view, lookup, lookup_tier, lookup_result, lookup_country]
          description: Restrict to one bucket kind. Omit for all kinds.
        - in: query
          name: limit
          required: false
          schema:
            type: integer
            minimum: 1
            maximum: 1000
            default: 100
          description: Maximum number of buckets to return. Capped at 1000.
      responses:
        '200':
          description: Usage buckets, highest count first.
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/UsageCount'
        '400':
          $ref: '#/components/responses/BadRequest'
        '401':
          $ref: '#/components/responses/Unauthorized'
        '500':
          $ref: '#/components/responses/InternalError'
```

- [ ] **Step 2: Add the UsageCount schema**

In `components.schemas`, immediately after the `CoverageGap` schema:

```yaml
    UsageCount:
      type: object
      description: |
        One daily aggregate usage bucket. Admin-only. Holds public
        content identifiers and bounded enum values only — never raw
        user input.
      required: [day, kind, key, count]
      properties:
        day:
          type: string
          format: date
          description: UTC calendar day for this bucket.
          example: '2026-08-14'
        kind:
          type: string
          enum: [region_view, org_view, lookup, lookup_tier, lookup_result, lookup_country]
          description: |
            `region_view` / `org_view` — detail fetches, keyed by slug.
            `lookup` — the region a postal code resolved to, keyed by
            region slug. `lookup_tier` / `lookup_result` /
            `lookup_country` — lookup outcome totals, keyed by the
            corresponding enum value.
        key:
          type: string
          description: Region or org slug, or the enum value for outcome kinds.
          example: chicago-il
        count:
          type: integer
          description: Number of events in this bucket on this day.
          example: 42
```

- [ ] **Step 3: Regenerate wire types**

Run: `just api-gen`
Expected: `api/internal/httpapi/oapi/types.gen.go` gains a `UsageCount` struct; `api/internal/httpapi/openapi.yaml` updates to match.

Then: `cd web && npm run generate:api`
Expected: `web/src/lib/api.gen.ts` regenerates cleanly.

- [ ] **Step 4: Write the failing handler test**

Create `api/internal/httpapi/usage_test.go`:

```go
package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

type stubUsageReader struct {
	rows []atlas.UsageCount
	err  error
	// captured args, so the test can assert the handler parsed them.
	gotFrom, gotTo, gotKind string
	gotLimit                int
}

func (s *stubUsageReader) ListUsage(_ context.Context, from, to, kind string, limit int) ([]atlas.UsageCount, error) {
	s.gotFrom, s.gotTo, s.gotKind, s.gotLimit = from, to, kind, limit
	return s.rows, s.err
}

func newUsageRouter(t *testing.T, reader atlas.UsageReader) http.Handler {
	t.Helper()
	return httpapi.New(httpapi.Config{
		Store:      newTestStore(t),
		AdminToken: "test-token",
		Usage:      nil,
		UsageCounts: reader,
	})
}

func TestListUsage_ReturnsBuckets(t *testing.T) {
	reader := &stubUsageReader{rows: []atlas.UsageCount{
		{Day: "2026-08-14", Kind: "region_view", Key: "chicago-il", Count: 42},
	}}
	r := newUsageRouter(t, reader)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage?from=2026-08-01&to=2026-08-31&kind=region_view&limit=25", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var got []struct {
		Day   string `json:"day"`
		Kind  string `json:"kind"`
		Key   string `json:"key"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].Key != "chicago-il" || got[0].Count != 42 {
		t.Errorf("unexpected body: %+v", got)
	}
	if reader.gotFrom != "2026-08-01" || reader.gotTo != "2026-08-31" {
		t.Errorf("range = %s..%s, want 2026-08-01..2026-08-31", reader.gotFrom, reader.gotTo)
	}
	if reader.gotKind != "region_view" || reader.gotLimit != 25 {
		t.Errorf("kind/limit = %s/%d, want region_view/25", reader.gotKind, reader.gotLimit)
	}
}

func TestListUsage_RequiresFromAndTo(t *testing.T) {
	// Without a bounded range the handler would scan the whole table.
	r := newUsageRouter(t, &stubUsageReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage?from=2026-08-01", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestListUsage_RejectsMalformedDate(t *testing.T) {
	r := newUsageRouter(t, &stubUsageReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage?from=August&to=2026-08-31", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
}

func TestListUsage_RequiresBearerToken(t *testing.T) {
	r := newUsageRouter(t, &stubUsageReader{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage?from=2026-08-01&to=2026-08-31", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}
```

> `newTestStore` here is the httpapi package's own test helper. Check
> `api/internal/httpapi/coverage_test.go` for the exact helper name and
> `httpapi.Config` construction the sibling admin tests use, and match
> it — do not invent a new helper.

- [ ] **Step 5: Run the test to verify it fails**

Run: `cd api && mise exec -- go test ./internal/httpapi/... -run TestListUsage -v`
Expected: FAIL — `unknown field UsageCounts in struct literal`.

- [ ] **Step 6: Write the handler**

Create `api/internal/httpapi/usage.go`:

```go
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// dayParamFormat is the YYYY-MM-DD form of the from/to query params.
const dayParamFormat = "2006-01-02"

// listUsageHandler answers GET /api/v1/admin/usage. Bearer-gated.
// Returns accumulated daily usage buckets, highest-count first.
//
// from and to are required: an unbounded range would scan the entire
// rollup table, and every real caller (the digest workflow) knows the
// month it wants.
func listUsageHandler(reader atlas.UsageReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		q := r.URL.Query()

		from, ok := parseDayParam(w, r, q.Get("from"), "from", rid)
		if !ok {
			return
		}
		to, ok := parseDayParam(w, r, q.Get("to"), "to", rid)
		if !ok {
			return
		}
		limit, ok := parseLimitParam(w, r, maxUsageLimit, rid)
		if !ok {
			return
		}

		rows, err := reader.ListUsage(r.Context(), from, to, q.Get("kind"), limit)
		if err != nil {
			logger.ErrorContext(r.Context(), "list usage failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		out := make([]oapi.UsageCount, 0, len(rows))
		for _, c := range rows {
			out = append(out, toOAPIUsageCount(c))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// parseDayParam validates a required YYYY-MM-DD query param, writing a
// 400 problem document and returning ok=false when it is missing or
// malformed.
func parseDayParam(w http.ResponseWriter, r *http.Request, raw, name, rid string) (string, bool) {
	if raw == "" {
		writeProblem(w, r, http.StatusBadRequest, problemBadRequest, "Missing Parameter",
			"The "+name+" query parameter is required (format: YYYY-MM-DD).", rid)
		return "", false
	}
	if _, err := time.Parse(dayParamFormat, raw); err != nil {
		writeProblem(w, r, http.StatusBadRequest, problemBadRequest, "Invalid Parameter",
			"The "+name+" query parameter must be a date in YYYY-MM-DD form.", rid)
		return "", false
	}
	return raw, true
}

func toOAPIUsageCount(c atlas.UsageCount) oapi.UsageCount {
	return oapi.UsageCount{
		Day:   c.Day,
		Kind:  oapi.UsageCountKind(c.Kind),
		Key:   c.Key,
		Count: c.Count,
	}
}
```

> Two things to reconcile against the existing code rather than assume:
> the `problemBadRequest` constant name (grep `problem` in
> `api/internal/httpapi/problem.go`), and whether `oapi.UsageCount.Day`
> generated as `string` or `openapi_types.Date` — `format: date` often
> produces the latter. If it is a Date, convert with
> `time.Parse(dayParamFormat, c.Day)` and wrap it.

- [ ] **Step 7: Add the limit constant and wire the route**

In whichever file holds `maxAdminListLimit` (grep for it), add alongside:

```go
// maxUsageLimit caps GET /api/v1/admin/usage. Higher than the
// submission/coverage list cap because the digest legitimately pulls a
// few hundred buckets per month in one call.
const maxUsageLimit = 1000
```

In `router.go`, add the `UsageCounts` config field beside `CoverageGaps`:

```go
	// UsageCounts, when non-nil, backs the admin GET /api/v1/admin/usage
	// read endpoint. Satisfied by the same SQLite store as Submissions.
	UsageCounts atlas.UsageReader
```

and register the route inside the same guarded block that registers `coverage-gaps`, following its exact nil-guard shape:

```go
r.Get("/usage", listUsageHandler(cfg.UsageCounts, logger))
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `cd api && mise exec -- go test ./internal/httpapi/... -run TestListUsage -v`
Expected: PASS — all four tests.

- [ ] **Step 9: Run the full API gate**

Run: `just api-check`
Expected: lint clean, all tests pass, no codegen drift.

- [ ] **Step 10: Commit**

```bash
git add api/openapi.yaml api/internal/httpapi/ web/src/lib/api.gen.ts
git commit -m "feat(api): add bearer-gated GET /api/v1/admin/usage"
```

---

### Task 6: Wire the recorder into `serve.go`

**Files:**
- Modify: `api/cmd/server/serve.go`

- [ ] **Step 1: Add the config fields and flags**

In the `serveConfig` struct (beside `coverageSampleRate` / `coverageMaxRows` at ~line 50):

```go
	usageFlushInterval time.Duration
	usageKeepDays      int
```

In the config constructor (~line 71):

```go
		usageFlushInterval:     c.Duration("usage-flush-interval"),
		usageKeepDays:          c.Int("usage-keep-days"),
```

In the flag list, after the `coverage-max-rows` flag (~line 156):

```go
			&cli.DurationFlag{
				Name:    "usage-flush-interval",
				Usage:   "how often buffered usage counts are written to SQLite",
				Value:   time.Minute,
				Sources: cli.EnvVars("URBANIST_USAGE_FLUSH_INTERVAL"),
			},
			&cli.IntFlag{
				Name:    "usage-keep-days",
				Usage:   "days of daily usage rollups to retain (0 disables pruning)",
				Value:   400,
				Sources: cli.EnvVars("URBANIST_USAGE_KEEP_DAYS"),
			},
```

400 days is deliberate: it keeps a full year plus a month of margin, so a year-over-year comparison is always available in the digest.

- [ ] **Step 2: Construct and run the recorder**

After the coverage recorder block (~line 204):

```go
	// Usage rollups share the SQLite store with submissions, so like the
	// coverage recorder they exist only when that store does. Unlike
	// coverage, this is on by default: the buckets hold public content
	// identifiers only, and recording them is the point of the slice.
	var usageRec *usage.Recorder
	if subs != nil {
		usageRec = usage.New(subs, cfg.usageFlushInterval, cfg.usageKeepDays, logger)
		go usageRec.Run(ctx)
	}
```

Add the import `"github.com/mjrossi/urbanist-atlas/api/internal/usage"`.

- [ ] **Step 3: Pass it to the router**

In the `httpapi.Config` literal, after `CoverageGaps`:

```go
		Usage:                  usageRec,
		UsageCounts:            usageReaderOrNil(subs),
```

- [ ] **Step 4: Add the typed-nil guard**

Next to `coverageReaderOrNil` (~line 462), mirroring it exactly:

```go
// usageReaderOrNil returns a nil interface (not a typed-nil) when the
// SQLite store is absent, so the router's nil check on UsageCounts
// behaves. Mirrors coverageReaderOrNil — same typed-nil trap.
func usageReaderOrNil(s *sqlite.Store) atlas.UsageReader {
	if s == nil {
		return nil
	}
	return s
}
```

- [ ] **Step 5: Drain on shutdown**

Add to the `shutdownDeps` struct, beside the existing `recorder` field:

```go
	usageRec *usage.Recorder
```

Populate it where `recorder: recorder` is set (~line 268):

```go
		usageRec:    usageRec,
```

In `awaitShutdown`, immediately after the coverage drain:

```go
		// Flush buffered usage counts before the deferred store Close
		// runs, so the last interval isn't lost. Run(ctx) also flushes on
		// its own context cancellation; this is the belt-and-braces call
		// for the case where the signal races the ticker goroutine.
		// Nil-safe.
		if err := d.usageRec.Wait(shutdownCtx); err != nil {
			logger.Warn("usage: final flush incomplete on shutdown", "err", err)
		}
```

- [ ] **Step 6: Log the config**

In `logServeConfig` (~line 288), beside the coverage fields:

```go
		"usage_flush_interval", cfg.usageFlushInterval,
		"usage_keep_days", cfg.usageKeepDays,
```

- [ ] **Step 7: Verify build and full gate**

Run: `cd api && mise exec -- go build ./... && just api-check`
Expected: builds clean, lint clean, tests pass.

- [ ] **Step 8: Manual smoke test**

```bash
just api-run &
sleep 3
curl -s 'http://127.0.0.1:8080/api/v1/regions/chicago-il' > /dev/null
curl -s 'http://127.0.0.1:8080/api/v1/lookup?postal_code=60601&country=US' > /dev/null
```

Wait ~65 seconds for a flush interval to elapse, then:

```bash
TODAY=$(date -u +%F)
curl -s -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
  "http://127.0.0.1:8080/api/v1/admin/usage?from=$TODAY&to=$TODAY" | jq
```

Expected: JSON array containing `region_view`/`chicago-il` and `lookup_result`/`hit` buckets with count 1.

> This needs a SQLite store, so run with `URBANIST_DB_PATH` set to a
> scratch file and `URBANIST_ADMIN_TOKEN` set. If `just api-run` does
> not configure those, set them inline for this smoke test only.

- [ ] **Step 9: Commit**

```bash
git add api/cmd/server/serve.go
git commit -m "feat(api): wire usage recorder into serve lifecycle"
```

---

### Task 7: The monthly digest workflow

**Files:**
- Create: `.github/workflows/usage-digest.yml`

- [ ] **Step 1: Write the workflow**

```yaml
name: Monthly usage digest

# Pushes a monthly summary of how the Atlas is actually being used, as a
# GitHub issue. This is the consumption layer for the usage rollups
# (api/internal/usage) — see
# docs/superpowers/specs/2026-09-06-usage-digest-design.md.
#
# Unlike uptime.yml and backup-sqlite.yml (which reuse one open issue so
# a sustained outage doesn't spam), this opens a NEW issue each month: each
# digest is a durable record, and the issue list is the archive.
#
# Each of the four sources degrades independently to a "unavailable"
# line. A Cloudflare token hiccup must not cost the content and coverage
# numbers. The job fails only if every source fails.

on:
  schedule:
    # 13:00 UTC on the 2nd of each month — a day of margin so the
    # previous month is fully flushed and backed up.
    - cron: '0 13 2 * *'
  workflow_dispatch:

permissions:
  contents: read
  issues: write

jobs:
  digest:
    runs-on: ubuntu-latest
    steps:
      - name: Compute reporting months
        id: months
        run: |
          set -euo pipefail
          # Previous month and the one before it, for deltas.
          echo "cur_start=$(date -u -d 'last month' +%Y-%m-01)" >> "$GITHUB_OUTPUT"
          echo "cur_end=$(date -u -d "$(date -u +%Y-%m-01) -1 day" +%F)" >> "$GITHUB_OUTPUT"
          echo "cur_label=$(date -u -d 'last month' +%Y-%m)" >> "$GITHUB_OUTPUT"
          echo "prev_start=$(date -u -d '2 months ago' +%Y-%m-01)" >> "$GITHUB_OUTPUT"
          echo "prev_end=$(date -u -d "$(date -u -d 'last month' +%Y-%m-01) -1 day" +%F)" >> "$GITHUB_OUTPUT"
          echo "prev_label=$(date -u -d '2 months ago' +%Y-%m)" >> "$GITHUB_OUTPUT"

      - name: Fetch usage rollups
        id: usage
        continue-on-error: true
        env:
          ADMIN_TOKEN: ${{ secrets.URBANIST_ADMIN_TOKEN }}
        run: |
          set -euo pipefail
          fetch() {
            curl -fsS --max-time 30 \
              -H "Authorization: Bearer ${ADMIN_TOKEN}" \
              "https://api.urbanistatlas.com/api/v1/admin/usage?from=$1&to=$2&limit=1000"
          }
          fetch "${{ steps.months.outputs.cur_start }}" "${{ steps.months.outputs.cur_end }}" > usage_cur.json
          fetch "${{ steps.months.outputs.prev_start }}" "${{ steps.months.outputs.prev_end }}" > usage_prev.json
          echo "ok=true" >> "$GITHUB_OUTPUT"

      - name: Fetch coverage gaps
        id: coverage
        continue-on-error: true
        env:
          ADMIN_TOKEN: ${{ secrets.URBANIST_ADMIN_TOKEN }}
        run: |
          set -euo pipefail
          curl -fsS --max-time 30 \
            -H "Authorization: Bearer ${ADMIN_TOKEN}" \
            'https://api.urbanistatlas.com/api/v1/admin/coverage-gaps?limit=200' > coverage.json
          echo "ok=true" >> "$GITHUB_OUTPUT"

      - name: Fetch Cloudflare Web Analytics
        id: cloudflare
        continue-on-error: true
        env:
          CF_TOKEN: ${{ secrets.CF_ANALYTICS_TOKEN }}
          CF_ACCOUNT_ID: ${{ secrets.CF_ACCOUNT_ID }}
          CF_SITE_TAG: ${{ secrets.CF_WEB_ANALYTICS_SITE_TAG }}
        run: |
          set -euo pipefail
          query_month() {
            curl -fsS --max-time 30 https://api.cloudflare.com/client/v4/graphql \
              -H "Authorization: Bearer ${CF_TOKEN}" \
              -H 'Content-Type: application/json' \
              --data @- <<EOF
          {"query":"query { viewer { accounts(filter: {accountTag: \"${CF_ACCOUNT_ID}\"}) { rumPageloadEventsAdaptiveGroups(limit: 1, filter: {siteTag: \"${CF_SITE_TAG}\", datetime_geq: \"$1T00:00:00Z\", datetime_leq: \"$2T23:59:59Z\"}) { count sum { visits } } } } }"}
          EOF
          }
          query_month "${{ steps.months.outputs.cur_start }}" "${{ steps.months.outputs.cur_end }}" > cf_cur.json
          query_month "${{ steps.months.outputs.prev_start }}" "${{ steps.months.outputs.prev_end }}" > cf_prev.json
          echo "ok=true" >> "$GITHUB_OUTPUT"

      - name: Fetch Fly Prometheus health
        id: health
        continue-on-error: true
        env:
          FLY_TOKEN: ${{ secrets.FLY_API_TOKEN_DEPLOY }}
          FLY_ORG: ${{ secrets.FLY_ORG_SLUG }}
        run: |
          set -euo pipefail
          # Fly's managed Prometheus is not reachable over 6PN from here,
          # but it exposes a token-authenticated HTTP query API.
          # Instant queries over a 30d window — Prometheus retention makes
          # a full previous-month range unavailable, so this is a
          # trailing-30d snapshot, labelled as such in the digest.
          q() {
            curl -fsS --max-time 30 -G \
              -H "Authorization: Bearer ${FLY_TOKEN}" \
              --data-urlencode "query=$1" \
              "https://api.fly.io/prometheus/${FLY_ORG}/api/v1/query"
          }
          q 'sum(increase(atlas_http_requests_total[30d]))' > health_requests.json
          q 'sum(increase(atlas_http_requests_total{status=~"5.."}[30d]))' > health_5xx.json
          q 'histogram_quantile(0.95, sum by (le) (rate(atlas_http_request_duration_seconds_bucket[30d])))' > health_p95.json
          q 'sum by (status) (increase(atlas_submissions_total[30d]))' > health_submissions.json
          echo "ok=true" >> "$GITHUB_OUTPUT"

      - name: Fail if every source failed
        if: >
          steps.usage.outputs.ok != 'true' &&
          steps.coverage.outputs.ok != 'true' &&
          steps.cloudflare.outputs.ok != 'true' &&
          steps.health.outputs.ok != 'true'
        run: |
          echo "every digest source failed — not opening an empty issue" >&2
          exit 1

      - name: Render and open the digest issue
        uses: actions/github-script@v9
        env:
          CUR_LABEL: ${{ steps.months.outputs.cur_label }}
          PREV_LABEL: ${{ steps.months.outputs.prev_label }}
          USAGE_OK: ${{ steps.usage.outputs.ok }}
          COVERAGE_OK: ${{ steps.coverage.outputs.ok }}
          CLOUDFLARE_OK: ${{ steps.cloudflare.outputs.ok }}
          HEALTH_OK: ${{ steps.health.outputs.ok }}
        with:
          script: |
            const fs = require('fs');
            const readJSON = (p) => {
              try { return JSON.parse(fs.readFileSync(p, 'utf8')); } catch { return null; }
            };
            const UNAVAILABLE = '_⚠️ Source unavailable this run — see the workflow log._';
            const { CUR_LABEL, PREV_LABEL } = process.env;

            // ---- Content popularity -------------------------------
            const cur = readJSON('usage_cur.json') || [];
            const prev = readJSON('usage_prev.json') || [];
            const totalsFor = (rows, kind) => {
              const m = new Map();
              for (const r of rows.filter(r => r.kind === kind)) {
                m.set(r.key, (m.get(r.key) || 0) + r.count);
              }
              return m;
            };
            const topTable = (kind, heading, n = 15) => {
              const c = totalsFor(cur, kind), p = totalsFor(prev, kind);
              const rows = [...c.entries()].sort((a, b) => b[1] - a[1]).slice(0, n);
              if (!rows.length) return `#### ${heading}\n\n_No data recorded._\n`;
              const lines = rows.map(([k, v]) => {
                const before = p.get(k) || 0;
                const delta = before === 0 ? '—' :
                  `${v - before >= 0 ? '+' : ''}${Math.round(((v - before) / before) * 100)}%`;
                return `| \`${k}\` | ${v} | ${before} | ${delta} |`;
              });
              return `#### ${heading}\n\n| Key | ${CUR_LABEL} | ${PREV_LABEL} | Δ |\n|---|--:|--:|--:|\n${lines.join('\n')}\n`;
            };
            const sumKind = (rows, kind) =>
              rows.filter(r => r.kind === kind).reduce((a, r) => a + r.count, 0);

            let content;
            if (process.env.USAGE_OK === 'true') {
              const outcome = (key) => {
                const c = cur.filter(r => r.kind === 'lookup_result' && r.key === key)
                             .reduce((a, r) => a + r.count, 0);
                return c;
              };
              const hits = outcome('hit'), misses = outcome('miss'), mil = outcome('military');
              const total = hits + misses + mil;
              const rate = total ? Math.round((hits / total) * 100) : 0;
              const empties = cur.filter(r => r.kind === 'lookup_tier' && r.key === 'empty')
                                 .reduce((a, r) => a + r.count, 0);
              content = [
                `**Lookups:** ${total} total — ${hits} resolved (${rate}%), ${misses} missed, ${mil} military.`,
                `**Resolved but empty:** ${empties} (a resolved region with no orgs — the coverage-gap signal).`,
                `**Region detail views:** ${sumKind(cur, 'region_view')} · **Org detail views:** ${sumKind(cur, 'org_view')}`,
                '',
                topTable('lookup', 'Top regions by postal lookup'),
                topTable('region_view', 'Top regions by detail view'),
                topTable('org_view', 'Top organizations'),
              ].join('\n');
            } else {
              content = UNAVAILABLE;
            }

            // ---- Coverage gaps ------------------------------------
            let coverage;
            if (process.env.COVERAGE_OK === 'true') {
              const gaps = readJSON('coverage.json') || [];
              const counts = new Map();
              for (const g of gaps) {
                const k = `${g.kind}: ${g.input}`;
                counts.set(k, (counts.get(k) || 0) + 1);
              }
              const rows = [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 20);
              coverage = rows.length
                ? `| Input | Times seen |\n|---|--:|\n${rows.map(([k, v]) => `| \`${k}\` | ${v} |`).join('\n')}\n\n_Sampled at \`URBANIST_COVERAGE_SAMPLE_RATE\`; a recent partial sample, not a full log._`
                : '_No coverage gaps sampled._';
            } else {
              coverage = UNAVAILABLE;
            }

            // ---- Audience -----------------------------------------
            let audience;
            if (process.env.CLOUDFLARE_OK === 'true') {
              const pick = (p) => {
                const j = readJSON(p);
                const g = j?.data?.viewer?.accounts?.[0]?.rumPageloadEventsAdaptiveGroups?.[0];
                return g ? { views: g.count ?? 0, visits: g.sum?.visits ?? 0 } : null;
              };
              const c = pick('cf_cur.json'), p = pick('cf_prev.json');
              if (c) {
                const delta = (a, b) => (b ? `${a - b >= 0 ? '+' : ''}${Math.round(((a - b) / b) * 100)}%` : '—');
                audience = `| Metric | ${CUR_LABEL} | ${PREV_LABEL} | Δ |\n|---|--:|--:|--:|\n` +
                  `| Page views | ${c.views} | ${p ? p.views : '—'} | ${p ? delta(c.views, p.views) : '—'} |\n` +
                  `| Visits | ${c.visits} | ${p ? p.visits : '—'} | ${p ? delta(c.visits, p.visits) : '—'} |`;
              } else {
                audience = UNAVAILABLE;
              }
            } else {
              audience = UNAVAILABLE;
            }

            // ---- Health -------------------------------------------
            let health;
            if (process.env.HEALTH_OK === 'true') {
              const scalar = (p) => {
                const j = readJSON(p);
                const v = j?.data?.result?.[0]?.value?.[1];
                return v === undefined ? null : Number(v);
              };
              const reqs = scalar('health_requests.json');
              const errs = scalar('health_5xx.json');
              const p95 = scalar('health_p95.json');
              health = [
                `**Requests (trailing 30d):** ${reqs === null ? '—' : Math.round(reqs)}`,
                `**5xx:** ${errs === null ? '—' : Math.round(errs)}` +
                  (reqs && errs !== null ? ` (${(errs / reqs * 100).toFixed(2)}%)` : ''),
                `**p95 latency:** ${p95 === null ? '—' : `${(p95 * 1000).toFixed(0)} ms`}`,
                '',
                '_Prometheus retains ~30 days, so this is a trailing-30d snapshot rather than a calendar month._',
              ].join('\n');
            } else {
              health = UNAVAILABLE;
            }

            const runUrl = `${context.serverUrl}/${context.repo.owner}/${context.repo.repo}/actions/runs/${context.runId}`;
            const body = [
              `# Usage digest — ${CUR_LABEL}`,
              '',
              '## Audience', '', audience, '',
              '## What people looked at', '', content, '',
              '## Where coverage fell short', '', coverage, '',
              '## Health', '', health, '',
              '---', '',
              `Generated by [\`usage-digest.yml\`](${runUrl}). Design: \`docs/superpowers/specs/2026-09-06-usage-digest-design.md\`.`,
            ].join('\n');

            await github.rest.issues.create({
              owner: context.repo.owner,
              repo: context.repo.repo,
              title: `Usage digest — ${CUR_LABEL}`,
              body,
            });
```

- [ ] **Step 2: Fix the stray non-ASCII characters**

The heredoc above contains two characters that must be corrected before committing — a CJK character in "issue each month" (should read "a NEW issue each month"). Verify with:

Run: `LC_ALL=C grep -n '[^\x00-\x7F]' .github/workflows/usage-digest.yml`
Expected after fixing: only the intended `⚠️`, `Δ`, and `·` characters in the script body remain. Replace any other non-ASCII with its ASCII equivalent.

- [ ] **Step 3: Validate the YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/usage-digest.yml')); print('ok')"`
Expected: `ok`

- [ ] **Step 3: Document the new secrets**

Add a comment block near the top of the workflow, after the description:

```yaml
# Repo secrets required:
#   - URBANIST_ADMIN_TOKEN         (NEW) matches the Fly secret of the same name
#   - CF_ANALYTICS_TOKEN           (NEW) Cloudflare API token, Account Analytics:Read
#   - CF_WEB_ANALYTICS_SITE_TAG    (NEW) the Web Analytics site tag
#   - CF_ACCOUNT_ID                (existing, used by backup-sqlite.yml)
#   - FLY_API_TOKEN_DEPLOY         (existing, used by ci.yml + backup-sqlite.yml)
#   - FLY_ORG_SLUG                 (NEW) Fly org slug for the Prometheus API path
```

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/usage-digest.yml
git commit -m "feat(ci): monthly usage digest workflow"
```

- [ ] **Step 5: Dry-run it**

Push the branch, then in the GitHub UI: Actions → Monthly usage digest → Run workflow (select the branch).

Expected: the job completes and opens an issue titled `Usage digest — YYYY-MM`. Sections whose secrets are not yet configured show the "⚠️ Source unavailable" line rather than failing the run — that is the degradation contract working. Add the secrets and re-run to fill them in.

> The rollup table starts empty on first deploy, so the first digest's
> content section will legitimately read "No data recorded." That is
> expected, not a bug — see the spec's note that the digest cannot
> backfill.

---

### Task 8: Documentation

**Files:**
- Modify: `docs/deploy.md`
- Modify: `CLAUDE.md`
- Modify: `docs/superpowers/specs/2026-09-06-usage-digest-design.md`

- [ ] **Step 1: Add the runbook subsection**

In `docs/deploy.md` §Monitoring & incident response, after the "Coverage gaps (editorial)" bullet:

```markdown
- **Usage rollups (product).** Daily aggregate counts of content
  popularity and lookup outcomes, kept ~400 days on the SQLite volume
  (so they survive Prometheus's ~30-day window and ride the nightly R2
  backup). Read them directly:

  ```sh
  curl -fsS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
    'https://api.urbanistatlas.com/api/v1/admin/usage?from=2026-08-01&to=2026-08-31&kind=region_view&limit=25' | jq
  ```

  Tuned by `URBANIST_USAGE_FLUSH_INTERVAL` (default 1m) and
  `URBANIST_USAGE_KEEP_DAYS` (default 400). Counts buffer in RAM between
  flushes, so an ungraceful machine kill loses at most one interval.

  Note that per-slug popularity is recorded **here**, not in the logs.
  The `region view` / `org view` DEBUG slog lines are a debugging aid
  only — do not set `URBANIST_LOG_LEVEL=debug` in production to answer
  popularity questions.
```

Then add to the "What pages you" list:

```markdown
- **Monthly usage digest** — [`usage-digest.yml`](../.github/workflows/usage-digest.yml)
  opens an issue on the 2nd of each month summarising audience, content
  popularity, coverage gaps, and health. Unlike the alarms above, each
  month gets its own issue: the issue list is the archive.
```

- [ ] **Step 2: Fix the CLAUDE.md dependency list**

In `CLAUDE.md` §Tech conventions §Go, in the approved-exceptions list, after the `go-cmp` entry:

```markdown
  - `github.com/sqlc-dev/sqlc` — SQL-to-Go bindings for the SQLite
    submission/usage store, generated from
    `api/internal/store/sqlite/queries/` (build-time tool, not a runtime
    dependency; regenerated via `just api-gen`)
```

> Confirm the module path with `grep sqlc api/go.mod mise.toml` and use
> whatever is actually pinned there.

- [ ] **Step 3: Flip the spec status**

In `docs/superpowers/specs/2026-09-06-usage-digest-design.md`, change the header line to:

```markdown
**Status:** Shipped (2026-09-06).
```

- [ ] **Step 4: Verify docs links resolve**

Run: `grep -n 'usage-digest' docs/deploy.md CLAUDE.md .github/workflows/usage-digest.yml`
Expected: each referenced path exists on disk.

- [ ] **Step 5: Final full gate**

Run: `just api-check && cd web && npm run build && npm run lint`
Expected: everything passes.

- [ ] **Step 6: Commit**

```bash
git add docs/deploy.md CLAUDE.md docs/superpowers/specs/2026-09-06-usage-digest-design.md
git commit -m "docs: usage digest runbook and sqlc dependency note"
```

---

## Verification Checklist

- [ ] `just api-check` passes (lint, race tests, codegen drift)
- [ ] `cd web && npm run build && npm run lint` passes
- [ ] Manual smoke test in Task 6 Step 8 returns populated buckets
- [ ] `workflow_dispatch` run of `usage-digest.yml` opens an issue
- [ ] New repo secrets configured: `URBANIST_ADMIN_TOKEN`, `CF_ANALYTICS_TOKEN`, `CF_WEB_ANALYTICS_SITE_TAG`, `FLY_ORG_SLUG`
- [ ] `URBANIST_LOG_LEVEL` still unset in `fly.toml` (spec D5)
