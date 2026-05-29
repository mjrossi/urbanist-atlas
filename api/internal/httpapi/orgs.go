package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// getOrgHandler answers GET /api/v1/orgs/{slug}. Returns 404 with a
// problem+json document for unknown or non-approved slugs. The handler
// stays thin: parse → call store → encode. Hydration of Org.Regions
// happens in the Store layer; the wire-shape adapter is the same
// toOAPIOrg used by /regions/{slug} and /recent.
func getOrgHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		org, err := store.GetOrgBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, atlas.ErrOrgNotFound) {
				writeProblem(w, r, http.StatusNotFound, problemNotFound, "Organization Not Found",
					"No organization matches that slug.", rid)
				return
			}
			logger.ErrorContext(r.Context(), "get org failed", "err", err, "slug", slug, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error",
				"An unexpected error occurred while handling this request.", rid)
			return
		}
		if org == nil {
			logger.ErrorContext(r.Context(), "store contract violation: nil org with nil err", "slug", slug, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error",
				"An unexpected error occurred while handling this request.", rid)
			return
		}
		writeJSON(w, http.StatusOK, toOAPIOrg(*org))
	}
}
