package httpapi

import (
	"net/http"
	"testing"
)

// assertSecurityHeaders checks both hardening headers on a response.
func assertSecurityHeaders(t *testing.T, resp *http.Response) {
	t.Helper()
	if got, want := resp.Header.Get("X-Content-Type-Options"), "nosniff"; got != want {
		t.Errorf("X-Content-Type-Options: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Frame-Options"), "DENY"; got != want {
		t.Errorf("X-Frame-Options: want %q, got %q", want, got)
	}
}

// TestSecurityHeaders_PresentOnAPISuccessResponse asserts the
// middleware puts both hardening headers on a /api/v1/** 200.
func TestSecurityHeaders_PresentOnAPISuccessResponse(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	assertSecurityHeaders(t, resp)
}

// TestSecurityHeaders_PresentOnHealthz pins the global-scoping
// decision: unlike the ODbL attribution pair (data endpoints only),
// the hardening headers cover /healthz too. If someone moves the
// middleware inside the /api/v1 group, this test fails.
func TestSecurityHeaders_PresentOnHealthz(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	assertSecurityHeaders(t, resp)
}

// TestSecurityHeaders_PresentOnProblemResponse asserts even a 404
// problem document carries the hardening headers — the middleware
// sets them before any downstream WriteHeader can freeze the map.
func TestSecurityHeaders_PresentOnProblemResponse(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/no/such/route")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}
	assertSecurityHeaders(t, resp)
}
