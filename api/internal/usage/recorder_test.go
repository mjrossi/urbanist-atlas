package usage_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
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
	if got := store.flushed(); len(got) != 0 {
		t.Errorf("empty flush should not call the store, got %d counts", len(got))
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
	// is canceled, so the last interval's counts aren't lost.
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

	// Generous deadline against a 20ms ticker: the assertion is that the
	// ticker path flushes at all, not how promptly, so a loaded CI runner
	// should never turn this into a red build.
	deadline := time.After(10 * time.Second)
	for {
		if len(store.flushed()) > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("ticker did not flush within 10s")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestRecorder_BucketsByDayOfEvent(t *testing.T) {
	// The documented invariant: day is captured at Increment, not at
	// flush, so an interval spanning UTC midnight attributes each event
	// to the day it actually happened instead of smearing the whole
	// buffer onto the flush-time day.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)

	r.SetClock(fixedClock("2026-09-01"))
	r.Increment(usage.KindRegionView, "chicago")
	r.SetClock(fixedClock("2026-09-02"))
	r.Increment(usage.KindRegionView, "chicago")

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	want := []atlas.UsageCount{
		{Day: "2026-09-01", Kind: usage.KindRegionView, Key: "chicago", Count: 1},
		{Day: "2026-09-02", Kind: usage.KindRegionView, Key: "chicago", Count: 1},
	}
	sortCounts := cmpopts.SortSlices(func(a, b atlas.UsageCount) bool { return a.Day < b.Day })
	if diff := cmp.Diff(want, store.flushed(), sortCounts); diff != "" {
		t.Errorf("one flush spanning midnight should write two days (-want +got):\n%s", diff)
	}
}

func TestRecorder_DropsOverlongKey(t *testing.T) {
	// Defense in depth. Every caller is supposed to pass a canonical
	// slug; an over-long key means someone started passing raw request
	// input, which must not reach a WITHOUT ROWID table on the shared
	// volume. Dropped rather than truncated: truncation would silently
	// merge distinct buckets.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	r.Increment(usage.KindRegionView, strings.Repeat("a", 129))
	r.Increment(usage.KindRegionView, strings.Repeat("b", 128))
	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}

	got := store.flushed()
	if len(got) != 1 {
		t.Fatalf("want only the in-bounds key, got %+v", got)
	}
	if got[0].Key != strings.Repeat("b", 128) {
		t.Errorf("kept the wrong key: %q", got[0].Key)
	}
}

func TestRecorder_CapsDistinctKeys(t *testing.T) {
	// The buffer must not grow without bound between flushes. Existing
	// buckets keep counting past the cap (a bump costs no memory); only
	// new keys are refused.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	const over = 10500
	for i := range over {
		r.Increment(usage.KindRegionView, "slug-"+strconv.Itoa(i))
	}
	// A key buffered before the cap was hit still accumulates.
	r.Increment(usage.KindRegionView, "slug-0")

	if err := r.Flush(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got := store.flushed()
	if len(got) != 10000 {
		t.Errorf("buffer should cap at 10000 distinct keys, got %d", len(got))
	}
	for _, c := range got {
		if c.Key == "slug-0" && c.Count != 2 {
			t.Errorf("existing bucket should still accumulate past the cap, got %+v", c)
		}
	}
}

func TestRecorder_WaitJoinsRunGoroutine(t *testing.T) {
	// Wait must mean what coverage.Recorder.Wait means: when it returns,
	// no further write will be issued, so the caller may close the store.
	// Run does its own final flush on cancellation using a DETACHED
	// context, so without the join that flush could still be in flight.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)
	r.SetClock(fixedClock("2026-09-01"))

	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	r.Increment(usage.KindRegionView, "chicago")
	cancel()

	if err := r.Wait(context.Background()); err != nil {
		t.Fatalf("wait: %v", err)
	}
	// The Run goroutine has exited, so this count is final: no late
	// flush can append to it after Wait returned.
	if got := store.flushed(); len(got) != 1 || got[0].Count != 1 {
		t.Errorf("want exactly one flushed count after Wait, got %+v", got)
	}
}

func TestRecorder_WaitWithoutStartReturnsImmediately(t *testing.T) {
	// Nothing was started, so the join must not block until the deadline.
	store := &fakeStore{}
	r := usage.New(store, time.Hour, 400, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Wait(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("wait: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked despite Run never being started")
	}
}
