package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// healthHandler answers GET /healthz with a 200 and a tiny plaintext
// body. Used by Fly as a liveness probe — deliberately doesn't touch
// any dependency (DB, etc.) so a downstream outage doesn't cause Fly
// to recycle the machine and lose the recovery window. Readiness is
// /readyz.
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
// reads are served from the in-memory MemStore, so a SQLite problem must
// not take the only machine out of rotation. With submissions disabled
// pingable is nil and the probe always reports ok.
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
