package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// listRegionsHandler answers GET /api/v1/regions — the homepage
// Browse panel. Returns the editorial default browse set (metros +
// cities, per atlas.DefaultBrowseKinds) with ≥1 approved org
// attached directly or via a region-DAG descendant. Ordered by org
// count DESC then name ASC.
//
// The endpoint deliberately ships without filter parameters; the
// right filter axis (taxonomy via `kind`, DAG via `ancestor`, etc.)
// will be designed when a concrete browse UI use case appears.
//
// Business rules (descendant walk, count computation, national-tier
// exclusion) live in pkg/atlas + the SQL; this handler is the thin
// store-to-wire adapter.
func listRegionsHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		regions, err := store.ListRegions(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "list regions failed", "err", err, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error",
				"An unexpected error occurred while handling this request.", rid)
			return
		}
		respondCollection(w, toOAPIRegionSummaries(regions))
	}
}

// getRegionHandler answers GET /api/v1/regions/{slug}. Resolves any
// non-national region — metros, cities, counties, boroughs, states,
// multi-state coalitions. Returns 404 with a problem+json document
// for unknown slugs and for national-tier slugs (atlas.GetRegion
// signals both with (nil, nil)).
func getRegionHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		detail, err := atlas.GetRegion(r.Context(), store, slug)
		if err != nil {
			logger.ErrorContext(r.Context(), "get region failed", "err", err, "slug", slug, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error",
				"An unexpected error occurred while handling this request.", rid)
			return
		}
		if detail == nil {
			writeProblem(w, r, http.StatusNotFound, problemNotFound, "Region Not Found",
				"No region matches that slug.", rid)
			return
		}
		writeJSON(w, http.StatusOK, toOAPIRegionDetail(*detail))
	}
}
