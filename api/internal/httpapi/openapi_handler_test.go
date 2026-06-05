package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestEmbeddedOpenAPISpecMatchesCanonical guards against drift between
// the canonical `api/openapi.yaml` (the wire contract consumed by
// web/ via openapi-typescript) and the copy embedded into this
// package via `//go:embed openapi.yaml`. Go's embed directive can't
// reach across package boundaries (`..` is disallowed) and can't
// follow symlinks, so we maintain a sibling copy here — this test
// fails fast if someone updates one and forgets the other.
func TestEmbeddedOpenAPISpecMatchesCanonical(t *testing.T) {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test source file via runtime.Caller")
	}
	// here is .../api/internal/httpapi/openapi_handler_test.go
	canonical := filepath.Join(filepath.Dir(here), "..", "..", "openapi.yaml")
	want, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("read canonical spec at %s: %v", canonical, err)
	}
	if !bytes.Equal(openapiSpec, want) {
		t.Fatalf("embedded openapi.yaml drifted from canonical %s.\n"+
			"Run `just api-oapi-gen` to recopy the file alongside the "+
			"handler before committing.", canonical)
	}
}

// TestOpenAPIHandlerServesEmbeddedSpec covers the handler's wire
// contract: status 200, Content-Type, and the exact embedded bytes.
func TestOpenAPIHandlerServesEmbeddedSpec(t *testing.T) {
	srv := httptest.NewServer(openapiHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type: want %q, got %q", "application/yaml", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(body, openapiSpec) {
		t.Errorf("response body did not match embedded spec (%d vs %d bytes)", len(body), len(openapiSpec))
	}
}
