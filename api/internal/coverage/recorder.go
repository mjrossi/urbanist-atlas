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
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		defer cancel()
		if err := r.store.RecordCoverageGap(ctx, kind, country, input); err != nil {
			r.logger.ErrorContext(ctx, "coverage: record failed", "err", err, "kind", kind)
			return
		}
		if err := r.store.PruneCoverageGaps(ctx, r.maxRows); err != nil {
			r.logger.WarnContext(ctx, "coverage: prune failed", "err", err)
		}
	}()
}

// Wait blocks until all in-flight coverage writes have finished. Called
// during graceful shutdown (and by tests) so sampled rows aren't lost.
// Nil-safe.
func (r *Recorder) Wait() {
	if r == nil {
		return
	}
	r.wg.Wait()
}
