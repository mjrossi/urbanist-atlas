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

// flushTimeout bounds the final shutdown flush so a wedged DB can't
// stall process exit.
const flushTimeout = 5 * time.Second

// maxKeyLen and maxBufferedKeys are defense in depth, not the primary
// guard. Every caller is supposed to pass a canonical slug or a bounded
// enum value (see the Kind* docs), so neither bound should ever be
// reached in normal operation. They exist so that a future caller that
// forgets and passes raw request input cannot turn the buffer into an
// unbounded allocation or write oversized rows into a WITHOUT ROWID
// table on the shared 1 GiB volume. Over-long keys and overflow past the
// cap are dropped, not truncated: a truncated key would silently merge
// distinct buckets, which is worse than losing a count.
const (
	maxKeyLen       = 128
	maxBufferedKeys = 10000
)

// CountStore is the persistence seam the Recorder writes through —
// satisfied by *sqlite.Store. Mirrors coverage.GapStore.
type CountStore interface {
	UpsertUsageCounts(ctx context.Context, counts []atlas.UsageCount) error
	PruneUsage(ctx context.Context, cutoffDay string) error
}

// bucketKey identifies one accumulator slot. day is captured at
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

	// wg tracks the Run goroutine so Wait can join it, matching
	// coverage.Recorder.Wait's contract — see Wait.
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

// SetClock overrides the time source. Tests pin it so day bucketing and
// prune cutoffs are deterministic; production uses time.Now().UTC().
// Mirrors coverage.Recorder.SetRNG.
func (r *Recorder) SetClock(fn func() time.Time) {
	if r == nil || fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.now = fn
}

// Increment buckets one event. It returns immediately — nothing touches
// the database until the next flush. Blank kind or key is dropped: a
// blank slug would create a meaningless bucket that pollutes the
// digest's top-N sections. An over-long key is dropped too (see
// maxKeyLen).
func (r *Recorder) Increment(kind, key string) {
	if r == nil || r.store == nil || kind == "" || key == "" || len(key) > maxKeyLen {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bk := bucketKey{day: r.now().Format(dayFormat), kind: kind, key: key}
	// Bumping a bucket that already exists costs no new memory, so the
	// cap applies only to new keys.
	if _, ok := r.buf[bk]; !ok && len(r.buf) >= maxBufferedKeys {
		r.dropped++
		return
	}
	r.buf[bk]++
}

// Start launches Run in its own goroutine, registering it with the
// WaitGroup *before* returning so a Wait that follows immediately is
// guaranteed to see it. Production callers should use this rather than
// `go rec.Run(ctx)`, which registers from inside the new goroutine and
// so races a Wait on a very short-lived process. Nil-safe.
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

// Run drives the flush ticker until ctx is canceled, then performs one
// final flush so the last interval's counts survive shutdown. Intended
// to be called in its own goroutine; prefer Start, which closes the
// registration race.
func (r *Recorder) Run(ctx context.Context) {
	if r == nil || r.store == nil {
		return
	}
	r.wg.Add(1)
	defer r.wg.Done()
	r.run(ctx)
}

// run is the ticker loop shared by Start and Run. It assumes the caller
// has already registered with r.wg.
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
			// Detached context: the caller's ctx is already canceled,
			// so reusing it would abort the very write we need.
			flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
			if err := r.Flush(flushCtx); err != nil {
				r.logger.Warn("usage: final flush failed", "err", err)
			}
			cancel()
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
	dropped := r.dropped
	r.dropped = 0
	cutoff := ""
	if r.keepDays > 0 {
		cutoff = r.now().AddDate(0, 0, -r.keepDays).Format(dayFormat)
	}
	r.mu.Unlock()

	// Reaching maxBufferedKeys means a caller is passing unbounded keys;
	// that is a bug worth seeing, not a routine condition.
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

// Wait performs a final flush and then blocks until the Run goroutine
// has exited, or until ctx is done — whichever comes first. Called
// during graceful shutdown alongside coverage.Recorder.Wait, and it
// means the same thing there: when Wait returns nil, no further write
// will be issued, so the caller may safely close the store.
//
// Joining matters because Run performs its own final flush on ctx
// cancellation using a *detached* context. Without the join, Wait could
// return while that flush is still in flight and the deferred store
// Close would race it. If Run was never started the WaitGroup is empty
// and the join returns immediately. Nil-safe.
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
		// Report the flush error in preference to the deadline: it says
		// more about what actually went wrong.
		if flushErr != nil {
			return flushErr
		}
		return ctx.Err()
	}
}
