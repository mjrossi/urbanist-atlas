package sqlite

// sqlc turns queries/*.sql into the typed bindings in ./gen. go generate
// runs from this file's directory, so the config path is relative to it.

//go:generate sqlc generate -f ../../../sqlc.yaml
