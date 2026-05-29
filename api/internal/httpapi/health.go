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

// pinger is the (optional) interface the store implements when it
// can be health-pinged. Historically backed by the Postgres adapter
// (since retired); kept on the read-side Store interface so a future
// downstream-bound store implementation can opt in. The bundled
// in-memory FileStore does not implement it. Defined here (not in
// pkg/atlas) because the contract exists for the HTTP readiness
// layer's benefit, not for orchestrators in pkg/atlas.
type pinger interface {
	Ping(ctx context.Context) error
}

// readyHandler answers GET /readyz with 200 only when the store's
// downstream dependency is reachable. Returns 503 with an RFC 9457
// problem document otherwise. The 1-second deadline keeps Fly's
// readiness check predictable; a store that takes longer than 1s to
// acknowledge a ping is effectively unavailable for handling a burst
// of /lookup requests.
//
// In the current deployment the file-backed in-memory FileStore does
// not implement pinger, so /readyz collapses to "200 ok" — same
// shape as /healthz. The hook stays in place for a future
// network-bound store.
func readyHandler(store any, logger *slog.Logger) http.HandlerFunc {
	p, _ := store.(pinger)
	return func(w http.ResponseWriter, r *http.Request) {
		if p != nil {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if err := p.Ping(ctx); err != nil {
				logger.WarnContext(r.Context(), "readyz: store ping failed",
					slog.String("error", err.Error()))
				writeProblem(w, r, http.StatusServiceUnavailable,
					"https://urbanistatlas.com/problems/not-ready",
					"Service not ready",
					"The data store is not currently reachable; try again shortly.",
					requestIDFromContext(r.Context()))
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}
