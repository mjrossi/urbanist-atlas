# urbanist-atlas — common dev commands.
# Run `just` (no args) to list recipes.
#
# `just` itself is pinned via mise (see mise.toml in the user's global
# tools — `aqua:casey/just`); a `mise install` at the repo root is
# enough to provision it.

set shell := ["bash", "-cu"]

# ── default ───────────────────────────────────────────

# show available recipes
default:
    @just --list --unsorted

# ── api: build & verify ───────────────────────────────

# run the API server with text logs
api-run:
    cd api && go run ./cmd/server serve --log-format=text

# build the api binary to api/bin/urbanist-atlas-server
api-build:
    cd api && mkdir -p bin && go build -o bin/urbanist-atlas-server ./cmd/server

# format Go code
api-fmt:
    cd api && gofmt -w .

# go vet ./...
api-vet:
    cd api && go vet ./...

# go test ./... with race detector, no cache (matches CI)
api-test:
    cd api && go test ./... -race -count=1

# vet + test — the CI gate for api/, run locally
api-check: api-vet api-test

# go mod tidy
api-tidy:
    cd api && go mod tidy

# ── api: operational subcommands ──────────────────────
# These wrap the server binary's urfave/cli subcommands.
# `migrate-*`, `seed`, and `loadpostal` currently return
# "not yet implemented" — they'll work once the Postgres store lands.

# apply pending DB migrations
migrate-up:
    cd api && go run ./cmd/server migrate up

# roll back the most recent migration
migrate-down:
    cd api && go run ./cmd/server migrate down

# show migration status
migrate-status:
    cd api && go run ./cmd/server migrate status

# load curated org seed data (api/seed/orgs.yaml) into the DB
seed:
    cd api && go run ./cmd/server seed

# ingest postal-code → region CSVs into the DB
loadpostal src country='US':
    cd api && go run ./cmd/server loadpostal --src {{src}} --country {{country}}

# ── api: live curl helpers (server must be running) ───

# curl /healthz against localhost
healthz port='8080':
    @curl -sS -i "http://localhost:{{port}}/healthz" | sed -n '1,8p'

# curl /api/v1/lookup, pretty-printed via jq
# usage: just lookup 11217  (or `just lookup M5V CA`)
lookup code country='US' port='8080':
    @curl -sS "http://localhost:{{port}}/api/v1/lookup?postal_code={{code}}&country={{country}}" | jq

# ── ci-equivalent ─────────────────────────────────────

# run every check CI would run today against the current tree
ci: api-check
