package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// TestHEAD_PublicRoutes_Return200WithEmptyBody pins the contract that
// every public GET route also accepts HEAD with the same headers and
// no body. Link-preview tools (Slack, Discord, etc.) and uptime probes
// HEAD URLs before unfurling or alerting; without this, those flows
// hit a 405 from chi's default routing.
func TestHEAD_PublicRoutes_Return200WithEmptyBody(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name        string
		path        string
		wantContent string // expected Content-Type prefix; "" skips the check
	}{
		{
			name:        "healthz",
			path:        "/healthz",
			wantContent: "text/plain",
		},
		{
			name: "readyz",
			path: "/readyz",
			// FileStore doesn't implement pinger so /readyz collapses
			// to plaintext "ok" — same shape as /healthz.
			wantContent: "text/plain",
		},
		{
			name:        "openapi.yaml",
			path:        "/api/v1/openapi.yaml",
			wantContent: "application/yaml",
		},
		{
			name:        "lookup",
			path:        "/api/v1/lookup?postal_code=11217&country=US",
			wantContent: "application/json",
		},
		{
			name:        "regions",
			path:        "/api/v1/regions",
			wantContent: "application/json",
		},
		{
			name:        "regions/{slug}",
			path:        "/api/v1/regions/brooklyn-ny",
			wantContent: "application/json",
		},
		{
			name:        "orgs/{slug}",
			path:        "/api/v1/orgs/transalt-brooklyn",
			wantContent: "application/json",
		},
		{
			name:        "recent",
			path:        "/api/v1/recent",
			wantContent: "application/json",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodHead, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("HEAD %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("status: want 200, got %d", resp.StatusCode)
			}
			if tc.wantContent != "" {
				if got := resp.Header.Get("Content-Type"); got == "" {
					t.Errorf("Content-Type: want prefix %q, got empty", tc.wantContent)
				} else if !startsWith(got, tc.wantContent) {
					t.Errorf("Content-Type: want prefix %q, got %q", tc.wantContent, got)
				}
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if len(body) != 0 {
				t.Errorf("HEAD body: want empty, got %d bytes", len(body))
			}
		})
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// TestNotFound_ReturnsProblemJSON pins API-04-a: an unmatched route must
// return 404 as application/problem+json (RFC 9457), not chi's stdlib
// text/plain "404 page not found". Once external consumers depend on
// "every non-2xx is problem+json," a bare text/plain 404 is a contract
// violation they'd have to code around.
func TestNotFound_ReturnsProblemJSON(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name        string
		path        string
		wantODbL    bool // /api/v1 paths carry the ODbL attribution headers
		wantTypeURI string
	}{
		{
			name:        "top-level unknown route",
			path:        "/does-not-exist",
			wantODbL:    false,
			wantTypeURI: problemNotFound,
		},
		{
			name:        "api/v1 unknown route",
			path:        "/api/v1/does-not-exist",
			wantODbL:    true,
			wantTypeURI: problemNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status: want 404, got %d", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Type"); !startsWith(got, "application/problem+json") {
				t.Errorf("Content-Type: want prefix %q, got %q", "application/problem+json", got)
			}

			var body problemBody
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode problem body: %v", err)
			}
			if body.Type != tc.wantTypeURI {
				t.Errorf("type: want %q (an existing catalog URI), got %q", tc.wantTypeURI, body.Type)
			}
			if body.Status != http.StatusNotFound {
				t.Errorf("status field: want 404, got %d", body.Status)
			}
			if body.RequestID == "" {
				t.Error("request_id: want non-empty, got empty")
			}

			if tc.wantODbL {
				if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
					t.Errorf("X-Data-License: want %q, got %q", want, got)
				}
				if resp.Header.Get("X-Data-Attribution") == "" {
					t.Error("X-Data-Attribution: want non-empty, got empty")
				}
			}
		})
	}
}

// TestMethodNotAllowed_ReturnsProblemJSON pins API-04-a's 405 half: a
// wrong-method request to a known route must return 405 as
// application/problem+json, not chi's stdlib empty 405. The 405 reuses an
// existing catalog type URI (no new problemMethodNotAllowed const — that
// would force an openapi.yaml mirror edit, which D-09 keeps exclusive to
// cluster (a) / plan 03-01); the status code + title carry the
// method-not-allowed semantics.
func TestMethodNotAllowed_ReturnsProblemJSON(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name     string
		method   string
		path     string
		wantODbL bool
	}{
		{
			name:     "wrong method on /healthz",
			method:   http.MethodPost,
			path:     "/healthz",
			wantODbL: false,
		},
		{
			name:     "wrong method on /api/v1/lookup",
			method:   http.MethodPost,
			path:     "/api/v1/lookup",
			wantODbL: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("status: want 405, got %d", resp.StatusCode)
			}
			if got := resp.Header.Get("Content-Type"); !startsWith(got, "application/problem+json") {
				t.Errorf("Content-Type: want prefix %q, got %q", "application/problem+json", got)
			}

			var body problemBody
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode problem body: %v", err)
			}
			// 405 reuses an existing catalog URI — must NOT be a freshly
			// invented one. Accept either problemValidation or problemNotFound
			// (both predate this plan in problem.go / the openapi catalog).
			if body.Type != problemValidation && body.Type != problemNotFound {
				t.Errorf("type: want an existing catalog URI (problemValidation or problemNotFound), got %q", body.Type)
			}
			if body.Status != http.StatusMethodNotAllowed {
				t.Errorf("status field: want 405, got %d", body.Status)
			}
			if body.RequestID == "" {
				t.Error("request_id: want non-empty, got empty")
			}

			if tc.wantODbL {
				if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
					t.Errorf("X-Data-License: want %q, got %q", want, got)
				}
				if resp.Header.Get("X-Data-Attribution") == "" {
					t.Error("X-Data-Attribution: want non-empty, got empty")
				}
			}
		})
	}
}
