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

# vet + test + gen-no-diff — the CI gate for api/, run locally
api-check: api-vet api-test api-gen-check

# go mod tidy
api-tidy:
    cd api && go mod tidy

# regenerate sqlc Go bindings from internal/store/postgres/queries/*.sql.
# Wrapped in `mise exec --` so the pinned sqlc version is used even
# when the shell doesn't have mise activated.
api-sqlc-gen:
    cd api && mise exec -- sqlc generate -f internal/store/postgres/sqlc.yaml

# regenerate oapi-codegen Go types AND refresh the embedded copy of
# openapi.yaml next to the handler that serves it. Both flow through
# `go generate ./...` so adding a new generated artifact is just a
# matter of dropping a //go:generate directive on the right file.
api-oapi-gen:
    cd api && mise exec -- go generate ./...

# run the postgres-backed integration tests under the `integration`
# build tag (requires Docker). Cheap default test suite stays
# tag-free so `just api-test` keeps running on machines without
# Docker.
api-test-integration:
    cd api && go test -tags=integration -race -count=1 ./internal/store/postgres/...

# fail if any generated file would change. Regenerates oapi-codegen
# and the embedded spec copy via `go generate`, then regenerates sqlc,
# then asks git if anything moved. Used inside api-check so `just ci`
# rejects commits that forgot to regenerate.
api-gen-check:
    @cd api && mise exec -- go generate ./...
    @cd api && mise exec -- sqlc generate -f internal/store/postgres/sqlc.yaml
    @cd api && git diff --exit-code -- \
        internal/httpapi/oapi/types.gen.go \
        internal/httpapi/openapi.yaml \
        internal/store/postgres/gen/ \
        || (echo "generated files drifted; run \`just api-oapi-gen && just api-sqlc-gen\` and commit." && exit 1)

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

# ── pg: dev postgres lifecycle (docker-based) ─────────
# Local dev Postgres runs in a named docker container with a
# persistent volume on port 55432 (non-default to avoid clashing
# with any system Postgres on :5432). Same image
# (postgres:17-alpine) as the integration test suite, so the wire
# is identical.
#
# Credentials and DB name are dev-only and match what
# mise.development.toml hands to URBANIST_DB_URL:
#   user: urbanist  pass: urbanist  db: urbanist_atlas_dev

# start the dev postgres container (creates on first run, starts on subsequent), then wait for it to accept connections
pg-up:
    @if ! docker container inspect urbanist-atlas-pg >/dev/null 2>&1; then \
        docker run -d --name urbanist-atlas-pg \
            -p 55432:5432 \
            -e POSTGRES_USER=urbanist \
            -e POSTGRES_PASSWORD=urbanist \
            -e POSTGRES_DB=urbanist_atlas_dev \
            -v urbanist-atlas-pg-data:/var/lib/postgresql/data \
            postgres:17-alpine >/dev/null; \
    else \
        docker start urbanist-atlas-pg >/dev/null; \
    fi
    @until docker exec urbanist-atlas-pg pg_isready -U urbanist -d urbanist_atlas_dev >/dev/null 2>&1; do sleep 0.5; done
    @echo "dev postgres ready on :55432 (db: urbanist_atlas_dev)"

# stop the dev postgres container (keeps the data volume so a later pg-up is instant)
pg-down:
    @docker stop urbanist-atlas-pg >/dev/null 2>&1 || true
    @echo "dev postgres stopped (data volume kept; pg-reset to nuke)"

# nuke the container AND the data volume — start completely fresh
pg-reset:
    @docker rm -f urbanist-atlas-pg >/dev/null 2>&1 || true
    @docker volume rm urbanist-atlas-pg-data >/dev/null 2>&1 || true
    @echo "dev postgres container + data volume removed; run 'just pg-up' to recreate"

# open a psql shell into the dev database (via TCP — the alpine
# image puts its socket at /var/run/postgresql, not psql's default /tmp)
pg-shell:
    docker exec -it urbanist-atlas-pg psql -h localhost -U urbanist urbanist_atlas_dev

# tail the dev postgres container logs (Ctrl-C to detach)
pg-logs:
    docker logs -f urbanist-atlas-pg

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
