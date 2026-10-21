package httpapi

import "net/http"

// securityHeadersMiddleware sets browser-hardening headers on every
// response. A JSON API has no framing use, so DENY rather than
// SAMEORIGIN. They are transport hardening, so openapi.yaml does not
// document them. Headers are set before next runs because net/http
// ignores header changes after the first WriteHeader.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
