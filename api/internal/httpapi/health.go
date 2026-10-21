package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// healthHandler answers GET /healthz, the liveness probe. It touches no
// dependency, so a store outage never fails liveness and restarts the
// process. Readiness is /readyz.
func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}

// pinger is implemented by the SQLite submission store. Defined here
// rather than in pkg/atlas because only the readiness probe needs it.
type pinger interface {
	Ping(ctx context.Context) error
}

// readyHandler answers GET /readyz with 200 and `ok`, or `degraded` when
// the submission store fails a 1-second ping. It never answers 503:
// reads are served from memory, so a SQLite problem must not mark the
// server unready. With submissions disabled the probe always reports ok.
func readyHandler(pingable any, logger *slog.Logger, m *Metrics) http.HandlerFunc {
	p, _ := pingable.(pinger)
	return func(w http.ResponseWriter, r *http.Request) {
		body := "ok\n"
		if p != nil {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := p.Ping(ctx); err != nil {
				m.incStorePingFailure()
				logger.WarnContext(r.Context(), "readyz: store ping failed", "err", err)
				body = "degraded\n"
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}
