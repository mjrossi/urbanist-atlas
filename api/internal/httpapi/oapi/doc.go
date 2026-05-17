// Package oapi holds the oapi-codegen-generated Go types for the
// Urbanist Atlas OpenAPI 3 spec (api/openapi.yaml). It is consumed by
// the HTTP handlers in internal/httpapi so request/response shapes
// stay in lockstep with the wire contract.
//
// The generated file (types.gen.go) is committed to git. Regenerate
// after editing api/openapi.yaml by running `go generate ./...` from
// api/ (or `just api-oapi-gen`, which wraps it).
//
//go:generate oapi-codegen -config cfg.yaml ../../../openapi.yaml
package oapi
