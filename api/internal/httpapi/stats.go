package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// statsHandler answers GET /api/v1/stats, the atlas-wide counts.
// /api/v1/regions is only the browse subset, so summing it undercounts
// orgs attached solely to states, boroughs or multi-state regions.
func statsHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		stats, err := store.Stats(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "stats failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		writeJSON(w, http.StatusOK, toOAPIStats(stats))
	}
}
