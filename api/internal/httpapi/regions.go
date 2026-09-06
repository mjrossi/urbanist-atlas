package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/coverage"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// listRegionsHandler answers GET /api/v1/regions — the homepage
// Browse panel. Returns the editorial default browse set (metros +
// cities, per atlas.IsDefaultBrowseKind) with ≥1 approved org
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
			writeInternalProblem(w, r, rid)
			return
		}
		respondCollection(w, toOAPIRegionSummaries(regions))
	}
}

// searchRegionsHandler answers GET /api/v1/regions/search — the region
// type-ahead behind the public submission form. Case-insensitive search
// over the FULL non-national region graph (every kind, not just the
// browse set ListRegions returns), ranked for relevance, each result
// carrying a state-ancestor context label for disambiguation. A blank
// `q` returns an empty `data` array (not a 400) so the SPA's
// empty-input state needs no special handling.
//
// Ranking, the national-tier exclusion, and the context label live in
// pkg/atlas (MemStore.SearchRegions); this handler is the thin
// parse-validate-encode adapter.
func searchRegionsHandler(store atlas.Store, logger *slog.Logger, m *Metrics, rec *coverage.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		q := strings.TrimSpace(r.URL.Query().Get("q"))
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
		// A non-blank query that matched nothing is a coverage gap worth
		// sampling. Skip blank queries (the SPA's empty-input state).
		if len(results) == 0 && q != "" {
			rec.RecordEmpty("search", "", q)
		}
		logger.DebugContext(r.Context(), "region search",
			"query_len", len(q), "result_count", len(results), "rid", rid)
		respondCollection(w, toOAPIRegionSearchResults(results))
	}
}

// getRegionHandler answers GET /api/v1/regions/{slug}. Resolves any
// non-national region — metros, cities, counties, boroughs, states,
// multi-state coalitions. Returns 404 with a problem+json document
// for unknown slugs and for national-tier slugs (atlas.GetRegion
// signals both with (nil, nil)).
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
			// Deliberately NOT bucketed into usage_daily. slug here is
			// the raw path param, so recording it would let any caller
			// mint unbounded rows in a 400-day table that shares the
			// submission volume — and an unresolved slug is not content
			// popularity in the first place. The hit/miss split lives in
			// Prometheus (incRegionView); raw misses land in
			// coverage_gaps, which is sampled and row-capped precisely
			// because it holds user input.
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
