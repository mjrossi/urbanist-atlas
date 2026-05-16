package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// lookupHandler answers GET /api/v1/lookup?postal_code=…&country=….
//
// Responses:
//   - 200: a LookupResult JSON body.
//   - 400: missing or invalid query parameters.
//   - 404: postal code not found in our data.
//   - 500: anything else (logged with the request ID).
func lookupHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		postal := strings.TrimSpace(r.URL.Query().Get("postal_code"))
		country := atlas.Country(strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country"))))

		if postal == "" {
			writeError(w, http.StatusBadRequest, "postal_code is required")
			return
		}
		if country == "" {
			writeError(w, http.StatusBadRequest, "country is required (US or CA)")
			return
		}
		if country != atlas.CountryUS && country != atlas.CountryCA {
			writeError(w, http.StatusBadRequest, "country must be US or CA")
			return
		}

		result, err := atlas.Lookup(r.Context(), store, atlas.LookupQuery{
			PostalCode: postal,
			Country:    country,
		})
		if err != nil {
			if errors.Is(err, atlas.ErrPostalCodeNotFound) {
				writeError(w, http.StatusNotFound, "postal code not found — try a nearby code, or submit an organization for your area")
				return
			}
			logger.ErrorContext(r.Context(), "lookup failed",
				"err", err,
				"postal_code", postal,
				"country", country,
				"rid", requestIDFromContext(r.Context()),
			)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}
