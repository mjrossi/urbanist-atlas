package httpapi

import (
	_ "embed"
	"net/http"
)

// openapiSpec is the v1 wire contract, embedded into the server binary
// so `GET /api/v1/openapi.yaml` can serve the exact bytes that shipped
// with the running release.
//
// The CANONICAL source of truth is `api/openapi.yaml` (the file
// imported by web/ via openapi-typescript). Because go:embed paths
// cannot escape the package directory (`..` is not allowed) and cannot
// follow symlinks, this package keeps a real copy of the spec
// alongside. A unit test (TestEmbeddedOpenAPISpecMatchesCanonical)
// asserts the two files are byte-identical so they can't silently
// drift; the `go:generate` directive below refreshes the copy.
//
// To update the spec: edit `api/openapi.yaml`, then run
// `just api-oapi-gen` (which runs `go generate ./...` to regenerate
// `oapi/types.gen.go` and to recopy the YAML next to this file). The
// unit test will fail on the next `go test` until both files match.
//
//go:generate cp ../../openapi.yaml openapi.yaml
//go:embed openapi.yaml
var openapiSpec []byte

// openapiHandler answers GET /api/v1/openapi.yaml with the bytes of
// the embedded spec. Content-Type is `application/yaml` per the
// matching response declared in the spec itself.
func openapiHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		// Short cache window — the spec is versioned with the binary
		// and changes only on redeploy.
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(openapiSpec)
	}
}
