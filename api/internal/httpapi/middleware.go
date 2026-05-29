package httpapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// timeoutMiddleware bounds a single request's total processing time
// by attaching a context deadline. Handlers (and the pkg/atlas /
// SQLite calls they forward into) receive a cancellable ctx; when the
// deadline expires the downstream sees ctx.Done() and can abort
// cleanly. Coarser ReadTimeout/WriteTimeout on http.Server still
// apply at the transport layer; this is the per-request budget the
// application code can react to.
//
// We do NOT use http.TimeoutHandler. That helper races the handler
// against a timer and writes a 503 from a separate goroutine, which
// (a) doesn't propagate cancellation to the original handler — it
// keeps running until it notices — and (b) interferes with the
// problem+json response shape this API standardizes on. Cancelling
// the request context is the cleaner contract: handlers that already
// forward ctx into pkg/atlas naturally inherit the deadline, and
// nothing has to know the timeout exists.
func timeoutMiddleware(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type ctxKey string

const requestIDKey ctxKey = "rid"

// requestIDMiddleware attaches a request ID to every request: the
// incoming X-Request-ID header if present, otherwise a fresh random
// hex string. The ID is echoed back in the response header and
// stuffed into the request context for downstream use.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if rid == "" {
			rid = newRequestID()
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requestIDFromContext returns the request ID stored in the context
// by requestIDMiddleware, or "" if none.
func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should not fail; if it does we still want some
		// identifier rather than a fatal — fall back to a timestamp.
		return time.Now().UTC().Format("20060102T150405.000000")
	}
	return hex.EncodeToString(b[:])
}

// recovererMiddleware turns panics into 500s without taking the
// process down. The panic is logged with the request ID and path,
// and the client receives an RFC 9457 problem document so error
// responses are uniformly machine-readable.
func recovererMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// http.ErrAbortHandler is the documented "I handled
					// this, drop the connection" signal; re-panic so
					// net/http does its thing.
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					rid := requestIDFromContext(r.Context())
					logger.ErrorContext(r.Context(), "panic",
						"err", rec,
						"path", r.URL.Path,
						"method", r.Method,
						"rid", rid,
					)
					writeInternalProblem(w, r, rid)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// loggingMiddleware emits one structured access-log line per request.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.InfoContext(r.Context(), "http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"rid", requestIDFromContext(r.Context()),
			)
		})
	}
}

// statusRecorder wraps http.ResponseWriter so the access-log
// middleware can report the response status and byte count without
// peeking at internal fields.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	bytes   int
	written bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.written {
		s.status = code
		s.written = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.written = true
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Flush implements http.Flusher when the underlying writer does. The
// recorder stays transparent to streaming handlers (SSE, chunked
// responses); status capture is unaffected because Flush goes through
// Write/WriteHeader first.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker so connection-upgrade handlers
// (websockets, long-poll) can take ownership of the conn through the
// middleware chain. Status tracking ends at hijack — the caller owns
// the conn from that point.
func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Push implements http.Pusher so HTTP/2 server-push is propagated
// through the middleware chain when the underlying writer supports it.
func (s *statusRecorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := s.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// Compile-time interface satisfaction. Mounting these as static
// assertions means a future refactor that drops one of the methods
// fails to build, not just regress at runtime.
var (
	_ http.Flusher  = (*statusRecorder)(nil)
	_ http.Hijacker = (*statusRecorder)(nil)
	_ http.Pusher   = (*statusRecorder)(nil)
)
