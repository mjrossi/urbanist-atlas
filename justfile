# urbanist-atlas — common dev commands.
# Run `just` (no args) to list recipes, organized by group.
#
# `just` itself is pinned in mise.toml (`aqua:casey/just`); a single
# `mise install` at the repo root provisions it alongside go, node,
# sqlc, goose, oapi-codegen, and staticcheck.
#
# Groups: api, data, postgres, web, heroku, smoke, ci. Each group
# corresponds to a section comment below.

set shell := ["bash", "-cu"]

# ── default ───────────────────────────────────────────

# show available recipes, organized by group
[private]
default:
    @just --list --unsorted

# ── api: build & verify ───────────────────────────────

# run the API server with text logs
[group('api')]
api-run:
    cd api && go run ./cmd/server serve --log-format=text

# build the api binary to api/bin/urbanist-atlas-server
[group('api')]
api-build:
    cd api && mkdir -p bin && go build -o bin/urbanist-atlas-server ./cmd/server

# format Go code
[group('api')]
api-fmt:
    cd api && gofmt -w .

# go vet ./...
[group('api')]
api-vet:
    cd api && go vet ./...

# go test ./... with race detector, no cache (matches CI)
[group('api')]
api-test:
    cd api && go test ./... -race -count=1

# vet + test + gen-no-diff — the CI gate for api/, run locally
[group('api')]
api-check: api-vet api-test api-gen-check

# go mod tidy
[group('api')]
api-tidy:
    cd api && go mod tidy

# regenerate sqlc Go bindings from internal/store/postgres/queries/*.sql.
# Wrapped in `mise exec --` so the pinned sqlc version is used even
# when the shell doesn't have mise activated.
[group('api')]
[doc('regenerate sqlc Go bindings (mise-pinned sqlc)')]
api-sqlc-gen:
    cd api && mise exec -- sqlc generate -f internal/store/postgres/sqlc.yaml

# regenerate oapi-codegen Go types AND refresh the embedded copy of
# openapi.yaml next to the handler that serves it. Both flow through
# `go generate ./...` so adding a new generated artifact is just a
# matter of dropping a //go:generate directive on the right file.
[group('api')]
[doc('regenerate oapi-codegen types + sync embedded openapi.yaml')]
api-oapi-gen:
    cd api && mise exec -- go generate ./...

# run the postgres-backed integration tests under the `integration`
# build tag (requires Docker). Cheap default test suite stays
# tag-free so `just api-test` keeps running on machines without
# Docker.
[group('api')]
[doc('run postgres integration tests (Docker; build tag: integration)')]
api-test-integration:
    cd api && go test -tags=integration -race -count=1 ./internal/store/postgres/...

# fail if any generated file would change. Regenerates oapi-codegen
# and the embedded spec copy via `go generate`, then regenerates sqlc,
# then asks git if anything moved. Used inside api-check so `just ci`
# rejects commits that forgot to regenerate.
[group('api')]
[doc('fail if any generated file would drift after a regen')]
api-gen-check:
    @cd api && mise exec -- go generate ./...
    @cd api && mise exec -- sqlc generate -f internal/store/postgres/sqlc.yaml
    @cd api && git diff --exit-code -- \
        internal/httpapi/oapi/types.gen.go \
        internal/httpapi/openapi.yaml \
        internal/store/postgres/gen/ \
        || (echo "generated files drifted; run \`just api-oapi-gen && just api-sqlc-gen\` and commit." && exit 1)

# ── data: operational subcommands ─────────────────────
# These wrap the server binary's urfave/cli subcommands for the
# data-loading flow (migrations + seed fixtures).

# apply pending DB migrations
[group('data')]
migrate-up:
    cd api && go run ./cmd/server migrate up

# roll back the most recent migration
[group('data')]
migrate-down:
    cd api && go run ./cmd/server migrate down

# show migration status
[group('data')]
migrate-status:
    cd api && go run ./cmd/server migrate status

# load curated org seed data (api/seed/orgs.toml) into the DB
[group('data')]
seed:
    cd api && go run ./cmd/server seed

# load region taxonomy (toml -> regions + region_parents)
# usage: just loadregions seed/regions_us.toml US
[group('data')]
[doc('load region taxonomy; e.g. `just loadregions seed/regions_us.toml US`')]
loadregions src country='US':
    cd api && go run ./cmd/server loadregions --src {{src}} --country {{country}}

# map postal codes to leaf regions (csv -> postal_codes)
# usage: just loadpostal seed/postal_codes_us.csv US
[group('data')]
[doc('map postal codes to leaf regions; e.g. `just loadpostal seed/postal_codes_us.csv US`')]
loadpostal src country='US':
    cd api && go run ./cmd/server loadpostal --src {{src}} --country {{country}}

# load all bundled fixtures in the right order:
# regions first (so leaf slugs resolve), then postal codes, then orgs.
# Wraps the `loaddata` binary subcommand so dev runs go through the
# exact same orchestration the Heroku deploy uses
# (heroku run urbanist-atlas-server loaddata). The country list
# lives in api/internal/loaddata/loaddata.go — add new countries
# there, not here.
[group('data')]
[doc('load every bundled fixture in dependency order (regions → postal → orgs)')]
loaddata:
    cd api && go run ./cmd/server loaddata

# ── postgres: dev container lifecycle ─────────────────
# Local dev Postgres runs in a named docker container with a
# persistent volume on port 55432 (non-default to avoid clashing
# with any system Postgres on :5432). Same image
# (postgres:17-alpine) as the integration test suite, so the wire
# is identical.
#
# Credentials and DB name are dev-only and match what
# mise.development.toml hands to DATABASE_URL:
#   user: urbanist  pass: urbanist  db: urbanist_atlas_dev

# start the dev postgres container (creates on first run, starts on subsequent), then wait for it to accept connections
[group('postgres')]
[doc('start the dev postgres container and wait for readiness')]
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
    @i=0; until docker exec urbanist-atlas-pg pg_isready -U urbanist -d urbanist_atlas_dev >/dev/null 2>&1; do \
        i=$((i+1)); \
        if [ "$i" -ge 120 ]; then \
            echo "pg-up: postgres still not ready after ~60s; check 'docker logs urbanist-atlas-pg'" >&2; \
            exit 1; \
        fi; \
        sleep 0.5; \
    done
    @echo "dev postgres ready on :55432 (db: urbanist_atlas_dev)"

# stop the dev postgres container (keeps the data volume so a later pg-up is instant)
[group('postgres')]
[doc('stop the dev postgres container (data volume kept)')]
pg-down:
    @docker stop urbanist-atlas-pg >/dev/null 2>&1 || true
    @echo "dev postgres stopped (data volume kept; pg-reset to nuke)"

# nuke the container AND the data volume — start completely fresh
[group('postgres')]
pg-reset:
    @docker rm -f urbanist-atlas-pg >/dev/null 2>&1 || true
    @docker volume rm urbanist-atlas-pg-data >/dev/null 2>&1 || true
    @echo "dev postgres container + data volume removed; run 'just pg-up' to recreate"

# open a psql shell into the dev database (via TCP — the alpine
# image puts its socket at /var/run/postgresql, not psql's default /tmp)
[group('postgres')]
[doc('open a psql shell into the dev database (via TCP)')]
pg-shell:
    docker exec -it urbanist-atlas-pg psql -h localhost -U urbanist urbanist_atlas_dev

# tail the dev postgres container logs (Ctrl-C to detach)
[group('postgres')]
pg-logs:
    docker logs -f urbanist-atlas-pg

# ── web: build & verify ───────────────────────────────

# run the Vite dev server (defaults to http://localhost:5173)
[group('web')]
web-dev:
    cd web && npm run dev

# install JS deps with the lockfile (matches CI)
[group('web')]
web-deps:
    cd web && npm ci

# run eslint
[group('web')]
web-lint:
    cd web && npm run lint

# vitest --run (no watch, matches CI)
[group('web')]
web-test:
    cd web && npm test -- --run

# production-mode bundle build
[group('web')]
web-build:
    cd web && npm run build

# regenerate the TS wire types from api/openapi.yaml
[group('web')]
web-oapi-gen:
    cd web && npm run generate:api

# fail if api.gen.ts would change after a regen against the
# current openapi.yaml. Mirrors api-gen-check; the wire contract
# can't drift silently between the two halves.
[group('web')]
[doc('fail if api.gen.ts would drift after a regen')]
web-gen-check:
    @cd web && npm run generate:api
    @git diff --exit-code -- web/src/lib/api.gen.ts \
        || (echo "TS wire types drifted; run \`just web-oapi-gen\` and commit." && exit 1)

# deps + lint + test + build + gen-no-diff — the CI gate for web/,
# run locally
[group('web')]
[doc('deps + lint + test + build + gen-no-diff — CI gate for web/')]
web-check: web-deps web-lint web-test web-build web-gen-check

# ── heroku: deploy + ops ──────────────────────────────
# Thin wrappers around `heroku` so the deploy / logs / config verbs
# are discoverable via `just --list`. App name (urbanist-atlas) is
# pinned via -a so these work from any branch without git remote
# config. Initial provisioning (app creation, Essential-0 attach,
# secrets) lives in docs/deploy.md — these recipes are for ongoing
# ops once the app exists.

# Uses HEAD:main so this works from a feature branch without first
# switching back to a main-tracking checkout — Heroku only deploys
# refs pushed to its `main` branch.
[group('heroku')]
[doc('push current HEAD to Heroku as main (release phase runs migrations)')]
heroku-deploy:
    git push heroku HEAD:main

# tail live Heroku logs
[group('heroku')]
heroku-logs:
    heroku logs --tail -a urbanist-atlas

# list app config (names + masked values)
[group('heroku')]
heroku-config:
    heroku config -a urbanist-atlas

# open an interactive shell inside a one-off dyno
[group('heroku')]
heroku-ssh:
    heroku run bash -a urbanist-atlas

# Runs against PROD data in a billed one-off dyno. Idempotent —
# every loader upserts by stable key, so re-runs converge rather than
# duplicate. Use after a seed-data edit lands on main.
[group('heroku')]
[doc('re-seed the LIVE database (one-off dyno; idempotent upserts)')]
heroku-loaddata:
    heroku run urbanist-atlas-server loaddata -a urbanist-atlas

# capture an on-demand Postgres backup; `heroku pg:backups` shows retention
[group('heroku')]
db-backup:
    heroku pg:backups:capture -a urbanist-atlas

# ── smoke: live curl helpers (server must be running) ─

# curl /healthz against localhost
[group('smoke')]
healthz port='8080':
    @curl -sS -i "http://localhost:{{port}}/healthz" | sed -n '1,8p'

# curl /api/v1/lookup, pretty-printed via jq
# usage: just lookup 11217  (or `just lookup M5V CA`)
[group('smoke')]
[doc('curl /api/v1/lookup and jq the body; e.g. `just lookup 11217` or `just lookup M5V CA`')]
lookup code country='US' port='8080':
    @curl -sS "http://localhost:{{port}}/api/v1/lookup?postal_code={{code}}&country={{country}}" | jq

# ── ci-equivalent ─────────────────────────────────────

# run every check CI would run today against the current tree
[group('ci')]
ci: api-check web-check
