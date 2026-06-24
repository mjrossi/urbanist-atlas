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
func getOrgHandler(store atlas.Store, logger *slog.Logger, m *Metrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		org, err := store.GetOrgBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, atlas.ErrOrgNotFound) {
				m.incOrgView(false)
				logger.DebugContext(r.Context(), "org view", "slug", slug, "found", false, "rid", rid)
				writeProblem(w, r, http.StatusNotFound, problemNotFound, "Organization Not Found",
					"We don't have this organization in the atlas yet. It may not be indexed, or the link you followed may be out of date.", rid)
				return
			}
			logger.ErrorContext(r.Context(), "get org failed", "err", err, "slug", slug, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		if org == nil {
			logger.ErrorContext(r.Context(), "store contract violation: nil org with nil err", "slug", slug, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		m.incOrgView(true)
		logger.DebugContext(r.Context(), "org view", "slug", slug, "found", true, "rid", rid)
		writeJSON(w, http.StatusOK, toOAPIOrg(*org))
	}
}
