package httpapi

import (
	_ "embed"
	"net/http"
)

// openapiSpec is the v1 wire contract as shipped with this binary.
// The embed directive cannot reach `api/openapi.yaml` (no `..`, no
// symlinks), so this package keeps a copy, refreshed by `just api-gen`
// and held byte-identical by TestEmbeddedOpenAPISpecMatchesCanonical.
//
//go:generate cp ../../openapi.yaml openapi.yaml
//go:embed openapi.yaml
var openapiSpec []byte

// openapiHandler serves the embedded spec.
func openapiHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		// The spec changes only on redeploy.
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openapiSpec)
	}
}
