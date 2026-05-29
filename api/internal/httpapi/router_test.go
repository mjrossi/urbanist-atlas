package httpapi

import (
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
