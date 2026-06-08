package coverage

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
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
	r.Wait()                               // must not panic
}

func TestRecorder_DisabledSampleRate(t *testing.T) {
	store := &fakeGapStore{}
	r := New(store, 0, 100, discardLogger())
	r.RecordEmpty("lookup", "US", "00000")
	r.Wait()
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
	r.Wait()

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
	r.Wait()
	if got := store.count(); got != 0 {
		t.Errorf("unsampled recorder wrote %d rows, want 0", got)
	}
}

func TestRecorder_WriteErrorSwallowed(t *testing.T) {
	store := &fakeGapStore{recErr: errors.New("boom")}
	r := New(store, 1.0, 100, discardLogger())
	r.SetRNG(func() float64 { return 0.0 })
	r.RecordEmpty("lookup", "US", "00000") // error is logged + swallowed, never panics
	r.Wait()

	// A failed write does not prune.
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.pruned) != 0 {
		t.Errorf("prune called after write error: %v", store.pruned)
	}
}
