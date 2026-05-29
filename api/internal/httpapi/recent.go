package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// recentHandler answers GET /api/v1/recent — the homepage "Recently
// added" strip. Returns the 10 most-recently-approved organizations
// newest-first, with national-tier-only orgs filtered out. The cap,
// ordering, and filter live in pkg/atlas + the SQL; this handler is a
// thin adapter.
//
// The OpenAPI contract for this endpoint does not declare a `limit`
// query parameter; the cap is intentionally fixed to keep the response
// shape stable for downstream consumers. A future spec edit can open
// it up.
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
