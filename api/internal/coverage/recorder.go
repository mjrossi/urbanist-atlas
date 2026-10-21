// Package coverage records sampled empty-result lookups and searches,
// the editorial "which input returns nothing?" signal. It owns the
// sampling gate and the fire-and-forget write, so the request path never
// blocks on, or fails for, a coverage write.
package coverage

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

// writeTimeout bounds each background write so a wedged DB can't leak
// goroutines.
const writeTimeout = 2 * time.Second

// maxConcurrentWrites caps in-flight background writes. The SQLite store
// has one connection, shared with submissions, so an unbounded fan-out at
// a high sample rate could starve it. Samples over the cap are dropped,
// which also bounds the shutdown drain.
const maxConcurrentWrites = 4

// maxInputLen caps the bytes stored per row: the table is row-capped, not
// byte-capped, and shares its volume with submissions. 400 bytes holds
// the spec's 100-character search q at UTF-8's 4-byte maximum.
const maxInputLen = 400

// GapStore is the persistence seam the Recorder writes through.
type GapStore interface {
	RecordCoverageGap(ctx context.Context, kind, country, input string) error
	PruneCoverageGaps(ctx context.Context, maxRows int) error
}

// Recorder samples empty-result events and persists them off the request
// path. A nil *Recorder is a valid no-op, so handlers can call
// RecordEmpty unconditionally.
type Recorder struct {
	store      GapStore
	sampleRate float64
	maxRows    int
	logger     *slog.Logger
	rng        func() float64
	sem        chan struct{} // see maxConcurrentWrites
	wg         sync.WaitGroup
}

// New builds a Recorder. sampleRate is the probability (0..1) that an
// event is persisted; <= 0 disables capture. maxRows caps the table,
// pruned after each write; <= 0 leaves it unbounded.
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

// SetRNG overrides the sampling source, for deterministic tests.
func (r *Recorder) SetRNG(fn func() float64) {
	if r != nil && fn != nil {
		r.rng = fn
	}
}

// RecordEmpty samples and, if selected, persists an empty-result event.
// kind is "lookup" or "search"; country is "" for searches; input is the
// normalized postal code or query. The write runs in the background on a
// detached, bounded context, so request cancellation can't abort it.
func (r *Recorder) RecordEmpty(kind, country, input string) {
	if r == nil || r.store == nil || r.sampleRate <= 0 {
		return
	}
	if r.rng() >= r.sampleRate {
		return
	}
	input = truncate(input, maxInputLen)
	// Never block the caller: drop the sample when the writer is saturated.
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

// truncate cuts s to at most n bytes, dropping any UTF-8 sequence the cut
// splits.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

// Wait blocks until in-flight writes finish or ctx is done, returning
// ctx.Err() in the latter case; abandoned writes are best-effort.
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
