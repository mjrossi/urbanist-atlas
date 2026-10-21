package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// bearerAuthMiddleware requires `Authorization: Bearer <adminToken>`.
// Unlike clientSecretMiddleware, an empty token answers every request
// with 503, so a missing config never exposes the admin endpoints.
func bearerAuthMiddleware(adminToken string) func(http.Handler) http.Handler {
	if adminToken == "" {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeProblem(w, r, http.StatusServiceUnavailable, problemAdminDisabled,
					"Admin Endpoints Disabled",
					"URBANIST_ADMIN_TOKEN is not configured on this server.",
					requestIDFromContext(r.Context()))
			})
		}
	}
	// subtle.ConstantTimeCompare returns early on a length mismatch, which
	// would leak the token's length; fixed-width digests avoid that.
	expectedHash := sha256.Sum256([]byte(adminToken))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := extractBearerToken(r.Header.Get("Authorization"))
			// Missing and wrong tokens get the same response.
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

// extractBearerToken returns the token from a bearer Authorization value,
// or "". The scheme is case-insensitive per RFC 6750.
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
