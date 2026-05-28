package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

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
		rawPostalIn := r.URL.Query().Get("postal_code")
		rawCountryIn := r.URL.Query().Get("country")

		// Length-cap the raw input before TrimSpace so a multi-megabyte
		// blob doesn't even reach the normalization pass.
		if len(rawPostalIn) > maxPostalCodeLen {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Postal Code",
				"The postal_code query parameter is longer than the maximum allowed length.", rid)
			return
		}
		if len(rawCountryIn) > maxCountryLen {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Invalid Country",
				"The country query parameter is longer than the maximum allowed length.", rid)
			return
		}

		rawPostal := strings.TrimSpace(rawPostalIn)
		country := atlas.Country(strings.ToUpper(strings.TrimSpace(rawCountryIn)))

		if rawPostal == "" {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Missing Postal Code",
				"The postal_code query parameter is required.", rid)
			return
		}
		if country == "" {
			writeProblem(w, r, http.StatusBadRequest, problemValidation, "Missing Country",
				"The country query parameter is required.", rid)
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
				writeProblem(w, r, http.StatusNotFound, problemNotFound, "Postal Code Not Found",
					"No region is mapped to that postal code. Try a nearby code, or submit an organization for your area.", rid)
				return
			}
			logger.ErrorContext(r.Context(), "lookup failed",
				"err", err,
				"postal_code", postal,
				"country", country,
				"rid", rid,
			)
			writeProblem(w, r, http.StatusInternalServerError, problemInternal, "Internal Server Error",
				"An unexpected error occurred while handling this request.", rid)
			return
		}

		writeJSON(w, http.StatusOK, toOAPILookupResult(result))
	}
}
