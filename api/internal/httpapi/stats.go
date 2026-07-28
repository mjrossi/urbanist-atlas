package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// statsHandler answers GET /api/v1/stats — the atlas-wide size summary
// behind the SPA's masthead and "by the numbers" panel.
//
// This endpoint exists so no consumer has to derive a catalog size
// from /api/v1/regions. That list is the browseable subset (metros and
// cities), so summing its direct_org_count drops every org attached
// solely to a state, province, borough, or multi-state region. The
// counting rules live in pkg/atlas; this handler is a thin adapter.
//
// Single-resource response, so writeJSON rather than
// respondCollection — there is no `{ meta, data }` envelope. ODbL
// attribution still rides on the headers set by odblHeadersMiddleware.
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
