package httpapi

import "net/http"

// healthHandler answers GET /healthz with a 200 and a tiny plaintext
// body. Used by Fly's health checks; deliberately doesn't touch any
// dependency (DB, etc.) so it stays cheap and predictable.
func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}
}
