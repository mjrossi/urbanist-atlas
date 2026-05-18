package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// TestClientSecret_EmptyConfig_NoOp documents that when no secret is
// configured (URBANIST_CLIENT_SECRET is unset), the middleware is a
// no-op — every request passes through to the next handler. This is
// the local-dev path: contributors don't need to set the env var to
// run the server.
func TestClientSecret_EmptyConfig_NoOp(t *testing.T) {
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := clientSecretMiddleware("")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected downstream handler to run when no secret is configured")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestClientSecret_NoHeader_Unauthorized documents that when a secret
// IS configured but the request omits the X-Atlas-Client header, the
// middleware short-circuits with a 401 and the downstream handler
// never runs.
func TestClientSecret_NoHeader_Unauthorized(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler must not run when the X-Atlas-Client header is missing")
	})

	handler := clientSecretMiddleware("the-secret")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

// newSecretedTestServer mirrors lookup_test.go's newTestServer but
// wires a configured client-secret into the router so the bypass /
// gating behavior can be exercised end-to-end. Fixtures are loaded so
// /api/v1/lookup has data to return on the happy path.
func newSecretedTestServer(t *testing.T, secret string) *httptest.Server {
	t.Helper()
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	handler := New(Config{
		Store:        store,
		Logger:       slog.New(slog.DiscardHandler),
		APIVersion:   "v1",
		ClientSecret: secret,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// TestClientSecret_RouterBypass_Healthz pins the contract from
// CLAUDE.md § Launch strategy: /healthz must respond 200 even when
// the client-secret gate is configured AND the request omits the
// header. Liveness probes don't carry the secret.
func TestClientSecret_RouterBypass_Healthz(t *testing.T) {
	srv := newSecretedTestServer(t, "configured-but-not-sent")

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz status: got %d, want %d (bypass is required)", resp.StatusCode, http.StatusOK)
	}
}

// TestClientSecret_RouterBypass_OpenAPI pins the contract that the
// embedded OpenAPI spec stays publicly discoverable even when the
// gate is configured. Downstream consumers need to see the wire
// contract before they can know what secret to send.
func TestClientSecret_RouterBypass_OpenAPI(t *testing.T) {
	srv := newSecretedTestServer(t, "configured-but-not-sent")

	resp, err := http.Get(srv.URL + "/api/v1/openapi.yaml")
	if err != nil {
		t.Fatalf("GET /api/v1/openapi.yaml: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/api/v1/openapi.yaml status: got %d, want %d (bypass is required)", resp.StatusCode, http.StatusOK)
	}
}

// TestClientSecret_RouterGated_Lookup verifies the opposite half: a
// data endpoint (/api/v1/lookup) IS gated when a secret is
// configured. Without the header → 401; with the right header → 200.
func TestClientSecret_RouterGated_Lookup(t *testing.T) {
	srv := newSecretedTestServer(t, "the-secret")

	// Without the header: 401.
	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET /api/v1/lookup (no header): %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no header: got %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	// With the right header: 200.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/lookup?postal_code=11217&country=US", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-Atlas-Client", "the-secret")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/lookup (with header): %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("with header: got %d, want %d", resp2.StatusCode, http.StatusOK)
	}
}

// TestClientSecret_Unauthorized_ProblemDocShape pins the wire contract
// for the 401: application/problem+json content type, with the
// `unauthorized` problem-type URI matching CLAUDE.md and the OpenAPI
// spec's published URI scheme.
func TestClientSecret_Unauthorized_ProblemDocShape(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler must not run")
	})

	handler := clientSecretMiddleware("the-secret")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/problem+json")
	}

	var body oapi.ProblemDetails
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Type != problemUnauthorized {
		t.Errorf("type: got %q, want %q", body.Type, problemUnauthorized)
	}
	if body.Status != http.StatusUnauthorized {
		t.Errorf("status field: got %d, want %d", body.Status, http.StatusUnauthorized)
	}
	if body.Title != "Unauthorized" {
		t.Errorf("title: got %q, want %q", body.Title, "Unauthorized")
	}
}

// TestClientSecret_CorrectHeader_PassesThrough documents the happy
// path: when the X-Atlas-Client header matches the configured secret,
// the middleware lets the request through to the downstream handler.
func TestClientSecret_CorrectHeader_PassesThrough(t *testing.T) {
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := clientSecretMiddleware("the-secret")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	req.Header.Set("X-Atlas-Client", "the-secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("downstream handler was not called even though the secret matched")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
}

// TestClientSecret_WrongHeader_Unauthorized pins behavior for the
// "header present but wrong value" case (vs the missing-header case).
// Both code paths land in the same 401 today, but pinning each
// separately means a future refactor (e.g. swapping in
// subtle.ConstantTimeCompare) can't silently regress one branch.
func TestClientSecret_WrongHeader_Unauthorized(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("downstream handler must not run when X-Atlas-Client value is wrong")
	})

	handler := clientSecretMiddleware("the-secret")(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/lookup", nil)
	req.Header.Set("X-Atlas-Client", "totally-wrong-value")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
