package httpapi

import (
	"net/http"
	"strings"
)

// corsMiddleware is a minimal stdlib CORS implementation. An allowlist
// entry is an exact origin ("http://localhost:5173") or a "*." suffix
// ("*.link00seven.workers.dev"). A suffix matches https origins only, so
// a plaintext http:// subdomain cannot satisfy it.
func corsMiddleware(allowed []string) func(http.Handler) http.Handler {
	exact := map[string]struct{}{}
	suffixes := make([]string, 0)
	for _, a := range allowed {
		switch {
		case strings.HasPrefix(a, "*."):
			suffixes = append(suffixes, a[1:]) // ".link00seven.workers.dev"
		default:
			exact[a] = struct{}{}
		}
	}

	allow := func(origin string) bool {
		if _, ok := exact[origin]; ok {
			return true
		}
		// Suffixes match https origins only.
		if !strings.HasPrefix(origin, "https://") {
			return false
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
			// Vary even for disallowed origins, so a shared cache never
			// serves one origin's response to another.
			if origin != "" {
				w.Header().Add("Vary", "Origin")
				if allow(origin) {
					h := w.Header()
					h.Set("Access-Control-Allow-Origin", origin)
					h.Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Atlas-Client, X-Request-ID")
					h.Set("Access-Control-Max-Age", "86400")
					// Browsers hide non-safelisted response headers from
					// cross-origin script unless they are listed here.
					h.Set("Access-Control-Expose-Headers", "X-Request-ID, Retry-After, X-Next-Cursor")
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
