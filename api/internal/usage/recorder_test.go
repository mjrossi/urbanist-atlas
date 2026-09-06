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

func (f *fakeStore) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// fixedClock pins "now" so day bucketing and prune cutoffs are
// deterministic.
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
	sortCounts := cmpopts.SortSlices(func(a, b atlas.UsageCount) bool {
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Key < b.Key
	})
	if diff := cmp.Diff(want, store.flushed(), sortCounts); diff != "" {
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

func TestRecorder_IgnoresBlankKindOrKey(t *testing.T) {
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
	store.setErr(nil)
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
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
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

func TestRecorder_RunFlushesOnInterval(t *testing.T) {
	// The ticker path — counts must reach the store without any
	// shutdown or manual Flush.
	store := &fakeStore{}
	r := usage.New(store, 20*time.Millisecond, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	r.Increment(usage.KindRegionView, "chicago-il")

	deadline := time.After(2 * time.Second)
	for {
		if len(store.flushed()) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("ticker did not flush within 2s")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
