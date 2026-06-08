// Package coverage records sampled empty-result lookups and searches —
// the editorial "which input returns nothing?" signal. It is the seam
// between the HTTP handlers (which know when a result came back empty)
// and the SQLite store (which persists the sampled rows), owning the
// sampling gate and the fire-and-forget write so handlers stay thin and
// the request path never blocks on, or fails for, a coverage write.
package coverage

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"
)

// writeTimeout bounds each background coverage write so a wedged DB
// can't leak goroutines. Generous for a local SQLite write.
const writeTimeout = 2 * time.Second

// maxConcurrentWrites caps in-flight background coverage writes. The SQLite
// store serializes writes through a single connection shared with the
// submission path, so an unbounded fan-out of sampled empties — easy to
// provoke at a high sample rate — could pile up goroutines and starve that
// connection. Samples over the cap are dropped (capture is best-effort),
// which also keeps the shutdown drain bounded.
const maxConcurrentWrites = 4

// GapStore is the persistence seam the Recorder writes through —
// satisfied by *sqlite.Store.
type GapStore interface {
	RecordCoverageGap(ctx context.Context, kind, country, input string) error
	PruneCoverageGaps(ctx context.Context, maxRows int) error
}

// Recorder samples empty-result events and persists them off the request
// path. A nil *Recorder is a valid no-op (capture disabled), so handlers
// can call RecordEmpty unconditionally — mirroring the *Metrics
// nil-guard convention.
type Recorder struct {
	store      GapStore
	sampleRate float64
	maxRows    int
	logger     *slog.Logger
	rng        func() float64
	sem        chan struct{} // bounds concurrent background writes; see maxConcurrentWrites
	wg         sync.WaitGroup
}

// New builds a Recorder. sampleRate is the probability (0..1) that a
// given empty-result event is persisted; <= 0 disables capture entirely.
// maxRows caps the table (pruned opportunistically after each write);
// <= 0 leaves it unbounded.
func New(store GapStore, sampleRate float64, maxRows int, logger *slog.Logger) *Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Recorder{
		store:      store,
		sampleRate: sampleRate,
		maxRows:    maxRows,
		logger:     logger,
		rng:        rand.Float64,
		sem:        make(chan struct{}, maxConcurrentWrites),
	}
}

// SetRNG overrides the sampling source. Tests inject a deterministic
// function; production uses math/rand/v2's concurrency-safe Float64.
func (r *Recorder) SetRNG(fn func() float64) {
	if r != nil && fn != nil {
		r.rng = fn
	}
}

// RecordEmpty samples and (if selected) persists an empty-result event
// off the request path. kind is "lookup" or "search"; country is "" for
// searches; input is the normalized postal code or the search query.
//
// It returns immediately — the write runs in a background goroutine with
// a detached, bounded context so request cancellation can't abort it. A
// nil Recorder, a disabled sample rate, or an unsampled event are all
// no-ops.
func (r *Recorder) RecordEmpty(kind, country, input string) {
	if r == nil || r.store == nil || r.sampleRate <= 0 {
		return
	}
	if r.rng() >= r.sampleRate {
		return
	}
	// Acquire a write slot without blocking the caller. When the writer is
	// saturated, drop the sample: the request path must never wait on a
	// coverage write, and capture is best-effort (sampled + row-capped).
	select {
	case r.sem <- struct{}{}:
	default:
		r.logger.Debug("coverage: sample dropped, writer saturated", "kind", kind)
		return
	}
	r.wg.Go(func() {
		defer func() { <-r.sem }()
		defer func() {
			if p := recover(); p != nil {
				r.logger.Error("coverage: write panicked", "panic", p, "kind", kind)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()
		if err := r.store.RecordCoverageGap(ctx, kind, country, input); err != nil {
			r.logger.ErrorContext(ctx, "coverage: record failed", "err", err, "kind", kind)
			return
		}
		if err := r.store.PruneCoverageGaps(ctx, r.maxRows); err != nil {
			r.logger.WarnContext(ctx, "coverage: prune failed", "err", err)
		}
	})
}

// Wait blocks until all in-flight coverage writes have finished, or until
// ctx is done — whichever comes first. Called during graceful shutdown
// (with the shutdown context) so sampled rows aren't lost, while the
// shared deadline keeps a wedged write from overrunning the shutdown
// budget. Returns ctx.Err() if the deadline wins; in-flight writes are
// detached and best-effort, so abandoned stragglers are acceptable.
// Nil-safe.
func (r *Recorder) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
