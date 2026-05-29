package httpapi

import (
	"net/http"
	"strings"
)

// corsMiddleware is an intentionally minimal CORS implementation that
// trades flexibility for stdlib-only simplicity. It supports two
// allowlist entry forms:
//
//   - Exact origin match: "http://localhost:5173"
//   - Suffix match (one leading "*"): "*.pages.dev" matches
//     "https://branch-x.urbanist-atlas.pages.dev"
//
// Anything else (regex, header allowlisting beyond the defaults,
// per-route policies) is out of scope for v1. When we need more, we
// adopt go-chi/cors and revisit.
func corsMiddleware(allowed []string) func(http.Handler) http.Handler {
	exact := map[string]struct{}{}
	suffixes := make([]string, 0)
	for _, a := range allowed {
		switch {
		case strings.HasPrefix(a, "*."):
			suffixes = append(suffixes, a[1:]) // ".pages.dev"
		default:
			exact[a] = struct{}{}
		}
	}

	allow := func(origin string) bool {
		if _, ok := exact[origin]; ok {
			return true
		}
		for _, suf := range suffixes {
			if strings.HasSuffix(origin, suf) {
				return true
			}
		}
		return false
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			// Always Vary on Origin when one is present, even for
			// disallowed origins — otherwise a shared cache could
			// store the no-CORS-headers response for origin A and
			// serve it back to origin B's preflight (or vice versa).
			// Set BEFORE the allowlist check so the preflight-from-
			// disallowed-origin 204 below also carries it.
			if origin != "" {
				w.Header().Add("Vary", "Origin")
				if allow(origin) {
					h := w.Header()
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Atlas-Client, X-Request-ID")
					h.Set("Access-Control-Max-Age", "86400")
				}
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
