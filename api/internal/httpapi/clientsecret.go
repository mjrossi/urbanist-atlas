package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// clientSecretMiddleware requires a shared `X-Atlas-Client` header, or
// does nothing when secret is empty (local dev). The secret is meant to
// ship in the public frontend bundle, so this deters casual scrapers and
// is not a security boundary.
func clientSecretMiddleware(secret string) func(http.Handler) http.Handler {
	if secret == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	// Hashed for the same length-independent compare as bearerAuthMiddleware.
	expectedHash := sha256.Sum256([]byte(secret))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotHash := sha256.Sum256([]byte(r.Header.Get("X-Atlas-Client")))
			if subtle.ConstantTimeCompare(gotHash[:], expectedHash[:]) != 1 {
				// Missing and wrong secrets get the same response.
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
