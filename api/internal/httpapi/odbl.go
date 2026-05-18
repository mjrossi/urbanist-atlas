package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

// dataLicense is the SPDX identifier of the Urbanist Atlas dataset
// license. The source of truth for the license itself is
// LICENSE-DATA at the repo root; this constant only echoes the
// identifier into response headers and the collection-meta envelope.
const dataLicense = "ODbL-1.0"

// dataAttributionURL is the URL downstream consumers should link to
// when crediting the source. Hardcoded here so the production
// frontend domain is the canonical attribution target even if a
// future config field overrides the CORS origins or the serving host.
const dataAttributionURL = "https://urbanistatlas.com"

// odblHeadersMiddleware sets the two attribution headers on every
// response that passes through it. Mount inside the /api/v1 route
// group so /healthz (which is not a data endpoint) stays clean.
//
// Headers are set BEFORE delegating to next.ServeHTTP so they're
// already in the response writer's header map by the time a handler
// calls WriteHeader — net/http flushes headers on the first body
// write, so ordering matters.
//
// Headers are set unconditionally (success or error). Distinguishing
// 2xx from non-2xx in middleware would require response-status
// sniffing, and an error response that quotes the dataset license
// is harmless. See the slice #24 spec §1 for the rationale.
func odblHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Data-License", dataLicense)
		h.Set("X-Data-Attribution", dataAttributionURL)
		next.ServeHTTP(w, r)
	})
}

// newMeta returns the meta block for a single response: license,
// attribution URL, and a fresh UTC timestamp truncated to seconds.
//
// `GeneratedAt` is truncated before marshaling. Go's default
// `time.Time` JSON encoder uses RFC3339Nano, which emits sub-second
// digits when present; truncating to whole seconds drops the
// fractional component on the wire — a zero-nanosecond UTC time
// round-trips through RFC3339Nano as "...Z" with no '.'. The slice
// #24 spec mandates second precision. There is no caching layer in
// v1; the timestamp records when this response was produced.
func newMeta() oapi.Meta {
	return oapi.Meta{
		License:        dataLicense,
		AttributionUrl: dataAttributionURL,
		GeneratedAt:    time.Now().UTC().Truncate(time.Second),
	}
}

// respondCollection writes a JSON `{ meta, data }` envelope for a
// list response. Use this from any collection handler instead of
// writeJSON; single-resource handlers continue to use writeJSON.
//
// T uses generics so the call site keeps its typed slice (e.g.
// []oapi.MetroSummary) without an explicit any conversion. The
// helper encodes whatever T is, so adding a new collection
// endpoint is one call-site change plus a new *Envelope schema in
// openapi.yaml.
//
// A nil input slice is coerced to an empty slice so the encoded
// JSON has `"data": []`, never `"data": null`. Downstream clients
// can rely on `data` always being an array.
func respondCollection[T any](w http.ResponseWriter, items []T) {
	if items == nil {
		items = []T{}
	}
	body := struct {
		Meta oapi.Meta `json:"meta"`
		Data []T       `json:"data"`
	}{
		Meta: newMeta(),
		Data: items,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}
