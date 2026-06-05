package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// quietLogger discards log output so the retry/cache tests don't spam the
// test runner with the downloader's info/warn lines.
func quietLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// noopSleep replaces the backoff seam so retry tests don't spend real
// seconds. Restored via t.Cleanup.
func stubSleep(t *testing.T, fn func(context.Context, time.Duration) error) {
	t.Helper()
	prev := etlSleep
	etlSleep = fn
	t.Cleanup(func() { etlSleep = prev })
}

func TestDownloadSource_CachedSkipsFetch(t *testing.T) {
	body := []byte("region source payload v1")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("GARBAGE — should never be served"))
	}))
	t.Cleanup(srv.Close)

	dst := filepath.Join(t.TempDir(), "list.csv")
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatalf("seed cache file: %v", err)
	}

	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex(body)}
	if err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger()); err != nil {
		t.Fatalf("downloadSource: %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server hits: want 0 (cached skip), got %d", got)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, body) {
		t.Errorf("cache file mutated: got %q", got)
	}
}

func TestDownloadSource_StaleCacheReDownloads(t *testing.T) {
	body := []byte("region source payload v2")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	dst := filepath.Join(t.TempDir(), "list.csv")
	if err := os.WriteFile(dst, []byte("stale bytes"), 0o644); err != nil {
		t.Fatalf("seed stale file: %v", err)
	}

	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex(body)}
	if err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger()); err != nil {
		t.Fatalf("downloadSource: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits: want 1 (stale re-download), got %d", got)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, body) {
		t.Errorf("file not overwritten with fresh bytes: got %q", got)
	}
}

func TestDownloadSource_RetriesOn5xx(t *testing.T) {
	body := []byte("flaky payload")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	stubSleep(t, func(context.Context, time.Duration) error { return nil })

	dst := filepath.Join(t.TempDir(), "list.csv")
	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex(body)}
	if err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger()); err != nil {
		t.Fatalf("downloadSource: %v", err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("server hits: want 3 (2x503 then 200), got %d", got)
	}
}

func TestDownloadSource_RetriesOn429(t *testing.T) {
	body := []byte("rate-limited payload")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	stubSleep(t, func(context.Context, time.Duration) error { return nil })

	dst := filepath.Join(t.TempDir(), "list.csv")
	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex(body)}
	if err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger()); err != nil {
		t.Fatalf("downloadSource: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hits: want 2 (429 then 200), got %d", got)
	}
}

func TestDownloadSource_NoRetryOn404(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	stubSleep(t, func(context.Context, time.Duration) error {
		t.Error("etlSleep called: a 404 must not be retried")
		return nil
	})

	dst := filepath.Join(t.TempDir(), "list.csv")
	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex([]byte("whatever"))}
	if err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger()); err == nil {
		t.Fatal("downloadSource: want error on 404, got nil")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits: want 1 (404 not retried), got %d", got)
	}
}

func TestDownloadSource_NoRetryOnSha256Mismatch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("complete body, wrong checksum"))
	}))
	t.Cleanup(srv.Close)
	stubSleep(t, func(context.Context, time.Duration) error {
		t.Error("etlSleep called: a sha256 mismatch is deterministic, must not be retried")
		return nil
	})

	dst := filepath.Join(t.TempDir(), "list.csv")
	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex([]byte("expected different bytes"))}
	err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger())
	if err == nil {
		t.Fatal("downloadSource: want sha256 mismatch error, got nil")
	}
	if !errorContains(err, "sha256 mismatch") {
		t.Errorf("error: want sha256 mismatch, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits: want 1 (mismatch not retried), got %d", got)
	}
}

func TestDownloadSource_GivesUpAfterMaxAttempts(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	stubSleep(t, func(context.Context, time.Duration) error { return nil })

	dst := filepath.Join(t.TempDir(), "list.csv")
	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex([]byte("x"))}
	if err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger()); err == nil {
		t.Fatal("downloadSource: want error after exhausting retries, got nil")
	}
	if got := hits.Load(); got != etlDownloadMaxAttempts {
		t.Errorf("server hits: want %d (all attempts), got %d", etlDownloadMaxAttempts, got)
	}
}

func TestDownloadSource_BackoffRespectsCtxCancel(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	// Simulate the context being canceled while waiting out the backoff:
	// the seam returns ctx.Err(), which downloadSource must propagate
	// instead of pressing on to the next attempt.
	stubSleep(t, func(context.Context, time.Duration) error { return context.Canceled })

	dst := filepath.Join(t.TempDir(), "list.csv")
	src := etl.SourceDescriptor{Filename: "list.csv", URL: srv.URL, SHA256: sha256Hex([]byte("x"))}
	err := downloadSource(context.Background(), http.DefaultClient, src, dst, quietLogger())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("downloadSource: want context.Canceled, got %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits: want 1 (canceled during first backoff), got %d", got)
	}
}

func errorContains(err error, sub string) bool {
	return err != nil && strings.Contains(err.Error(), sub)
}
