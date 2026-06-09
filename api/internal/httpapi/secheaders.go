package httpapi

import "net/http"

// securityHeadersMiddleware sets two browser-hardening headers on
// every response that passes through it. Mount at the very top of the
// global chain in New — unlike the ODbL attribution pair, which is
// scoped to /api/v1 data endpoints, these apply to ALL responses:
// /healthz, /readyz, the served OpenAPI document, and problem+json
// errors alike.
//
//   - X-Content-Type-Options: nosniff — opts out of browser MIME
//     sniffing so a response is only ever interpreted as its declared
//     Content-Type. Cheap insurance for an API that serves JSON,
//     problem+json, and YAML.
//   - X-Frame-Options: DENY — a JSON API has no legitimate framing
//     use case, so deny outright rather than SAMEORIGIN.
//
// These are transport-level hardening, not part of the wire contract
// — openapi.yaml deliberately does not document them.
//
// Ordering note: headers MUST be set BEFORE delegating to
// next.ServeHTTP. Once a downstream handler calls w.WriteHeader
// (explicitly or implicitly via the first w.Write), net/http freezes
// the response header map — any Set call after that point is a silent
// no-op. See the matching note on odblHeadersMiddleware.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
