package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/internal/coverage"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// lookupHandler answers GET /api/v1/lookup?postal_code=…&country=….
func lookupHandler(store atlas.Store, logger *slog.Logger, m *Metrics, rec *coverage.Recorder, u *usage.Recorder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := requestIDFromContext(r.Context())
		rawPostalIn := r.URL.Query().Get("postal_code")
		rawCountryIn := r.URL.Query().Get("country")

		// Capped before TrimSpace so an oversized value is never processed.
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
		// No known-country gate: an unknown country is simply a 404 from
		// atlas.Lookup. Normalizing here gives logs and the military check
		// the canonical form; the store's own re-normalization is idempotent.
		postal := atlas.NormalizePostalCode(country, rawPostal)

		result, err := atlas.Lookup(r.Context(), store, atlas.LookupQuery{
			PostalCode: postal,
			Country:    country,
		})
		if err != nil {
			if errors.Is(err, atlas.ErrPostalCodeNotFound) {
				// APO/FPO/DPO ZIPs are valid addresses with no residential
				// region — never a coverage gap — so they get a tailored
				// message and a distinct metrics label, not the generic miss.
				if atlas.IsMilitaryPostalCode(country, postal) {
					m.incLookup(string(country), "military")
					u.Increment(usage.KindLookupResult, "military")
					u.Increment(usage.KindLookupCountry, metricCountry(string(country)))
					writeProblem(w, r, http.StatusNotFound, problemMilitaryZIP, "Military or Diplomatic ZIP Code",
						"APO, FPO, and DPO ZIP codes are military and diplomatic addresses that aren't tied to a local region. Enter a residential ZIP code to find organizations near you.", rid)
					return
				}
				m.incLookup(string(country), "miss")
				u.Increment(usage.KindLookupResult, "miss")
				u.Increment(usage.KindLookupCountry, metricCountry(string(country)))
				writeProblem(w, r, http.StatusNotFound, problemNotFound, "Postal Code Not Found",
					"No region is mapped to that postal code. Try a nearby code, or file a tip if you know an organization there.", rid)
				return
			}
			logger.ErrorContext(r.Context(), "lookup failed",
				"err", err,
				"postal_code", postal,
				"country", country,
				"rid", rid,
			)
			writeInternalProblem(w, r, rid)
			return
		}

		tier := lookupTier(len(result.Local), len(result.Regional), len(result.Statewide))
		m.incLookup(string(country), "hit")
		m.incLookupTier(string(country), tier)
		u.Increment(usage.KindLookupResult, "hit")
		u.Increment(usage.KindLookupTier, tier)
		u.Increment(usage.KindLookupCountry, metricCountry(string(country)))
		// ResolvedAncestry[0] is the region the postal code resolved to.
		// Usage records its slug, never the postal code, which is
		// semi-identifying at low volume (usage-digest spec D3).
		if len(result.ResolvedAncestry) > 0 {
			u.Increment(usage.KindLookup, result.ResolvedAncestry[0].Slug)
		}
		// A resolved region with no orgs in any tier is a coverage gap.
		// The recorder samples, which is what lets it take the postal code.
		if tier == "empty" {
			rec.RecordEmpty("lookup", string(country), postal)
		}
		logger.DebugContext(r.Context(), "lookup ok",
			"country", country,
			"tier", tier,
			"local_count", len(result.Local),
			"regional_count", len(result.Regional),
			"statewide_count", len(result.Statewide),
			"rid", rid,
		)
		writeJSON(w, http.StatusOK, toOAPILookupResult(result))
	}
}
