package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// lookupHandler answers GET /api/v1/lookup?postal_code=…&country=….
//
// The response body is the generated oapi.LookupResult (so the wire
// types stay in lockstep with `api/openapi.yaml`). Business logic and
// the source-of-truth Go types live in pkg/atlas; this handler only
// parses, calls into atlas.Lookup, and adapts the result onto the
// OpenAPI shape.
//
// Error responses are RFC 9457 problem documents (see problem.go).
func lookupHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		rawPostal := strings.TrimSpace(r.URL.Query().Get("postal_code"))
		country := atlas.Country(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country"))))

		if rawPostal == "" {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request", "postal_code is required", rid)
			return
		}
		if country == "" {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Bad Request", "country is required", rid)
			return
		}
		// Country is an opaque string per pkg/atlas/atlas.go; the handler
		// doesn't gate on a known-country list. Unknown countries fall
		// through to atlas.Lookup which returns ErrPostalCodeNotFound
		// (→ 404) when no matching postal code exists.

		// Canonicalize once at the boundary so logs, error details, and
		// downstream Store calls all see the same form. Both Store
		// implementations re-normalize internally; that second call is
		// idempotent.
		postal := atlas.NormalizePostalCode(country, rawPostal)

		result, err := atlas.Lookup(r.Context(), store, atlas.LookupQuery{
			PostalCode: postal,
			Country:    country,
		})
		if err != nil {
			if errors.Is(err, atlas.ErrPostalCodeNotFound) {
				writeProblem(w, r, http.StatusNotFound, problemNotFound, "Not Found",
					"postal code not found — try a nearby code, or submit an organization for your area", rid)
				return
			}
			logger.ErrorContext(r.Context(), "lookup failed",
				"err", err,
				"postal_code", postal,
				"country", country,
				"rid", rid,
			)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error", "internal error", rid)
			return
		}

		writeJSON(w, http.StatusOK, toOAPILookupResult(result))
	}
}

// toOAPILookupResult adapts the atlas package's native result type
// onto the oapi-generated wire type. The JSON shapes are identical;
// this adapter is a typed conversion so the handler signature is
// "returns oapi.LookupResult", which keeps the wire contract front and
// center in code review.
func toOAPILookupResult(in atlas.LookupResult) oapi.LookupResult {
	ancestry := make([]oapi.Region, 0, len(in.ResolvedAncestry))
	for _, r := range in.ResolvedAncestry {
		ancestry = append(ancestry, toOAPIRegion(r))
	}
	return oapi.LookupResult{
		Query: oapi.LookupQuery{
			PostalCode: in.Query.PostalCode,
			Country:    oapi.Country(in.Query.Country),
		},
		ResolvedPlaceLabel: in.ResolvedPlaceLabel,
		ResolvedAncestry:   ancestry,
		Local:              toOAPILookupOrgs(in.Local),
		Regional:           toOAPILookupOrgs(in.Regional),
	}
}

func toOAPILookupOrgs(orgs []atlas.Org) []oapi.LookupOrg {
	out := make([]oapi.LookupOrg, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, toOAPILookupOrg(o))
	}
	return out
}

// toOAPILookupOrg builds the /lookup-specific wire shape on top of the
// shared toOAPIOrg adapter (oapi_adapters.go). The only lookup-specific
// field is MatchedRegionSlugs, which the lookup algorithm in pkg/atlas
// computes per org.
func toOAPILookupOrg(o atlas.Org) oapi.LookupOrg {
	base := toOAPIOrg(o)
	matched := o.MatchedRegionSlugs
	if matched == nil {
		matched = []string{}
	}
	return oapi.LookupOrg{
		Id:                 base.Id,
		Slug:               base.Slug,
		Name:               base.Name,
		ShortDesc:          base.ShortDesc,
		WebsiteUrl:         base.WebsiteUrl,
		Tags:               base.Tags,
		Regions:            base.Regions,
		ContactUrl:         base.ContactUrl,
		MatchedRegionSlugs: matched,
	}
}

