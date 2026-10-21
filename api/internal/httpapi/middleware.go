package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// timeoutMiddleware gives each request a context deadline. It is not
// http.TimeoutHandler because that answers with a plain-text 503, not
// problem+json.
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

// requestIDMiddleware puts a request ID in the context and the
// X-Request-ID response header: the inbound header when validRequestID
// accepts it, else a fresh random one. The value is echoed and logged
// verbatim, so arbitrary input would allow log and header injection.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-ID")
		if !validRequestID(rid) {
			rid = newRequestID()
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// maxRequestIDLen fits a UUID or hex token while bounding what is logged.
const maxRequestIDLen = 64

// validRequestID reports whether an inbound request ID is safe to echo
// and log: 1 to maxRequestIDLen characters from [A-Za-z0-9._-].
func validRequestID(s string) bool {
	if s == "" || len(s) > maxRequestIDLen {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z',
			c >= 'a' && c <= 'z',
			c >= '0' && c <= '9',
			c == '.', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
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

// recovererMiddleware turns a panic into a logged 500 problem document.
func recovererMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// http.ErrAbortHandler asks net/http to drop the
					// connection, so it must propagate.
					//nolint:errorlint // rec is a recovered panic value (any), not a wrapped error chain — an identity comparison to the sentinel is what we want.
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

// Health-check paths, shared by the router and the access-log filter.
const (
	healthzPath = "/healthz"
	readyzPath  = "/readyz"
)

func isHealthCheckPath(p string) bool {
	return p == healthzPath || p == readyzPath
}

// loggingMiddleware emits one access-log line per request. Successful
// probes log at DEBUG so frequent polling doesn't drown the log; a failing
// probe stays at INFO. A degraded /readyz is a 200, so readyHandler logs it.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			level := slog.LevelInfo
			if sw.status < 400 && isHealthCheckPath(r.URL.Path) {
				level = slog.LevelDebug
			}
			logger.Log(r.Context(), level, "http",
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

// statusRecorder captures the response status and byte count.
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

// Flush passes through when the underlying writer supports it. A flush
// before WriteHeader sends 200, which is already the recorded default.
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

var _ http.Flusher = (*statusRecorder)(nil)
