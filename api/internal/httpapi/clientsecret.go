package httpapi

import (
	"crypto/subtle"
	"net/http"
)

// clientSecretMiddleware returns chi middleware that gates requests
// behind a shared `X-Atlas-Client` header. The expected secret is
// passed in; if it's empty, the middleware is a no-op (preserves
// local-dev ergonomics where URBANIST_CLIENT_SECRET isn't set).
//
// This is a cheap deterrent against casual scrapers, not a security
// boundary against motivated attackers. The secret is built into the
// frontend bundle via VITE_API_CLIENT_SECRET, so any client that
// runs the SPA can read it from devtools. Phase 2 (slices #26-#28)
// replaces this gate with per-user API keys + rate limiting.
func clientSecretMiddleware(secret string) func(http.Handler) http.Handler {
	if secret == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	expected := []byte(secret)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get("X-Atlas-Client"))
			// ConstantTimeCompare returns 0 for length mismatch or
			// differing bytes, 1 only on full equality. The timing-
			// safe compare matters less than `subtle.ConstantTimeEq`-
			// for-real-credentials but it's idiomatic and free.
			if subtle.ConstantTimeCompare(got, expected) != 1 {
				// Use the context-sourced rid (set by requestIDMiddleware
				// upstream) so the problem document's request_id matches
				// the X-Request-ID response header even when the client
				// didn't send one — same behavior as every other handler.
				writeProblem(w, r, http.StatusUnauthorized, problemUnauthorized,
					"Unauthorized", "missing or invalid X-Atlas-Client header",
					requestIDFromContext(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
