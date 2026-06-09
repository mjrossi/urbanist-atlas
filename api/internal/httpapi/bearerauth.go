package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// bearerAuthMiddleware gates requests behind a bearer token in the
// Authorization header. The expected token is configured via
// URBANIST_ADMIN_TOKEN on the server; clients send
// `Authorization: Bearer <token>`.
//
// Unlike clientSecretMiddleware (which intentionally no-ops when the
// secret is empty so local dev stays ergonomic without ceremony), an
// empty admin token disables admin endpoints entirely: every request
// is rejected with 503, on the principle that "no token configured"
// must never silently expose moderator-only endpoints.
func bearerAuthMiddleware(adminToken string) func(http.Handler) http.Handler {
	if adminToken == "" {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeProblem(w, r, http.StatusServiceUnavailable, problemInternal,
					"Admin Endpoints Disabled",
					"URBANIST_ADMIN_TOKEN is not configured on this server.",
					requestIDFromContext(r.Context()))
			})
		}
	}
	// Hash the configured token once at construction. Comparing fixed-width
	// SHA-256 digests (rather than the raw bytes) keeps the compare
	// constant-time regardless of the supplied token's length: a raw
	// subtle.ConstantTimeCompare returns immediately when the two slices
	// differ in length, which leaks the secret's length through timing.
	expectedHash := sha256.Sum256([]byte(adminToken))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := extractBearerToken(r.Header.Get("Authorization"))
			// Identical title/detail for missing AND wrong tokens so the
			// response shape doesn't leak which case the server hit. Both
			// sides are hashed to a fixed 32 bytes first so the
			// constant-time compare never short-circuits on a length
			// mismatch (which would leak the token length via timing).
			gotHash := sha256.Sum256([]byte(got))
			if subtle.ConstantTimeCompare(gotHash[:], expectedHash[:]) != 1 {
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

// extractBearerToken returns the token portion of an "Authorization:
// Bearer <token>" header value, or "" if the header is missing or
// not bearer-style. Case-insensitive on the scheme per RFC 6750.
func extractBearerToken(headerValue string) string {
	const prefix = "bearer "
	if len(headerValue) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(headerValue[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(headerValue[len(prefix):])
}
