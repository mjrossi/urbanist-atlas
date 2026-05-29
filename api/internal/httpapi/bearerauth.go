package httpapi

import (
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
	expected := []byte(adminToken)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := extractBearerToken(r.Header.Get("Authorization"))
			// Identical title/detail for missing AND wrong tokens so the
			// response shape doesn't leak which case the server hit. The
			// constant-time compare above gives the same property at the
			// byte level.
			if subtle.ConstantTimeCompare([]byte(got), expected) != 1 {
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
