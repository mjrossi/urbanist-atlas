package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// getOrgHandler answers GET /api/v1/orgs/{slug}, with a 404 for an
// unknown slug.
func getOrgHandler(store atlas.Store, logger *slog.Logger, m *Metrics, u *usage.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		org, err := store.GetOrgBySlug(r.Context(), slug)
		if err != nil {
			if errors.Is(err, atlas.ErrOrgNotFound) {
				m.incOrgView(false)
				// Not recorded in usage, as in getRegionHandler.
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
		// The canonical slug, not the raw path param, so casing
		// variants collapse into one bucket.
		u.Increment(usage.KindOrgView, org.Slug)
		logger.DebugContext(r.Context(), "org view", "slug", slug, "found", true, "rid", rid)
		writeJSON(w, http.StatusOK, toOAPIOrg(*org))
	}
}
