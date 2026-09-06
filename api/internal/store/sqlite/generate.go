package sqlite

// sqlc turns internal/store/sqlite/queries/*.sql into the typed bindings
// in ./gen. The config lives at api/sqlc.yaml, so the path is relative
// to this file's directory (go generate runs commands from there).
//
// This directive is what makes `just api-gen` regenerate sqlc output
// alongside the oapi-codegen artifacts, and what lets `just api-gen-check`
// catch sqlc drift in CI.

//go:generate sqlc generate -f ../../../sqlc.yaml
