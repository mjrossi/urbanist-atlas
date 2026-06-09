package httpapi

import (
	"crypto/sha256"
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
	// Hash the configured secret once. Comparing fixed-width SHA-256
	// digests rather than the raw bytes keeps the compare constant-time
	// regardless of the supplied value's length and matches the admin
	// bearer gate (bearerauth.go), so the two credential checks don't
	// diverge in rationale. This secret is low-stakes — it ships in the
	// frontend bundle — but a uniform, length-leak-free compare is free
	// and removes a "why is this one different?" maintenance snag.
	expectedHash := sha256.Sum256([]byte(secret))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHash := sha256.Sum256([]byte(r.Header.Get("X-Atlas-Client")))
			if subtle.ConstantTimeCompare(gotHash[:], expectedHash[:]) != 1 {
				// Use the context-sourced rid (set by requestIDMiddleware
				// upstream) so the problem document's request_id matches
				// the X-Request-ID response header even when the client
				// didn't send one — same behavior as every other handler.
				//
				// Identical title/detail for missing AND wrong secret so
				// the response shape doesn't leak which case the server
				// hit; the constant-time compare above provides the same
				// property at the byte level.
				writeProblem(w, r, http.StatusUnauthorized, problemUnauthorized,
					"Unauthorized",
					"Authentication is required for this endpoint.",
					requestIDFromContext(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
