package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz_ReturnsPlainOk(t *testing.T) {
	rec := httptest.NewRecorder()
	healthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Errorf("body: want %q, got %q", "ok", got)
	}
}

// TestReadyz_WithoutPingerReturnsOk pins the submissions-disabled case:
// with no pingable store, readiness is the same as liveness.
func TestReadyz_WithoutPingerReturnsOk(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	h := readyHandler(struct{}{}, logger, nil) // not a pinger

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "ok" {
		t.Errorf("body: want %q, got %q", "ok", got)
	}
}

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestReadyz_PingerOk(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	h := readyHandler(fakePinger{}, logger, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "ok\n" {
		t.Errorf("body: want %q, got %q", "ok\n", got)
	}
}

// TestReadyz_PingerFails_ReturnsDegraded200 pins that a failed store
// ping never takes the machine out of rotation: reads are served from
// memory, so only the body reports the write path is down.
func TestReadyz_PingerFails_ReturnsDegraded200(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	h := readyHandler(fakePinger{err: errors.New("dial tcp: connection refused")}, logger, nil)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("content-type: want text/plain; charset=utf-8, got %q", ct)
	}
	if got := rec.Body.String(); got != "degraded\n" {
		t.Errorf("body: want %q, got %q", "degraded\n", got)
	}
}
