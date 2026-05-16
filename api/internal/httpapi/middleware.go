package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

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
// process down. The panic is logged with the request ID and path.
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
					logger.ErrorContext(r.Context(), "panic",
						"err", rec,
						"path", r.URL.Path,
						"method", r.Method,
						"rid", requestIDFromContext(r.Context()),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
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
