package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/githubpr"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},

		// Case-insensitive.
		{"DEBUG", slog.LevelDebug},
		{"Warn", slog.LevelWarn},
		{"ERROR", slog.LevelError},

		// Surrounding whitespace is trimmed.
		{" debug ", slog.LevelDebug},
		{"\twarn\n", slog.LevelWarn},

		// Empty and unrecognized values fall back to Info.
		{"", slog.LevelInfo},
		{"verbose", slog.LevelInfo},
		{"debg", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseLevel(tt.in); got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty string is nil", "", nil},
		{"single value", "a", []string{"a"}},
		{"two values", "a,b", []string{"a", "b"}},
		{"surrounding whitespace trimmed", " a , b ", []string{"a", "b"}},
		{"tabs and newlines trimmed", "\ta\n,\tb\n", []string{"a", "b"}},
		{"empty entries dropped", "a,,b", []string{"a", "b"}},
		{"trailing comma dropped", "a,b,", []string{"a", "b"}},
		{"leading comma dropped", ",a,b", []string{"a", "b"}},
		{"whitespace-only entries dropped", "a, ,b", []string{"a", "b"}},
		{"only separators yields nil", " , , ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("splitCSV(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitCSV(%q)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestStartPromotionWorker_OutlivesSignalContext: approvals can land
// after SIGTERM while the HTTP server drains in-flight requests. A job
// enqueued in that window must still be processed (here: recorded as
// failed) before Stop returns, not stranded in the buffer.
func TestStartPromotionWorker_OutlivesSignalContext(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusInternalServerError)
	}))
	t.Cleanup(gh.Close)

	var (
		mu        sync.Mutex
		persisted []string
	)
	sigCtx, cancel := context.WithCancel(context.Background())
	worker := startPromotionWorker(sigCtx, githubpr.Config{
		BaseURL: gh.URL,
		Token:   "fake-token",
		Logger:  slog.New(slog.DiscardHandler),
		PersistResult: func(_ context.Context, publicID, _, _ string) error {
			mu.Lock()
			defer mu.Unlock()
			persisted = append(persisted, publicID)
			return nil
		},
	})
	cancel()
	// Long enough for a Run bound to sigCtx to observe the cancel and exit.
	time.Sleep(50 * time.Millisecond)

	sub := atlas.Submission{
		PublicID: "01928200-3344-7000-9abc-000000000001",
		Payload:  atlas.SubmissionPayload{Name: "Late Approval", RegionSlugs: []string{"brooklyn-ny"}},
	}
	if err := worker.Enqueue(context.Background(), sub); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if dropped, err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v (dropped=%v)", err, dropped)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(persisted) != 1 || persisted[0] != sub.PublicID {
		t.Fatalf("persisted = %v, want [%s]: job enqueued after the signal was lost", persisted, sub.PublicID)
	}
}

type fakeCountStore struct {
	mu      sync.Mutex
	upserts int
}

func (f *fakeCountStore) UpsertUsageCounts(context.Context, []atlas.UsageCount) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upserts++
	return nil
}

func (f *fakeCountStore) PruneUsage(context.Context, string) error { return nil }

// TestAwaitShutdown_TeardownRunsWhenHTTPShutdownFails: a request that
// outlasts the HTTP drain budget makes srv.Shutdown fail. The worker
// drain and usage flush must still run, or the deferred store Close
// lands under their in-flight writes.
func TestAwaitShutdown_TeardownRunsWhenHTTPShutdownFails(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	srv := &http.Server{
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			once.Do(func() { close(entered) })
			<-release
		}),
		ReadHeaderTimeout: time.Second,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serverErr := make(chan error, 1)
	go func() {
		err := srv.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErr <- err
	}()
	t.Cleanup(func() {
		close(release)
		_ = srv.Close()
	})
	go func() {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+ln.Addr().String(), nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered

	logger := slog.New(slog.DiscardHandler)
	worker := githubpr.New(githubpr.Config{Token: "fake-token", Logger: logger})
	go worker.Run(context.Background())
	counts := &fakeCountStore{}
	usageRec := usage.New(counts, time.Hour, 0, logger)
	usageRec.Increment("lookup", "11201")

	sigCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = awaitShutdown(sigCtx, logger, shutdownDeps{
		srv:             srv,
		serverErr:       serverErr,
		worker:          worker,
		usageRec:        usageRec,
		httpTimeout:     50 * time.Millisecond,
		teardownTimeout: 2 * time.Second,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("awaitShutdown err = %v, want it to wrap the HTTP drain's DeadlineExceeded", err)
	}
	if err := worker.Enqueue(context.Background(), atlas.Submission{PublicID: "x"}); !errors.Is(err, githubpr.ErrStopped) {
		t.Errorf("worker still accepting jobs (Enqueue err = %v): Stop was skipped", err)
	}
	counts.mu.Lock()
	defer counts.mu.Unlock()
	if counts.upserts == 0 {
		t.Error("usage counts never flushed: usageRec.Wait was skipped")
	}
}
