package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// listMetrosHandler answers GET /api/v1/metros — the homepage Browse
// panel. It returns every metro-equivalent region with at least one
// approved org, ordered by org count DESC then name ASC. The business
// rules (which kinds count as metro-equivalent, descendant walk, count
// computation) live in pkg/atlas + the SQL; this handler is a thin
// adapter.
func listMetrosHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		metros, err := store.ListMetros(r.Context())
		if err != nil {
			logger.ErrorContext(r.Context(), "list metros failed", "err", err, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}
		respondCollection(w, toOAPIMetroSummaries(metros))
	}
}

// getMetroHandler answers GET /api/v1/metros/{slug}. Returns 404 with a
// problem+json document for unknown slugs and for slugs that exist as
// a region but aren't metro-equivalent (e.g. a state slug). Store.
// GetMetro signals both conditions with (nil, nil).
func getMetroHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		slug := strings.TrimSpace(chi.URLParam(r, "slug"))
		detail, err := store.GetMetro(r.Context(), slug)
		if err != nil {
			logger.ErrorContext(r.Context(), "get metro failed", "err", err, "slug", slug, "rid", rid)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}
		if detail == nil {
			writeProblem(w, r, http.StatusNotFound, problemNotFound, "Not Found",
				"no metro with that slug", rid)
			return
		}
		writeJSON(w, http.StatusOK, toOAPIMetroDetail(*detail))
	}
}

// toOAPIMetroSummaries converts the domain-level metro list to the
// wire-level slice. Returns a non-nil zero-length slice when the input
// is empty so the JSON body is `[]`, not `null`.
func toOAPIMetroSummaries(in []atlas.MetroSummary) []oapi.MetroSummary {
	out := make([]oapi.MetroSummary, 0, len(in))
	for _, m := range in {
		out = append(out, oapi.MetroSummary{
			Region:   toOAPIRegion(m.Region),
			OrgCount: int32(m.OrgCount),
		})
	}
	return out
}

// toOAPIMetroDetail converts a single domain metro to the wire shape.
// Orgs are mapped via toOAPIOrgs (shared with /recent; the /lookup
// endpoint uses toOAPILookupOrgs which extends the same base). See
// oapi_adapters.go.
func toOAPIMetroDetail(in atlas.MetroDetail) oapi.MetroDetail {
	return oapi.MetroDetail{
		Region: toOAPIRegion(in.Region),
		Orgs:   toOAPIOrgs(in.Orgs),
	}
}
