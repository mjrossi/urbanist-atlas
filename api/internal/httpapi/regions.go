package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/coverage"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// listRegionsHandler answers GET /api/v1/regions with the default browse
// set (see Store.ListRegions). It takes no filters until a browse UI
// needs one.
func listRegionsHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		regions, err := store.ListRegions(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "list regions failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		respondCollection(w, toOAPIRegionSummaries(regions))
	}
}

// searchRegionsHandler answers GET /api/v1/regions/search, the region
// type-ahead over the whole non-national graph. A blank `q` is an empty
// result, not a 400, so an empty input box needs no special case.
func searchRegionsHandler(store atlas.Store, logger *slog.Logger, m *Metrics, rec *coverage.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		rawQ := r.URL.Query().Get("q")
		if utf8.RuneCountInString(rawQ) > maxRegionSearchQueryLen {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Query",
				fmt.Sprintf("The q query parameter must be at most %d characters.", maxRegionSearchQueryLen), rid)
			return
		}
		q := strings.TrimSpace(rawQ)
		limit, ok := parseLimitParam(w, r, maxRegionSearchLimit, rid)
		if !ok {
			return
		}
		results, err := store.SearchRegions(r.Context(), q, limit)
		if err != nil {
			logger.ErrorContext(r.Context(), "search regions failed", "err", err, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		m.incRegionSearch(len(q), len(results))
		// A non-blank query that matched nothing is a coverage gap.
		if len(results) == 0 && q != "" {
			rec.RecordEmpty("search", "", q)
		}
		logger.DebugContext(r.Context(), "region search",
			"query_len", len(q), "result_count", len(results), "rid", rid)
		respondCollection(w, toOAPIRegionSearchResults(results))
	}
}

// getRegionHandler answers GET /api/v1/regions/{slug} for any
// non-national region. Unknown and national-tier slugs are a 404
// (atlas.GetRegion returns nil, nil for both).
func getRegionHandler(store atlas.Store, logger *slog.Logger, m *Metrics, u *usage.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		detail, err := atlas.GetRegion(r.Context(), store, slug)
		if err != nil {
			logger.ErrorContext(r.Context(), "get region failed", "err", err, "slug", slug, "rid", rid)
			writeInternalProblem(w, r, rid)
			return
		}
		if detail == nil {
			m.incRegionView(false)
			// Not recorded in usage: slug is the raw path param, so any
			// caller could mint unbounded rows in a long-retention table
			// on the shared volume. Prometheus keeps the hit/miss split.
			logger.DebugContext(r.Context(), "region view", "slug", slug, "found", false, "rid", rid)
			writeProblem(w, r, http.StatusNotFound, problemNotFound, "Region Not Found",
				"We don't have this region in the atlas yet. It may not be indexed, or the link you followed may be out of date.", rid)
			return
		}
		m.incRegionView(true)
		// The canonical slug, not the raw path param, so casing
		// variants collapse into one bucket.
		u.Increment(usage.KindRegionView, detail.Region.Slug)
		logger.DebugContext(r.Context(), "region view", "slug", slug, "found", true, "rid", rid)
		writeJSON(w, http.StatusOK, toOAPIRegionDetail(*detail))
	}
}
