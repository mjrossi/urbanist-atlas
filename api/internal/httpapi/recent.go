package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// recentHandler answers GET /api/v1/recent with the newest orgs (see
// Store.ListRecent). The spec declares no `limit`, so the cap is fixed.
func recentHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		orgs, err := store.ListRecent(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "list recent failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		respondCollection(w, toOAPIOrgs(orgs))
	}
}
