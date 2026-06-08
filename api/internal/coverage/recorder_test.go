package coverage

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeGapStore struct {
	mu      sync.Mutex
	records []record
	pruned  []int
	recErr  error
}

type record struct{ kind, country, input string }

func (f *fakeGapStore) RecordCoverageGap(_ context.Context, kind, country, input string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recErr != nil {
		return f.recErr
	}
	f.records = append(f.records, record{kind, country, input})
	return nil
}

func (f *fakeGapStore) PruneCoverageGaps(_ context.Context, maxRows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pruned = append(f.pruned, maxRows)
	return nil
}

func (f *fakeGapStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.records)
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRecorder_NilSafe(t *testing.T) {
	var r *Recorder
	r.RecordEmpty("lookup", "US", "00000") // must not panic
	r.Wait(context.Background())           // must not panic
}

func TestRecorder_DisabledSampleRate(t *testing.T) {
	store := &fakeGapStore{}
	r := New(store, 0, 100, discardLogger())
	r.RecordEmpty("lookup", "US", "00000")
	r.Wait(context.Background())
	if got := store.count(); got != 0 {
		t.Errorf("disabled recorder wrote %d rows, want 0", got)
	}
}

func TestRecorder_AlwaysSamples(t *testing.T) {
	store := &fakeGapStore{}
	r := New(store, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 }) // always below the rate
	r.RecordEmpty("lookup", "US", "00000")
	r.RecordEmpty("search", "", "nope")
	r.Wait(context.Background())

	if got := store.count(); got != 2 {
		t.Fatalf("wrote %d rows, want 2", got)
	}
	// Each successful write triggers a prune with the configured cap.
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.pruned) != 2 {
		t.Fatalf("prune calls = %d, want 2", len(store.pruned))
	}
	for _, n := range store.pruned {
		if n != 100 {
			t.Errorf("prune cap = %d, want 100", n)
		}
	}
}

func TestRecorder_SkipsUnsampled(t *testing.T) {
	store := &fakeGapStore{}
	r := New(store, 0.5, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.9 }) // 0.9 >= 0.5 → skip
	r.RecordEmpty("lookup", "US", "00000")
	r.Wait(context.Background())
	if got := store.count(); got != 0 {
		t.Errorf("unsampled recorder wrote %d rows, want 0", got)
	}
}

func TestRecorder_WriteErrorSwallowed(t *testing.T) {
	store := &fakeGapStore{recErr: errors.New("boom")}
	r := New(store, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 })
	r.RecordEmpty("lookup", "US", "00000") // error is logged + swallowed, never panics
	r.Wait(context.Background())

	// A failed write does not prune.
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.pruned) != 0 {
		t.Errorf("prune called after write error: %v", store.pruned)
	}
}

// blockingGapStore holds each write on `block` until the test releases it,
// signaling `started` first so the test can confirm a write is in flight.
type blockingGapStore struct {
	started chan struct{}
	block   chan struct{}
	mu      sync.Mutex
	n       int
}

func (b *blockingGapStore) RecordCoverageGap(_ context.Context, _, _, _ string) error {
	b.started <- struct{}{}
	<-b.block
	b.mu.Lock()
	b.n++
	b.mu.Unlock()
	return nil
}

func (b *blockingGapStore) PruneCoverageGaps(_ context.Context, _ int) error { return nil }

func (b *blockingGapStore) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.n
}

// Wait must honor its context so a wedged write can't make graceful
// shutdown overrun its budget (the per-write timeout is independent of the
// shutdown deadline).
func TestRecorder_WaitRespectsContextDeadline(t *testing.T) {
	store := &blockingGapStore{started: make(chan struct{}, 1), block: make(chan struct{})}
	defer close(store.block) // release the stuck write so its goroutine can exit
	r := New(store, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 })

	r.RecordEmpty("lookup", "US", "00000") // spawns a write that blocks
	<-store.started                        // the write is now in flight

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // deadline already elapsed

	got := make(chan error, 1)
	go func() { got <- r.Wait(ctx) }()
	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after its context was canceled; drain is not bounded by the shutdown budget")
	}
}

type panicGapStore struct{}

func (panicGapStore) RecordCoverageGap(context.Context, string, string, string) error {
	panic("store boom")
}
func (panicGapStore) PruneCoverageGaps(context.Context, int) error { return nil }

// A panic in the detached write goroutine must be contained — the
// recoverer middleware only guards request goroutines, so an unrecovered
// panic here would take down the whole process.
func TestRecorder_RecoversFromWritePanic(t *testing.T) {
	r := New(panicGapStore{}, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 })
	r.RecordEmpty("lookup", "US", "00000") // the store panics inside the goroutine
	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// Reaching here without the test binary crashing proves the recover worked.
}

// RecordEmpty must cap concurrent background writes and drop (never queue
// or block) further samples once saturated, so a flood of empty-result
// requests can't pile up goroutines and starve the single shared write
// connection. Capture is best-effort by design.
func TestRecorder_DropsWhenSaturated(t *testing.T) {
	store := &blockingGapStore{started: make(chan struct{}, maxConcurrentWrites), block: make(chan struct{})}
	r := New(store, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 })

	// Occupy every write slot; each write blocks, holding its slot.
	for range maxConcurrentWrites {
		r.RecordEmpty("search", "", "blocked")
	}
	for range maxConcurrentWrites {
		<-store.started // all slots are now held by in-flight writes
	}

	// The next sample finds no free slot. It must be dropped without
	// blocking the caller — a blocking acquire here would hang the test.
	r.RecordEmpty("search", "", "dropped")

	close(store.block) // release the held writes
	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if got := store.count(); got != maxConcurrentWrites {
		t.Fatalf("recorded %d rows, want %d (the over-cap sample should have been dropped)", got, maxConcurrentWrites)
	}
}

// Concurrent RecordEmpty calls must be race-free across the sampling gate,
// the slot semaphore, the WaitGroup, and the store. Run under -race this
// guards the fire-and-forget path against future regressions.
func TestRecorder_ConcurrentRecordEmpty(t *testing.T) {
	store := &fakeGapStore{}
	r := New(store, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 })

	const n = 64
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			r.RecordEmpty("search", "", "concurrent")
		})
	}
	wg.Wait() // every RecordEmpty call has returned
	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// Some samples may be dropped under saturation; the count must stay sane.
	if got := store.count(); got < 1 || got > n {
		t.Fatalf("recorded %d rows, want 1..%d", got, n)
	}
}
