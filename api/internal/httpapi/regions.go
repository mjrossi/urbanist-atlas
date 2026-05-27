package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
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
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}
		respondCollection(w, toOAPIRegionSummaries(regions))
	}
}

// getRegionHandler answers GET /api/v1/regions/{slug}. Resolves any
// non-national region — metros, cities, counties, boroughs, states,
// multi-state coalitions. Returns 404 with a problem+json document
// for unknown slugs and for national-tier slugs (Store.GetRegion
// signals both with (nil, nil)).
func getRegionHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		detail, err := store.GetRegion(r.Context(), slug)
		if err != nil {
			logger.ErrorContext(r.Context(), "get region failed", "err", err, "slug", slug, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}
		if detail == nil {
			writeProblem(w, r, http.StatusNotFound, problemNotFound, "Not Found",
				"no region with that slug", rid)
			return
		}
		writeJSON(w, http.StatusOK, toOAPIRegionDetail(*detail))
	}
}

// toOAPIRegionSummaries converts the domain-level region list to the
// wire-level slice. Returns a non-nil zero-length slice when the input
// is empty so the JSON body is `[]`, not `null`.
func toOAPIRegionSummaries(in []atlas.RegionSummary) []oapi.RegionSummary {
	out := make([]oapi.RegionSummary, 0, len(in))
	for _, rs := range in {
		out = append(out, oapi.RegionSummary{
			Region:   toOAPIRegion(rs.Region),
			OrgCount: int32(rs.OrgCount),
		})
	}
	return out
}

// toOAPIRegionDetail converts a single domain region to the wire
// shape. Orgs are mapped via toOAPIOrgs (shared with /recent; the
// /lookup endpoint uses toOAPILookupOrgs which extends the same
// base). See oapi_adapters.go.
func toOAPIRegionDetail(in atlas.RegionDetail) oapi.RegionDetail {
	return oapi.RegionDetail{
		Region: toOAPIRegion(in.Region),
		Orgs:   toOAPIOrgs(in.Orgs),
	}
}
