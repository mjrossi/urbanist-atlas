package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCORS_VaryOriginAlwaysSetWhenOriginPresent pins that the
// middleware emits `Vary: Origin` whenever the request carries an
// Origin, even for disallowed origins and even on the preflight 204
// path. Without this a shared cache could merge the bare 204 from
// one origin with the CORS-headered 204 from another.
func TestCORS_VaryOriginAlwaysSetWhenOriginPresent(t *testing.T) {
	mw := corsMiddleware([]string{"https://allowed.example"})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	cases := []struct {
		name       string
		method     string
		origin     string
		wantVary   bool
		wantACAO   bool
		wantStatus int
	}{
		{
			name:       "allowed-origin GET",
			method:     http.MethodGet,
			origin:     "https://allowed.example",
			wantVary:   true,
			wantACAO:   true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "disallowed-origin GET",
			method:     http.MethodGet,
			origin:     "https://attacker.example",
			wantVary:   true,
			wantACAO:   false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "allowed-origin preflight",
			method:     http.MethodOptions,
			origin:     "https://allowed.example",
			wantVary:   true,
			wantACAO:   true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "disallowed-origin preflight",
			method:     http.MethodOptions,
			origin:     "https://attacker.example",
			wantVary:   true,
			wantACAO:   false,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "no-origin GET",
			method:     http.MethodGet,
			origin:     "",
			wantVary:   false,
			wantACAO:   false,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Errorf("status: want %d, got %d", tc.wantStatus, rr.Code)
			}
			vary := rr.Header().Values("Vary")
			gotVary := false
			for _, v := range vary {
				if strings.EqualFold(v, "Origin") {
					gotVary = true
					break
				}
			}
			if gotVary != tc.wantVary {
				t.Errorf("Vary Origin: want %v, got %v (full Vary: %v)", tc.wantVary, gotVary, vary)
			}
			gotACAO := rr.Header().Get("Access-Control-Allow-Origin") != ""
			if gotACAO != tc.wantACAO {
				t.Errorf("Access-Control-Allow-Origin presence: want %v, got %v (value: %q)",
					tc.wantACAO, gotACAO, rr.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestCORS_WildcardSuffixEnforcesHTTPSScheme pins that the `*.`-suffix
// form requires `https://`, so a spoofed `http://…` origin is rejected
// while the real `https://…` preview origin is allowed. The allowlist
// matches fly.toml's plus the dev localhost entry, so it also pins that
// exact entries stay scheme-sensitive.
func TestCORS_WildcardSuffixEnforcesHTTPSScheme(t *testing.T) {
	mw := corsMiddleware([]string{
		"https://urbanistatlas.com",
		"*.link00seven.workers.dev",
		"http://localhost:5173",
	})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(next)

	cases := []struct {
		name     string
		origin   string
		wantACAO bool
	}{
		{
			name:     "https workers.dev preview allowed",
			origin:   "https://preview.link00seven.workers.dev",
			wantACAO: true,
		},
		{
			name:     "http workers.dev preview REJECTED (HOST-03b)",
			origin:   "http://preview.link00seven.workers.dev",
			wantACAO: false,
		},
		{
			name:     "https apex allowed",
			origin:   "https://urbanistatlas.com",
			wantACAO: true,
		},
		{
			name:     "http apex REJECTED",
			origin:   "http://urbanistatlas.com",
			wantACAO: false,
		},
		{
			name:     "localhost dev origin unchanged (http allowed)",
			origin:   "http://localhost:5173",
			wantACAO: true,
		},
		{
			name:     "bare suffix without scheme REJECTED",
			origin:   "preview.link00seven.workers.dev",
			wantACAO: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Origin", tc.origin)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			gotACAO := rr.Header().Get("Access-Control-Allow-Origin") != ""
			if gotACAO != tc.wantACAO {
				t.Errorf("Access-Control-Allow-Origin presence for %q: want %v, got %v (value: %q)",
					tc.origin, tc.wantACAO, gotACAO, rr.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

// TestCORS_ExposesResponseHeaders pins that allowed origins can read the
// non-safelisted response headers the SPA depends on (web/src/lib/api.ts
// reads X-Request-ID and Retry-After). Without Access-Control-Expose-Headers
// the browser hides them from cross-origin script.
func TestCORS_ExposesResponseHeaders(t *testing.T) {
	mw := corsMiddleware([]string{"https://allowed.example"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const want = "X-Request-ID, Retry-After, X-Next-Cursor"
	for _, tc := range []struct {
		origin string
		want   string
	}{
		{origin: "https://allowed.example", want: want},
		{origin: "https://attacker.example", want: ""},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", tc.origin)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if got := rr.Header().Get("Access-Control-Expose-Headers"); got != tc.want {
			t.Errorf("origin %q: Access-Control-Expose-Headers = %q, want %q", tc.origin, got, tc.want)
		}
	}
}
