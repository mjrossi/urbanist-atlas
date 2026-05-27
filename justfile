# urbanist-atlas — common dev commands.
# Run `just` (no args) to list recipes, organized by group.
#
# `just` itself is pinned in mise.toml (`aqua:casey/just`); a single
# `mise install` at the repo root provisions it alongside go, node,
# sqlc, goose, oapi-codegen, and staticcheck.
#
# Groups: api, data, verify, postgres, web, preview, fly, smoke, ci.
# Each group corresponds to a section comment below.

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

# fail if any Go file would be rewritten by gofmt. `gofmt -l` lists
# offenders and exits 0 even on drift, so the explicit non-empty
# check turns that into a CI signal.
[group('api')]
[doc('fail if any Go file is not gofmt-clean')]
api-fmt-check:
    @cd api && drift="$(gofmt -l .)"; [ -z "$drift" ] || { echo "gofmt drift:"; echo "$drift"; echo "run \`just api-fmt\` and commit." >&2; exit 1; }

# go vet ./...
[group('api')]
api-vet:
    cd api && go vet ./...

# staticcheck ./... — pinned in mise.toml. Catches bugs `go vet`
# misses (unused fields, deprecated APIs, ineffective assignments).
[group('api')]
[doc('run staticcheck (mise-pinned) over the api module')]
api-staticcheck:
    cd api && mise exec -- staticcheck ./...

# go test ./... with race detector, no cache (matches CI)
[group('api')]
api-test:
    cd api && go test ./... -race -count=1

# fmt-check + vet + staticcheck + test + gen-no-diff — the CI gate for
# api/, run locally before pushing.
[group('api')]
api-check: api-fmt-check api-vet api-staticcheck api-test api-gen-check

# go mod tidy
[group('api')]
api-tidy:
    cd api && go mod tidy

# regenerate every codegen artifact: oapi-codegen Go types, the
# embedded copy of openapi.yaml, and sqlc bindings. All three flow
# through `go generate ./...` via //go:generate directives, so adding
# a new generated artifact is just a matter of dropping another
# directive on the right file.
[group('api')]
[doc('regenerate all codegen (oapi-codegen + embedded spec + sqlc)')]
api-gen:
    cd api && mise exec -- go generate ./...

# run the link checker over the seed orgs, write to /tmp/links.tsv
[group('api')]
[doc('check website_url for every seed org and write a TSV report')]
linkcheck:
    cd api && go run ./cmd/server linkcheck --src ./seed/orgs.toml --out /tmp/links.tsv
    @echo "report: /tmp/links.tsv"

# run the postgres-backed integration tests under the `integration`
# build tag (requires Docker). Cheap default test suite stays
# tag-free so `just api-test` keeps running on machines without
# Docker.
[group('api')]
[doc('run postgres integration tests (Docker; build tag: integration)')]
api-test-integration:
    cd api && go test -tags=integration -race -count=1 ./internal/store/postgres/...

# fail if any generated file would change after a fresh regen. One
# `go generate ./...` covers all three artifacts via the
# //go:generate directives in oapi/doc.go, httpapi/openapi_handler.go,
# and postgres/gen.go. Used inside api-check so `just ci` rejects
# commits that forgot to regenerate.
[group('api')]
[doc('fail if any generated file would drift after a regen')]
api-gen-check:
    @cd api && mise exec -- go generate ./...
    @cd api && git diff --exit-code -- \
        internal/httpapi/oapi/types.gen.go \
        internal/httpapi/openapi.yaml \
        internal/store/postgres/gen/ \
        || (echo "generated files drifted; run \`just api-gen\` and commit." && exit 1)

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
# exact same orchestration the Fly deploy uses
# (flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server loaddata").
# The country list lives in api/internal/loaddata/loaddata.go — add
# new countries there, not here.
[group('data')]
[doc('load every bundled fixture in dependency order (regions → postal → orgs)')]
loaddata:
    cd api && go run ./cmd/server loaddata

# fetch upstream Census/StatsCan source files into etl/sources/<country>/
# and validate checksums against etl/SOURCES.md. Foundation slice
# (#7.5.1) ships this as a no-op stub; concrete US/CA plans land in
# slices #7.5.3 / #7.5.4.
[group('data')]
[doc('etl: fetch upstream source files for a country (e.g. `just etl-download US`)')]
etl-download country:
    cd api && go run ./cmd/server etl download --country {{country}} --src ../etl/sources

# regenerate seed TOML/CSV under api/seed/ from staged etl/sources/
# data. Reproducible — same upstream vintage produces byte-identical
# output. No-op stub until #7.5.3/#7.5.4.
[group('data')]
[doc('etl: regenerate seed files from staged sources (e.g. `just etl-regenerate US`)')]
etl-regenerate country:
    cd api && go run ./cmd/server etl regenerate --country {{country}} --src ../etl/sources --out seed

# ── verify: out-of-band data hygiene checks ───────────
# These probe third-party URLs from the seed data. Because a handful
# of advocacy-org domains have lapsed and now resolve to parked
# domains that log requester IPs, the recipes egress via a gluetun
# container speaking Proton WireGuard. Set WIREGUARD_PRIVATE_KEY in
# mise.local.toml [env] before first run (see mise.local.toml.example).

# probe every website_url + contact_url in api/seed/orgs.toml through
# the VPN; writes tmp/org-url-report.{md,tsv}. Leaves gluetun running
# so re-runs are fast — `just verify-org-urls-down` when you're done.
[group('verify')]
[doc('probe every seed org website_url + contact_url via VPN; writes tmp/org-url-report.{md,tsv}')]
verify-org-urls:
    @mkdir -p tmp
    docker compose -f scripts/verify-org-urls.compose.yml --profile verify run --rm --build verify

# stop the gluetun VPN container brought up by `verify-org-urls`.
# The verify container itself is removed by --rm each run; gluetun
# is what lingers.
[group('verify')]
[doc('stop the gluetun VPN container used by verify-org-urls')]
verify-org-urls-down:
    docker compose -f scripts/verify-org-urls.compose.yml down

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

# ── preview: full-stack PR review against a local stack ─
# Cloudflare Workers preview URLs target the *current* QA API at
# qa-api.urbanistatlas.com — they don't see the API changes on a
# PR's branch. For a PR that adds or changes an API endpoint, the
# preview frontend will 404 against the not-yet-deployed backend.
#
# `just preview` is the local-stack alternative: brings up the dev
# Postgres, applies any pending migrations, seeds the DB if empty,
# and runs the API in the foreground. Reviewer is expected to be on
# the PR branch already (e.g. via `gh pr checkout <PR#>`) and to run
# `just web-dev` in a second terminal. See CONTRIBUTING.md
# "Full-stack PR review" for the workflow.
#
# Idempotent: pg-up reuses an existing container, migrate-up is a
# no-op when migrations are current, and loaddata is skipped when
# the orgs table is already populated.
[group('preview')]
[doc('one-shot local stack for full-stack PR review (DB + migrate + seed-if-empty + api)')]
preview:
    @just pg-up
    @just migrate-up
    @# Fail fast if the dev DB container isn't reachable, instead of
    @# letting `|| echo 0` silently fall through to a confusing
    @# `just loaddata` failure one indirection later.
    @if ! docker exec urbanist-atlas-pg psql -U urbanist -d urbanist_atlas_dev -c "SELECT 1" >/dev/null 2>&1; then \
        echo "preview: postgres container 'urbanist-atlas-pg' is not reachable. Check 'docker ps' and 'just pg-up'." >&2; \
        exit 1; \
    fi
    @count=$(docker exec urbanist-atlas-pg psql -U urbanist -d urbanist_atlas_dev -t -A -c "SELECT COUNT(*) FROM organizations" 2>/dev/null || echo 0); \
    if [ "$count" = "0" ]; then \
        echo "preview: empty DB — running loaddata"; \
        just loaddata; \
    else \
        echo "preview: $count orgs already loaded — skipping loaddata"; \
    fi
    @echo ""
    @echo "preview: API starting on http://localhost:8080"
    @echo "preview: in another terminal, run \`just web-dev\` (http://localhost:5173)"
    @echo ""
    just api-run

# ── fly: deploy + ops ─────────────────────────────────
# Thin wrappers around `flyctl` so the deploy / logs / secrets /
# ssh verbs are discoverable via `just --list`. App names
# (urbanist-atlas for the API, urbanist-atlas-db for the sibling
# Postgres) are pinned via -a so these work from any branch
# without flyctl config. Initial provisioning (app creation, volume,
# secrets, DNS, certs) lives in docs/deploy.md — these recipes
# are for ongoing ops once both apps exist.

# deploy the API app from the current checkout. Release-command in
# fly.toml runs `migrate up` before the new machine takes traffic.
#
# Manual fallback: every merge to main triggers a Fly deploy from the
# `deploy-api` job in .github/workflows/ci.yml using
# `flyctl deploy --remote-only`. Use this recipe when GitHub Actions
# is degraded, when you need to deploy a non-main branch for a
# hot-fix, or when you want to watch the build locally. For an
# Actions-side re-deploy of current main without an empty commit,
# `gh workflow run ci.yml --ref main`.
[group('fly')]
[doc('deploy the API to Fly — manual fallback; primary path is GHA on merge to main')]
fly-deploy:
    flyctl deploy -a urbanist-atlas

# deploy the sibling Postgres app. Rarely needed after first launch;
# the image rolls forward when we bump postgres:17-alpine to a newer
# patch, which is an explicit maintenance decision.
[group('fly')]
[doc('deploy the sibling Postgres app (rare; only on image bumps)')]
fly-deploy-db:
    flyctl deploy -a urbanist-atlas-db -c infra/postgres/fly.toml

# tail live Fly logs (API)
[group('fly')]
fly-logs:
    flyctl logs -a urbanist-atlas

# tail live Fly logs (DB)
[group('fly')]
fly-logs-db:
    flyctl logs -a urbanist-atlas-db

# list app secrets (names + digests; values are write-only)
[group('fly')]
fly-secrets:
    flyctl secrets list -a urbanist-atlas

# open an interactive shell inside a running API machine
[group('fly')]
fly-ssh:
    flyctl ssh console -a urbanist-atlas

# Runs against PROD data in a one-off ssh session. Idempotent —
# every loader upserts by stable key, so re-runs converge rather than
# duplicate. Use after a seed-data edit lands on main.
[group('fly')]
[doc('re-seed the LIVE database (flyctl ssh; idempotent upserts)')]
fly-loaddata:
    flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server loaddata"

# capture an on-demand Postgres backup to a local file. Same pipeline
# as the nightly GHA cron workflow at .github/workflows/backup.yml,
# but writes locally rather than uploading to R2 — for ad-hoc
# snapshots the maintainer wants in hand before a risky change.
[group('fly')]
[doc('on-demand local pg_dump via flyctl ssh (writes ./urbanist-atlas-YYYY-MM-DD.sql.gz)')]
db-backup:
    @out="urbanist-atlas-$(date -u +%Y-%m-%d).sql.gz"; \
    flyctl ssh console -a urbanist-atlas-db \
        -C "sh -c 'pg_dump -U urbanist urbanist_atlas | gzip -c'" \
        > "$out" && \
    test -s "$out" && \
    ls -lh "$out"

# Restore a previously captured backup into the LIVE Postgres. Destructive;
# the operator confirms by passing the dump path explicitly.
# usage: just db-restore ./urbanist-atlas-2026-05-21.sql.gz
[group('fly')]
[doc('restore a .sql.gz dump into the LIVE Postgres (destructive)')]
db-restore dump:
    @echo "→ Restoring {{dump}} into urbanist-atlas-db (DESTRUCTIVE)"
    @gunzip -c {{dump}} | flyctl ssh console -a urbanist-atlas-db \
        -C "psql -U urbanist -d urbanist_atlas"

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

# End-to-end smoke against the LIVE QA endpoint. The script lives at
# scripts/smoke.sh so CI can invoke it directly without needing `just`
# on the runner. Requires URBANIST_CLIENT_SECRET in the environment
# (or pass as a positional arg).
# usage: URBANIST_CLIENT_SECRET=... just smoke
#        or: just smoke <secret> [host]
[group('smoke')]
[doc('e2e smoke against qa-api.urbanistatlas.com (set URBANIST_CLIENT_SECRET first)')]
smoke secret='' host='qa-api.urbanistatlas.com':
    ./scripts/smoke.sh "{{secret}}" "{{host}}"

# ── ci-equivalent ─────────────────────────────────────

# run every check CI would run today against the current tree
[group('ci')]
ci: api-check web-check
