package httpapi

import (
	"net/http"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

// dataLicense is the SPDX identifier of the dataset license; LICENSE-DATA
// at the repo root is the license itself.
const dataLicense = "ODbL-1.0"

// dataAttributionURL is the link consumers credit. It is hardcoded so the
// production domain stays the attribution target whatever host serves
// the API.
const dataAttributionURL = "https://urbanistatlas.com"

// odblHeadersMiddleware sets the attribution headers on every response,
// errors included. Headers are set before next runs because net/http
// ignores header changes after the first WriteHeader. See
// docs/superpowers/specs/2026-05-18-odbl-response-shape-design.md.
func odblHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Data-License", dataLicense)
		h.Set("X-Data-Attribution", dataAttributionURL)
		next.ServeHTTP(w, r)
	})
}

// newMeta returns the meta block for one response. GeneratedAt is
// truncated to whole seconds, the precision the ODbL design sets,
// because time.Time marshals as RFC3339Nano.
func newMeta() oapi.Meta {
	return oapi.Meta{
		License:        dataLicense,
		AttributionUrl: dataAttributionURL,
		GeneratedAt:    time.Now().UTC().Truncate(time.Second),
	}
}

// respondCollection writes the `{ meta, data }` envelope that every
// collection endpoint uses. A nil slice encodes as `"data": []`.
func respondCollection[T any](w http.ResponseWriter, items []T) {
	body := struct {
		Meta oapi.Meta `json:"meta"`
		Data []T       `json:"data"`
	}{
		Meta: newMeta(),
		Data: nonNilSlice(items),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	encodeJSON(w, body)
}
