package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
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

// TestReadyz_WithoutPingerReturnsOk pins the contract: a store that
// doesn't implement pinger collapses readiness to "200 ok". The
// production file-backed MemStore has no downstream to ping, so
// /readyz behaves the same as in this test; the pinger hook stays in
// place for a future network-bound store (see readyHandler).
func TestReadyz_WithoutPingerReturnsOk(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	h := readyHandler(struct{}{}, logger) // not a pinger

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
	h := readyHandler(fakePinger{}, logger)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status: want 200, got %d", rec.Code)
	}
}

func TestReadyz_PingerFails_ReturnsProblem503(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	h := readyHandler(fakePinger{err: errors.New("dial tcp: connection refused")}, logger)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type: want application/problem+json, got %q", ct)
	}

	var problem oapi.ProblemDetails
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if want := "https://urbanistatlas.com/problems/not-ready"; problem.Type != want {
		t.Errorf("problem.type: want %q, got %q", want, problem.Type)
	}
	if problem.Status != http.StatusServiceUnavailable {
		t.Errorf("problem.status: want 503, got %d", problem.Status)
	}
}
