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
// Orgs are mapped via toOAPIOrgs (the same hydration used by /recent
// and shared with the LookupOrg adapter where applicable).
func toOAPIMetroDetail(in atlas.MetroDetail) oapi.MetroDetail {
	return oapi.MetroDetail{
		Region: toOAPIRegion(in.Region),
		Orgs:   toOAPIOrgs(in.Orgs),
	}
}

// toOAPIOrgs adapts a slice of domain Orgs to the wire shape. The base
// /lookup endpoint uses oapi.LookupOrg (with matched_region_slugs);
// /metros/{slug} and /recent use the plain oapi.Org without that
// extension.
func toOAPIOrgs(in []atlas.Org) []oapi.Org {
	out := make([]oapi.Org, 0, len(in))
	for _, o := range in {
		out = append(out, toOAPIOrg(o))
	}
	return out
}

func toOAPIOrg(o atlas.Org) oapi.Org {
	tags := make([]string, len(o.Tags))
	for i, t := range o.Tags {
		tags[i] = string(t)
	}
	regions := make([]oapi.Region, 0, len(o.Regions))
	for _, r := range o.Regions {
		regions = append(regions, toOAPIRegion(r))
	}
	out := oapi.Org{
		Id:         o.ID,
		Slug:       o.Slug,
		Name:       o.Name,
		ShortDesc:  o.ShortDesc,
		WebsiteUrl: o.WebsiteURL,
		Tags:       tags,
		Regions:    regions,
	}
	if o.ContactURL != "" {
		cu := o.ContactURL
		out.ContactUrl = &cu
	}
	return out
}
