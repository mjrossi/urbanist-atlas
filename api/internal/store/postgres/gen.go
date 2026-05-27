package postgres

// Codegen entry point for the postgres store. `go generate ./...` from
// the api/ root runs sqlc against sqlc.yaml here and rebuilds the
// gen/ subpackage. The mise-pinned sqlc version is used.
//
//go:generate mise exec -- sqlc generate -f sqlc.yaml
