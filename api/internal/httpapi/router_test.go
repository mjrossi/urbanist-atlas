package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
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
			name:        "readyz",
			path:        "/readyz",
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

// TestNotFound_ReturnsProblemJSON pins that an unmatched route gets a
// problem+json 404, not chi's text/plain default.
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

// TestMethodNotAllowed_ReturnsProblemJSON pins that a wrong method on a
// known route gets a problem+json 405, not chi's empty default. It reuses
// the not-found type URI; the status and title carry the meaning.
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

// panickingListStore panics from the /regions handler's store call so a
// test can drive a real panic through the full New() middleware stack.
type panickingListStore struct{ atlas.Store }

func (panickingListStore) ListRegions(context.Context) ([]atlas.RegionSummary, error) {
	panic("boom")
}

// TestNew_PanicIsLoggedAndCounted pins that a panicking handler still
// gets an access-log line and a status="500" request metric, i.e. that
// the recoverer runs inside the logging and metrics middleware.
func TestNew_PanicIsLoggedAndCounted(t *testing.T) {
	mem := atlas.NewMemStore()
	atlas.LoadDevFixtures(mem)
	logs := &captureHandler{}
	m := NewMetrics()
	srv := httptest.NewServer(New(Config{
		Store:       panickingListStore{mem},
		Logger:      slog.New(logs),
		Metrics:     m,
		CORSOrigins: []string{"https://allowed.example"},
	}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/regions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}

	if got := testutil.ToFloat64(m.httpRequests.WithLabelValues("/api/v1/regions", http.MethodGet, "500")); got != 1 {
		t.Errorf(`atlas_http_requests_total{route="/api/v1/regions",method="GET",status="500"} = %v, want 1`, got)
	}

	logs.mu.Lock()
	defer logs.mu.Unlock()
	var accessStatus int64 = -1
	for _, rec := range logs.records {
		if rec.Message != "http" {
			continue
		}
		rec.Attrs(func(a slog.Attr) bool {
			if a.Key == "status" {
				accessStatus = a.Value.Int64()
			}
			return true
		})
	}
	if accessStatus != http.StatusInternalServerError {
		t.Errorf("access-log status = %d, want 500 (-1 means no access-log line)", accessStatus)
	}
}
