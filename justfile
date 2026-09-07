# urbanist-atlas — common dev commands.
# Run `just` (no args) to list recipes, organized by group.
#
# `just` itself is pinned in mise.toml (`aqua:casey/just`); a single
# `mise install` at the repo root provisions it alongside go, node,
# oapi-codegen, and golangci-lint.
#
# Groups: api, data, verify, web, preview, fly, submissions, smoke, ci.
# Each group corresponds to a section comment below.

set shell := ["bash", "-cu"]

# ── default ───────────────────────────────────────────

# show available recipes, organized by group
[private]
default:
    @just --list --unsorted

# ── api: build & verify ───────────────────────────────

# run the API server with text logs
# Pins --seed-dir and --db-path to absolute paths via justfile_directory()
# so the recipe survives any URBANIST_* env state in the contributor's
# shell (mise.development.toml carries the same values for direct
# `go run` invocations; flags override env per urfave/cli).
[group('api')]
api-run:
    cd api && go run ./cmd/server serve --log-format=text \
        --seed-dir={{justfile_directory()}}/api/seed \
        --db-path={{justfile_directory()}}/api/tmp/atlas.db

# build the api binary to api/bin/urbanist-atlas-server
[group('api')]
api-build:
    cd api && mkdir -p bin && go build -o bin/urbanist-atlas-server ./cmd/server

# format Go code (gofumpt + goimports, configured in api/.golangci.yml)
[group('api')]
api-fmt:
    cd api && mise exec -- golangci-lint fmt

# golangci-lint v2 over the api module (config: api/.golangci.yml). Bundles
# govet + staticcheck + the gofumpt/goimports format-drift check alongside
# the curated standardization linter set, so it is the single api/ lint gate
# (it subsumes the old api-vet / api-staticcheck / api-fmt-check recipes).
[group('api')]
[doc('run golangci-lint (mise-pinned) over the api module')]
api-lint:
    cd api && mise exec -- golangci-lint run

# golangci-lint run --fix — apply every auto-fixable finding (and the
# gofumpt/goimports formatting) in place.
[group('api')]
[doc('apply golangci-lint auto-fixes over the api module')]
api-lint-fix:
    cd api && mise exec -- golangci-lint run --fix

# go test ./... with race detector, no cache (matches CI)
[group('api')]
api-test:
    cd api && go test ./... -race -count=1

# lint + test + gen-no-diff — the CI gate for api/, run locally before
# pushing. golangci-lint (api-lint) covers vet, staticcheck, and the
# gofumpt/goimports format-drift check in one pass.
[group('api')]
api-check: api-lint api-test api-gen-check

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

# fail if any generated file would change after a fresh regen. One
# `go generate ./...` covers both artifacts via the //go:generate
# directives in oapi/doc.go, httpapi/openapi_handler.go, and
# store/sqlite/generate.go. Used inside api-check so `just ci` rejects
# commits that forgot to regenerate.
[group('api')]
[doc('fail if any generated file would drift after a regen')]
api-gen-check:
    @cd api && mise exec -- go generate ./...
    @cd api && git diff --exit-code -- \
        internal/httpapi/oapi/types.gen.go \
        internal/httpapi/openapi.yaml \
        internal/store/sqlite/gen \
        || (echo "generated files drifted; run \`just api-gen\` and commit." && exit 1)

# ── data: operational subcommands ─────────────────────
# The TOML/CSV files under api/seed/ are the runtime source of truth
# for orgs/regions/postal — the server loads them into an in-memory
# FileStore at boot, so editing a seed file + redeploying is the
# whole data-update workflow. The historical loaddata/migrate/seed
# recipes are gone with the Postgres read-path retirement.

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

# Pre-deploy gate (HOST-01b): load the EMBEDDED seed bundle via the same
# BuildMemStore loader the server boots from and fail on any error — a
# dangling org region_slug, a cross-file DAG cycle, an orphan leaf. Unlike
# seed-check (region files only, network + tree mutation) this is offline
# and side-effect-free, so it CAN run in `just ci` and as its own CI job
# that deploy-api depends on. Covers the hand-curated orgs.toml + leaves
# seed-check never loads.
[group('data')]
[doc('fail if the embedded seed bundle does not load cleanly (pre-deploy gate)')]
seed-validate:
    cd api && go run ./cmd/server seed validate

# Fail if the committed US/CA region files drift from a fresh regenerate.
# Mirrors api-gen-check / web-gen-check. Runs FULLY OFFLINE: it regenerates
# from the minimal regions sources committed under etl/fixtures/ (the Census
# CBSA list as CSV + a DBF-only CMA zip), NOT from a network download.
# StatsCan (www12.statcan.gc.ca) is unreachable from GitHub Actions, which
# made the download-based gate chronically flaky; the regions pass only
# needs the 29 KB CMA attribute table, so it is vendored. The postal CSVs
# (HUD-gated US, 155 MB CA FSA) stay covered by the golden determinism tests
# under `just api-test`, not here. Refresh the fixtures with `just
# seed-fixtures` alongside a vintage bump — see etl/fixtures/README.md.
[group('data')]
[doc('fail if committed region seed drifts from an offline regen of etl/fixtures/')]
seed-check:
    @cd api && go run ./cmd/server etl regenerate --country US --target regions --src ../etl/fixtures --out seed
    @cd api && go run ./cmd/server etl regenerate --country CA --target regions --src ../etl/fixtures --out seed
    @git diff --exit-code -- api/seed/regions_us_msas.toml api/seed/regions_ca_cmas.toml \
        || (echo "seed drift: region files differ from a fresh regen of etl/fixtures/. Regenerate from your staged sources (\`just etl-regenerate US\` / \`CA\`), and if the upstream vintage changed also run \`just seed-fixtures\`; then commit." && exit 1)

# Rebuild the committed offline regions fixtures (etl/fixtures/) from the
# full staged sources under etl/sources/. Run after a vintage bump so
# `seed-check` regenerates from current data. The CA fixture is a minimal
# zip of just the CMA DBF (the geometry the 13 MB upstream zip also carries
# is postal-only). Needs the same staged sources `just etl-download` fetches
# plus openpyxl for the xlsx→csv step. See etl/fixtures/README.md.
[group('data')]
[doc('rebuild etl/fixtures/ (offline seed-check inputs) from staged etl/sources/')]
seed-fixtures:
    @mkdir -p etl/fixtures/us etl/fixtures/ca
    @python3 etl/scripts/xlsx_to_csv.py etl/sources/us/list1_2023.xlsx etl/fixtures/us/list1_2023.csv
    @tmp=$(mktemp -d); \
        unzip -o etl/sources/ca/lcma000b21a_e.zip lcma000b21a_e.dbf -d "$tmp" >/dev/null; \
        rm -f etl/fixtures/ca/lcma000b21a_e.zip; \
        ( cd "$tmp" && zip -X -j "{{justfile_directory()}}/etl/fixtures/ca/lcma000b21a_e.zip" lcma000b21a_e.dbf >/dev/null ); \
        rm -rf "$tmp"
    @echo "rebuilt etl/fixtures/{us/list1_2023.csv, ca/lcma000b21a_e.zip} from etl/sources/"

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

# check formatting without writing (prettier --check)
[group('web')]
web-format-check:
    cd web && npm run format:check

# auto-format the web sources (prettier --write)
[group('web')]
web-format:
    cd web && npm run format

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
[doc('deps + lint + format + test + build + gen-no-diff — CI gate for web/')]
web-check: web-deps web-lint web-format-check web-test web-build web-gen-check

# ── preview: full-stack PR review against a local stack ─
# Cloudflare Workers preview URLs target the *current* production
# API at api.urbanistatlas.com — they don't see the API changes on
# a PR's branch. For a PR that adds or changes an API endpoint, the
# preview frontend will 404 against the not-yet-deployed backend.
#
# `just preview` is the local-stack alternative: runs the API in the
# foreground against the file-backed store (api/seed/ is the runtime
# source of truth, no DB to start). Reviewer is expected to be on
# the PR branch already (e.g. via `gh pr checkout <PR#>`) and to run
# `just web-dev` in a second terminal.
[group('preview')]
[doc('one-shot local stack for full-stack PR review (file store + api)')]
preview:
    @echo "preview: API starting on http://localhost:8080 (file store)"
    @echo "preview: in another terminal, run \`just web-dev\` (http://localhost:5173)"
    @echo ""
    just api-run

# ── fly: deploy + ops ─────────────────────────────────
# Thin wrappers around `flyctl` so the deploy / logs / secrets /
# ssh verbs are discoverable via `just --list`. The API runs as a
# single Fly app (urbanist-atlas) backed by an in-memory FileStore
# built from the api/seed/ bundle baked into the image — no
# database, no sibling app. Initial provisioning lives in
# docs/deploy.md; these recipes are for ongoing ops.

# deploy the API app from the current checkout.
#
# Manual fallback: every merge to main triggers a Fly deploy from the
# `deploy-api` job in .github/workflows/ci.yml using
# `flyctl deploy --remote-only`. Use this recipe when GitHub Actions
# is degraded, when you need to deploy a non-main branch for a
# hot-fix, or when you want to watch the build locally.
[group('fly')]
[doc('deploy the API to Fly — manual fallback; primary path is GHA on merge to main')]
fly-deploy:
    flyctl deploy -a urbanist-atlas

# tail live Fly logs
[group('fly')]
fly-logs:
    flyctl logs -a urbanist-atlas

# list app secrets (names + digests; values are write-only)
[group('fly')]
fly-secrets:
    flyctl secrets list -a urbanist-atlas

# open an interactive shell inside a running API machine
[group('fly')]
fly-ssh:
    flyctl ssh console -a urbanist-atlas

# ── submissions: admin queue ops ──────────────────────
# Thin curl wrappers around GET/POST /api/v1/admin/submissions so
# triage doesn't require remembering bearer-auth invocations. All
# three HTTP recipes need ONE secret in the environment, matching the
# Fly secret of the same name (set it in mise.local.toml or your shell):
#   - URBANIST_ADMIN_TOKEN — bearer token for /api/v1/admin/*
# They used to also require URBANIST_CLIENT_SECRET and send an
# X-Atlas-Client header. That is no longer needed: /api/v1/admin/*
# sits outside the phase-1 client-secret gate (design decision D10),
# so the bearer alone is necessary and sufficient, and demanding both
# only broke callers holding the stronger credential.
# `base` defaults to the deployed API; pass `http://localhost:8080`
# to drive a local server instead.

# list submissions, default status=pending. Pass `approved` to see
# `promotion_pr_url` / `promotion_error` for already-actioned rows.
# usage: just submissions-list
#        just submissions-list approved
#        just submissions-list pending http://localhost:8080
[group('submissions')]
[doc('GET /api/v1/admin/submissions (default status=pending)')]
submissions-list status='pending' base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN (e.g. via mise.local.toml or your shell)}"
    @curl -sS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
        "{{base}}/api/v1/admin/submissions?status={{status}}" | jq

# approve a pending submission; the API enqueues the GitHub-PR worker
# and returns the updated row. The PR URL lands on the row a few
# seconds later — re-run `submissions-list approved` to see it.
# usage: just submissions-approve <uuid>
[group('submissions')]
[doc('POST /api/v1/admin/submissions/{id}/approve (queues GitHub PR)')]
submissions-approve id base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN}"
    @curl -sS -X POST -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
        "{{base}}/api/v1/admin/submissions/{{id}}/approve" | jq

# reject a pending submission with a moderator-facing reason. The
# reason is JSON-encoded via jq so quotes/newlines pass through safely.
# usage: just submissions-reject <uuid> "duplicate of foo-bar org"
[group('submissions')]
[doc('POST /api/v1/admin/submissions/{id}/reject with a reason')]
submissions-reject id reason base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN}"
    @body="$(jq -nc --arg r "{{reason}}" '{reason: $r}')"; \
        curl -sS -X POST -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
            -H "Content-Type: application/json" -d "$body" \
            "{{base}}/api/v1/admin/submissions/{{id}}/reject" | jq

# re-run the GitHub PR worker for an approved submission whose first
# attempt failed (e.g. transient GitHub outage). Executes the
# `submissions retry-pr` CLI subcommand inside the Fly machine, since
# it needs filesystem access to /data/atlas.db and the bundled
# URBANIST_GITHUB_TOKEN secret. Use after spotting a non-empty
# `promotion_error` on an approved row via `submissions-list approved`.
# usage: just submissions-retry-pr <uuid>
[group('submissions')]
[doc('retry the GitHub PR worker for an approved submission (via flyctl ssh)')]
submissions-retry-pr id:
    flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server submissions retry-pr --id={{id}}"

# ── usage: rollup + coverage reads ────────────────────
# Thin curl wrappers around the bearer-gated reads behind the monthly
# digest, so answering "what are people actually looking at" does not
# mean hand-assembling a date range and a bearer header each time.
#
# Needs ONE secret in the environment (mise.local.toml or your shell):
#   - URBANIST_ADMIN_TOKEN — must match the Fly secret of the same name
#
# Unlike the submissions recipes above, these send no X-Atlas-Client:
# /api/v1/admin/* sits outside the phase-1 client-secret gate, so the
# bearer alone is necessary and sufficient (design decision D10).
#
# `days` is a trailing window ending today, UTC — the date arithmetic
# tries BSD `date -v` first and falls back to GNU `date -d`, so the
# recipes work on macOS and Linux alike. `base` defaults to the deployed
# API; pass http://localhost:8080 to read a local server's DB.
#
# Counts lag by up to the flush interval (60s), so a view you just made
# will not appear instantly. See docs/deploy.md §Usage digest.

# top buckets for one kind, summed over a trailing window.
# kinds: region_view org_view lookup lookup_tier lookup_result lookup_country
# usage: just usage-top
#        just usage-top org_view
#        just usage-top lookup 90 50
[group('usage')]
[doc('GET /api/v1/admin/usage — top keys for one kind over a trailing window')]
usage-top kind='region_view' days='30' limit='25' base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN (e.g. via mise.local.toml)}"
    @from="$(date -u -v-{{days}}d +%F 2>/dev/null || date -u -d '{{days}} days ago' +%F)"; \
        to="$(date -u +%F)"; \
        echo "{{kind}}  $from .. $to  (top {{limit}})"; \
        curl -sS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
            "{{base}}/api/v1/admin/usage?from=$from&to=$to&kind={{kind}}&group_by=key&limit={{limit}}" \
        | jq -r 'if length == 0 then empty else (.[] | [.count, .key] | @tsv) end' \
        | column -t; \
        echo

# per-day series for one kind, oldest first — for spotting a spike or a
# drop rather than a total. Same kinds as usage-top.
# usage: just usage-days region_view 14
[group('usage')]
[doc('GET /api/v1/admin/usage — per-day rows for one kind (group_by=day)')]
usage-days kind='region_view' days='14' limit='200' base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN}"
    @from="$(date -u -v-{{days}}d +%F 2>/dev/null || date -u -d '{{days}} days ago' +%F)"; \
        to="$(date -u +%F)"; \
        curl -sS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
            "{{base}}/api/v1/admin/usage?from=$from&to=$to&kind={{kind}}&group_by=day&limit={{limit}}" \
        | jq -r 'sort_by(.day)[] | [.day, .count, .key] | @tsv' | column -t

# the three bounded-enum kinds at a glance: is the data serving people?
# lookup_result says hit/miss, lookup_tier says which tier satisfied
# them, lookup_country says the US/CA mix. A rising `miss` share is the
# editorial signal to chase — cross-reference `just usage-gaps`.
# usage: just usage-summary
#        just usage-summary 90
[group('usage')]
[doc('lookup_result + lookup_tier + lookup_country totals over a window')]
usage-summary days='30' base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN}"
    @from="$(date -u -v-{{days}}d +%F 2>/dev/null || date -u -d '{{days}} days ago' +%F)"; \
        to="$(date -u +%F)"; \
        echo "lookup outcomes  $from .. $to"; \
        for kind in lookup_result lookup_tier lookup_country; do \
            echo; echo "  $kind"; \
            curl -sS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
                "{{base}}/api/v1/admin/usage?from=$from&to=$to&kind=$kind&group_by=key&limit=25" \
            | jq -r '.[] | [.count, .key] | @tsv' | column -t | sed 's/^/    /'; \
        done

# postal codes and searches that found nothing — the coverage backlog.
# Sampled and row-capped server-side, so this is a signal of where to
# curate next, not an exhaustive log.
# usage: just usage-gaps
#        just usage-gaps 200
[group('usage')]
[doc('GET /api/v1/admin/coverage-gaps — lookups that returned nothing')]
usage-gaps limit='50' base='https://api.urbanistatlas.com':
    @: "${URBANIST_ADMIN_TOKEN:?set URBANIST_ADMIN_TOKEN}"
    @curl -sS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
        "{{base}}/api/v1/admin/coverage-gaps?limit={{limit}}" | jq

# query Fly's managed Prometheus (the ~30-day latency/5xx window that
# the rollup table deliberately does NOT duplicate).
#
# The header is NOT `Bearer`: a Fly token is the string "FlyV1 fm2_...",
# where FlyV1 is itself the Authorization scheme, so wrapping it in
# Bearer nests one scheme inside another and Fly answers 401 with a
# perfectly good token. Needs in the environment:
#   - FLY_PROMETHEUS_TOKEN — full value incl. the "FlyV1 " prefix,
#     from `flyctl tokens create readonly -o <org> --expiry 8760h`
#   - FLY_ORG_SLUG — the Slug column of `flyctl orgs list` (not the name)
# usage: just fly-prom
#        just fly-prom 'sum(increase(atlas_http_requests_total[7d]))'
[group('fly')]
[doc('query Fly managed Prometheus (default: 30d non-probe request total)')]
fly-prom query='sum(increase(atlas_http_requests_total{route!~"/healthz|/readyz"}[30d]))':
    @: "${FLY_PROMETHEUS_TOKEN:?set FLY_PROMETHEUS_TOKEN (full value, incl. the 'FlyV1 ' prefix)}"
    @: "${FLY_ORG_SLUG:?set FLY_ORG_SLUG (the Slug column of \`flyctl orgs list\`)}"
    @curl -sS -G -H "Authorization: $FLY_PROMETHEUS_TOKEN" \
        --data-urlencode 'query={{query}}' \
        "https://api.fly.io/prometheus/$FLY_ORG_SLUG/api/v1/query" \
    | jq -r '.data.result[]? | .value[1]' 


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

# End-to-end smoke against the LIVE production endpoint. The script
# lives at scripts/smoke.sh so CI can invoke it directly without
# needing `just` on the runner. Requires URBANIST_CLIENT_SECRET in
# the environment (or pass as a positional arg).
# usage: URBANIST_CLIENT_SECRET=... just smoke
#        or: just smoke <secret> [host]
[group('smoke')]
[doc('e2e smoke against api.urbanistatlas.com (set URBANIST_CLIENT_SECRET first)')]
smoke secret='' host='api.urbanistatlas.com':
    ./scripts/smoke.sh "{{secret}}" "{{host}}"

# ── ci-equivalent ─────────────────────────────────────

# run the offline, side-effect-free checks CI would run against the tree.
# `seed-check` is deliberately NOT in here: it needs network (census.gov,
# statcan.gc.ca) and mutates the tree (downloads into etl/sources/,
# rewrites api/seed/regions_*.toml in place), so it would break `just ci`
# for an offline maintainer or clobber custom-staged sources. CI runs it
# as its own `data` job regardless; run `just seed-check` by hand when you
# want the drift gate locally.
[group('ci')]
ci: api-check web-check
