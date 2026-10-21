// Package usage records daily aggregate usage counts (content popularity
// and lookup outcomes) behind the monthly usage digest.
//
// Unlike internal/coverage, which writes rare sampled events one at a
// time, usage counts arrive on every read request, so they accumulate in
// memory and flush on an interval: SQLite sees one small transaction per
// interval whatever the traffic, and never contends with submissions for
// the single connection. Buffering in RAM relies on fly.toml running one
// machine that is never auto-stopped; an ungraceful kill loses at most
// one interval of counts, acceptable for analytics but not for
// submissions.
package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Bucket kinds: the only values written to usage_daily.kind. They must
// match the CHECK constraint in migration 0003.
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

// flushTimeout bounds the final shutdown flush so a wedged DB can't
// stall process exit.
const flushTimeout = 5 * time.Second

// maxKeyLen and maxBufferedKeys are defense in depth: callers pass
// canonical slugs or enum values, so neither should be reached. They stop
// a caller that passes raw request input from growing the buffer without
// bound or writing oversized rows. Excess keys are dropped, not
// truncated, because a truncated key would silently merge buckets.
const (
	maxKeyLen       = 128
	maxBufferedKeys = 10000
)

// CountStore is the persistence seam the Recorder writes through.
type CountStore interface {
	UpsertUsageCounts(ctx context.Context, counts []atlas.UsageCount) error
	PruneUsage(ctx context.Context, cutoffDay string) error
}

// bucketKey identifies one accumulator slot. day is taken at Increment
// time, so an interval spanning midnight credits each event to its day.
type bucketKey struct {
	day  string
	kind string
	key  string
}

// Recorder accumulates usage counts in memory and flushes them on an
// interval. A nil *Recorder is a valid no-op, so handlers can call
// Increment unconditionally.
type Recorder struct {
	store    CountStore
	interval time.Duration
	keepDays int
	logger   *slog.Logger

	// wg tracks the flush goroutine so Wait can join it.
	wg sync.WaitGroup

	mu      sync.Mutex
	buf     map[bucketKey]int
	dropped int
	now     func() time.Time
}

// New builds a Recorder. interval is the flush cadence (non-positive
// falls back to one minute); keepDays bounds retention (<= 0 disables
// pruning).
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

// SetClock overrides the time source, for deterministic day bucketing
// and prune cutoffs in tests.
func (r *Recorder) SetClock(fn func() time.Time) {
	if r == nil || fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = fn
}

// Increment buckets one event without touching the database. A blank
// kind or key, or an over-long key, is dropped; a blank slug would only
// pollute the digest's top-N.
func (r *Recorder) Increment(kind, key string) {
	if r == nil || r.store == nil || kind == "" || key == "" || len(key) > maxKeyLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bk := bucketKey{day: r.now().Format(dayFormat), kind: kind, key: key}
	// Bumping an existing bucket costs no memory, so the cap applies only
	// to new keys.
	if _, ok := r.buf[bk]; !ok && len(r.buf) >= maxBufferedKeys {
		r.dropped++
		return
	}
	r.buf[bk]++
}

// Start launches the flush goroutine: a flush every interval, then a
// final one when ctx is canceled. It registers with the WaitGroup before
// returning, so an immediate Wait still joins it. Nil-safe.
func (r *Recorder) Start(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.run(ctx)
	}()
}

func (r *Recorder) run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := r.Flush(ctx); err != nil {
				r.logger.Warn("usage: periodic flush failed", "err", err)
			}
		case <-ctx.Done():
			// ctx is already canceled; reusing it would abort the final write.
			flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
			if err := r.Flush(flushCtx); err != nil {
				r.logger.Warn("usage: final flush failed", "err", err)
			}
			cancel()
			return
		}
	}
}

// Flush writes the buffered deltas and clears the buffer, snapshotting
// under the lock and writing outside it so requests never wait on the
// database. A failed batch is dropped rather than re-queued: the upsert
// accumulates, so retrying a partial commit would double-count.
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
	dropped := r.dropped
	r.dropped = 0
	cutoff := ""
	if r.keepDays > 0 {
		cutoff = r.now().AddDate(0, 0, -r.keepDays).Format(dayFormat)
	}
	r.mu.Unlock()

	// Reaching maxBufferedKeys means a caller passes unbounded keys: a bug.
	if dropped > 0 {
		r.logger.Warn("usage: buffer cap reached, counts dropped",
			"dropped", dropped, "cap", maxBufferedKeys)
	}

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

// Wait flushes, then blocks until the flush goroutine exits or ctx is
// done. A nil return means no further write will be issued, so the
// store can be closed: the join matters because the goroutine's own
// final flush runs on a detached context. Nil-safe.
func (r *Recorder) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	flushErr := r.Flush(ctx)

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return flushErr
	case <-ctx.Done():
		// The flush error says more than the deadline.
		if flushErr != nil {
			return flushErr
		}
		return ctx.Err()
	}
}
