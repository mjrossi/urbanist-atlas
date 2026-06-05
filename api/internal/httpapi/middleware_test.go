package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

// --- requestIDMiddleware ---

// TestRequestID_GeneratesWhenAbsent: with no inbound X-Request-ID, the
// middleware fabricates a hex string, echoes it in the response
// header, and threads it through the request context so handlers can
// quote it in error envelopes.
func TestRequestID_GeneratesWhenAbsent(t *testing.T) {
	var ridFromCtx string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ridFromCtx = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := requestIDMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	respRID := rec.Header().Get("X-Request-ID")
	if respRID == "" {
		t.Fatal("expected X-Request-ID response header to be set")
	}
	if ridFromCtx == "" {
		t.Fatal("expected request ID in context")
	}
	if respRID != ridFromCtx {
		t.Errorf("response rid %q != context rid %q (must match)", respRID, ridFromCtx)
	}
}

// TestRequestID_EchoesInbound: a request that already carries
// X-Request-ID gets that exact value echoed back and threaded into the
// context. Clients can correlate logs across hops by setting their own
// request id.
func TestRequestID_EchoesInbound(t *testing.T) {
	const inbound = "client-supplied-rid-1234"
	var ridFromCtx string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ridFromCtx = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := requestIDMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", inbound)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != inbound {
		t.Errorf("response rid: got %q, want %q (must echo inbound)", got, inbound)
	}
	if ridFromCtx != inbound {
		t.Errorf("context rid: got %q, want %q", ridFromCtx, inbound)
	}
}

// TestRequestIDFromContext_NoValue documents the helper's fallback for
// callers that build a context without going through the middleware
// (e.g. background workers or older tests). Returns "" rather than
// panicking on a missing key.
func TestRequestIDFromContext_NoValue(t *testing.T) {
	if got := requestIDFromContext(context.Background()); got != "" {
		t.Errorf("expected empty string from bare context, got %q", got)
	}
}

// --- recovererMiddleware ---

// TestRecoverer_PanicProducesProblemJSON: a handler panic becomes a
// structured 500 with application/problem+json content type — the
// process keeps running and the client gets a parseable envelope.
func TestRecoverer_PanicProducesProblemJSON(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	handler := recovererMiddleware(logger)(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	// The middleware must absorb the panic. If it escapes, the test
	// itself crashes — there's no need for a recover() here.
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/problem+json")
	}
	var body oapi.ProblemDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Type != problemInternal {
		t.Errorf("problem type: got %q, want %q", body.Type, problemInternal)
	}
	if body.Status != http.StatusInternalServerError {
		t.Errorf("problem status: got %d, want %d", body.Status, http.StatusInternalServerError)
	}
}

// TestRecoverer_PanicLogsRequestID: the panic log line carries the
// request id so an oncall reading server logs can correlate with the
// X-Request-ID the user quoted.
func TestRecoverer_PanicLogsRequestID(t *testing.T) {
	const inbound = "rid-for-recoverer-test"
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})

	// Chain requestID → recoverer so the panic handler has a rid in
	// context to log.
	handler := requestIDMiddleware(recovererMiddleware(logger)(next))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	req.Header.Set("X-Request-ID", inbound)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), inbound) {
		t.Errorf("panic log line did not include rid %q; log was:\n%s", inbound, buf.String())
	}
}

// TestRecoverer_AbortHandlerRepanics: http.ErrAbortHandler is the
// documented "I handled the response, drop the connection" signal.
// The middleware must re-panic with it so net/http does its normal
// thing rather than masking it as a 500.
func TestRecoverer_AbortHandlerRepanics(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})

	handler := recovererMiddleware(logger)(next)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected http.ErrAbortHandler to re-panic out of the middleware")
		}
		//nolint:errorlint // rec is a recovered panic value (any), not a wrapped error chain — identity comparison to the sentinel is intended.
		if rec != http.ErrAbortHandler {
			t.Errorf("re-panic value: got %v, want http.ErrAbortHandler", rec)
		}
	}()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

// --- loggingMiddleware ---

// TestLogging_EmitsAccessLogLine: every request produces one structured
// log record carrying method, path, and status. The exact wording is
// covered by the slog handler; this test just confirms a line is
// emitted and has the load-bearing fields.
func TestLogging_EmitsAccessLogLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "ok")
	})
	handler := loggingMiddleware(logger)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	line := buf.String()
	for _, want := range []string{`"method":"GET"`, `"path":"/api/v1/lookup"`, `"status":202`} {
		if !strings.Contains(line, want) {
			t.Errorf("access log missing %s; full line:\n%s", want, line)
		}
	}
}

// captureHandler is a slog.Handler that buffers structured records for
// test inspection. Concurrency-safe so chained-middleware tests don't
// trip the race detector.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// TestLogging_RecordsStatusFromHandler verifies the access-log status
// reflects whatever the downstream handler wrote — not the default
// 200 that statusRecorder seeds. This regression-guards the
// statusRecorder wrap.
func TestLogging_RecordsStatusFromHandler(t *testing.T) {
	h := &captureHandler{}
	logger := slog.New(h)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	handler := loggingMiddleware(logger)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if len(h.records) != 1 {
		t.Fatalf("expected exactly 1 log record, got %d", len(h.records))
	}
	var gotStatus int64
	h.records[0].Attrs(func(a slog.Attr) bool {
		if a.Key == "status" {
			gotStatus = a.Value.Int64()
		}
		return true
	})
	if gotStatus != http.StatusTeapot {
		t.Errorf("logged status: got %d, want %d", gotStatus, http.StatusTeapot)
	}
}

// TestLogging_HealthCheckSuppressedAtInfo: a successful liveness/readiness
// probe is demoted to DEBUG, so at the default INFO level it produces no
// access-log line — that's the whole point of quieting Fly's 15-30s probe
// traffic.
func TestLogging_HealthCheckSuppressedAtInfo(t *testing.T) {
	for _, path := range []string{healthzPath, readyzPath} {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil)) // default level: INFO

		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok\n")
		})
		handler := loggingMiddleware(logger)(next)

		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if got := buf.String(); got != "" {
			t.Errorf("%s 200 at INFO: want no log line, got:\n%s", path, got)
		}
	}
}

// TestLogging_HealthCheckFailureStillLogged: a non-2xx probe (e.g. a
// /readyz 503 when the store ping fails) stays at INFO so the outage is
// visible without raising the log level.
func TestLogging_HealthCheckFailureStillLogged(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil)) // default level: INFO

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := loggingMiddleware(logger)(next)

	req := httptest.NewRequest(http.MethodGet, readyzPath, nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	line := buf.String()
	for _, want := range []string{`"path":"/readyz"`, `"status":503`} {
		if !strings.Contains(line, want) {
			t.Errorf("failing probe log missing %s; full line:\n%s", want, line)
		}
	}
}

// TestLogging_HealthCheckVisibleAtDebug: the suppression is a downgrade,
// not a drop — at DEBUG the successful probe line reappears so operators
// can still inspect probe traffic when they ask for it.
func TestLogging_HealthCheckVisibleAtDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := loggingMiddleware(logger)(next)

	req := httptest.NewRequest(http.MethodGet, healthzPath, nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), `"path":"/healthz"`) {
		t.Errorf("healthz 200 at DEBUG: want log line, got:\n%s", buf.String())
	}
}

// --- statusRecorder ---

// TestStatusRecorder_DefaultStatus: a handler that calls Write without
// WriteHeader implicitly sends 200 OK; the recorder must reflect that
// in its status field so the access log is accurate.
func TestStatusRecorder_DefaultStatus(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	if _, err := sw.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if sw.status != http.StatusOK {
		t.Errorf("status: got %d, want %d", sw.status, http.StatusOK)
	}
	if sw.bytes != 5 {
		t.Errorf("bytes: got %d, want %d", sw.bytes, 5)
	}
}

// TestStatusRecorder_CapturesExplicitStatus: an explicit WriteHeader
// is captured; subsequent Writes accumulate bytes without overwriting
// the recorded status.
func TestStatusRecorder_CapturesExplicitStatus(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	sw.WriteHeader(http.StatusCreated)
	if _, err := sw.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := sw.Write([]byte("defgh")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}

	if sw.status != http.StatusCreated {
		t.Errorf("status: got %d, want %d", sw.status, http.StatusCreated)
	}
	if sw.bytes != 8 {
		t.Errorf("bytes: got %d, want %d (sum of 3 + 5)", sw.bytes, 8)
	}
}

// TestStatusRecorder_FirstWriteHeaderWins: HTTP only allows one
// status to be sent; the recorder pins the first WriteHeader call and
// ignores any subsequent ones (matches net/http's behavior which
// warns about superfluous WriteHeader calls).
func TestStatusRecorder_FirstWriteHeaderWins(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusRecorder{ResponseWriter: inner, status: http.StatusOK}

	sw.WriteHeader(http.StatusCreated)
	sw.WriteHeader(http.StatusTeapot) // ignored — first call wins.

	if sw.status != http.StatusCreated {
		t.Errorf("status: got %d, want %d (first WriteHeader must win)", sw.status, http.StatusCreated)
	}
}

// flushTrackingWriter wraps httptest.ResponseRecorder so the test can
// confirm the statusRecorder.Flush() passthrough actually called the
// underlying Flush.
type flushTrackingWriter struct {
	*httptest.ResponseRecorder
	flushed int
}

func (f *flushTrackingWriter) Flush() {
	f.flushed++
	f.ResponseRecorder.Flush()
}

// TestStatusRecorder_FlushPassthrough confirms statusRecorder.Flush()
// delegates to the wrapped writer's Flusher implementation when one
// exists. Required so SSE / chunked-response handlers don't get a
// silently no-op wrapper.
func TestStatusRecorder_FlushPassthrough(t *testing.T) {
	tracker := &flushTrackingWriter{ResponseRecorder: httptest.NewRecorder()}
	sw := &statusRecorder{ResponseWriter: tracker, status: http.StatusOK}

	sw.Flush()
	sw.Flush()

	if tracker.flushed != 2 {
		t.Errorf("inner Flush called %d times, want 2", tracker.flushed)
	}
}

// nonFlushWriter is the negative-path foil: a ResponseWriter that does
// NOT implement http.Flusher. The recorder's Flush() must be a no-op
// rather than panicking on a failed type assertion.
type nonFlushWriter struct{ http.ResponseWriter }

// TestStatusRecorder_FlushNoopWhenInnerNotFlusher: when the wrapped
// writer doesn't implement http.Flusher, statusRecorder.Flush() is a
// silent no-op (matches the conservative passthrough contract).
func TestStatusRecorder_FlushNoopWhenInnerNotFlusher(t *testing.T) {
	sw := &statusRecorder{ResponseWriter: nonFlushWriter{httptest.NewRecorder()}, status: http.StatusOK}

	// Just must not panic — assertion-free.
	sw.Flush()
}

// TestStatusRecorder_HijackPushReturnNotSupportedFallback covers the
// negative path for both Hijack and Push: when the wrapped writer
// doesn't implement the interface, the recorder returns
// http.ErrNotSupported rather than panicking. (httptest.ResponseRecorder
// implements neither, so it's the natural foil.)
func TestStatusRecorder_HijackPushReturnNotSupportedFallback(t *testing.T) {
	sw := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	if _, _, err := sw.Hijack(); !errors.Is(err, http.ErrNotSupported) {
		t.Errorf("Hijack: got err=%v, want http.ErrNotSupported", err)
	}
	if err := sw.Push("/some-resource", nil); !errors.Is(err, http.ErrNotSupported) {
		t.Errorf("Push: got err=%v, want http.ErrNotSupported", err)
	}
}
