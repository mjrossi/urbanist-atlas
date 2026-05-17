# Urbanist Atlas — project conventions

This file is the contract for how code in this repo is written. Read it
before making non-trivial changes. The full approved design lives in
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (maintainer's
machine); the load-bearing pieces are summarized here so this repo is
self-explanatory.

## What this is

A directory of transit + safe-streets advocacy organizations, searchable
by US ZIP or Canadian postal code. Two halves:

- `api/` — Go service on Fly.io, Postgres-backed, exposes `/api/v1`.
- `web/` — React + Vite SPA on Cloudflare Pages.

Companion to the maintainer's publication, *Urbanist Lexicon*
(mjrossi.com). Visual language deliberately mirrors that site.

## Scope (v1)

- Transit + safe-streets organizations only. Housing/YIMBY is out of scope.
- US + Canada from day one.
- Results return **local** (city/county) + **regional** (metro/state/
  province/multi-state) orgs. No national orgs in the default lookup.

## Tooling: mise

Language runtimes and project tools are managed by
[mise](https://mise.jdx.dev). Install mise once, then `mise install` at
the repo root provisions everything pinned in `mise.toml`: Go, Node,
sqlc, goose, staticcheck, oapi-codegen.

- **`mise.toml`** — base: tool versions + production-default env.
- **`mise.development.toml`** — local dev overrides; activate with
  `MISE_ENV=development` in your shell.
- **`mise.ci.toml`** — CI overrides; the workflow sets `MISE_ENV=ci`.
- **`mise.local.toml`** — gitignored, for machine-specific overrides.
  See `mise.local.toml.example`.

Add new tooling by editing `mise.toml` rather than instructing
contributors to `brew install` things. The goal: a contributor with mise
and a clone of the repo has everything they need to run tests and the dev
server.

Postgres for the dev loop runs in a docker container, lifecycled by
`just pg-up` / `pg-down` / `pg-reset` / `pg-shell` / `pg-logs` on port
`55432`. Same `postgres:17-alpine` image as the testcontainers-based
integration suite, so the wire is identical. Docker is the only dev
dependency that isn't installed by mise.

## Tech conventions

### Go (`api/`)

- **Standard library first.** Pull a dependency only when stdlib genuinely
  can't do it. Approved exceptions:
  - `github.com/go-chi/chi/v5` — HTTP router
  - `github.com/urfave/cli/v3` — CLI / startup
  - `github.com/jackc/pgx/v5` — Postgres driver (used via sqlc)
  - `github.com/sqlc-dev/sqlc` — type-safe SQL codegen
  - `github.com/pressly/goose/v3` — migrations, embedded as a library
  - `github.com/oapi-codegen/oapi-codegen/v2` — Go types generated from `api/openapi.yaml` (types-only; no chi-server stubs)
  - `github.com/pelletier/go-toml/v2` — TOML loading for hand-curated seed data (regions + orgs)
  - `github.com/google/go-cmp/cmp` — diff-friendly test assertions
  - `github.com/testcontainers/testcontainers-go` — Postgres integration tests (test-only, under `//go:build integration`)
- **Logging:** `log/slog` (stdlib). JSON in prod, text in dev.
- **Errors:** stdlib `errors` + `fmt.Errorf("...: %w", err)`. No third-party
  errors libraries.
- **Config:** all via urfave/cli flags with env-var fallbacks
  (`URBANIST_DB_URL`, `URBANIST_ADMIN_TOKEN`, `URBANIST_PORT`, etc.). No `viper`.
- **Layout:** standard. `cmd/` for binaries, `pkg/` for the public library,
  `internal/` for non-exported.
- **Style:** `gofmt`, `go vet`, `staticcheck`. No custom linter config.
- **Module path:** `github.com/mjrossi/urbanist-atlas/api`.

The Go side is **library-first**: `pkg/atlas` is the importable surface,
and `cmd/server` is a thin urfave/cli wrapper around it. A future
`cmd/atlas` end-user CLI is anticipated and the package design must
support it. Handlers in `internal/httpapi/` should be ~10 lines each:
parse request → call a `pkg/atlas` function → encode response. No
business logic in handlers.

`serve` accepts `--store=memory|postgres` (postgres default) and
`--db-url` (with `URBANIST_DB_URL` env fallback). The memory store
stays available for tests and offline CLI use.

### React (`web/`)

Sensible community defaults for a 2026 app. **Confirm with the maintainer
before adding any library not in this list.**

- **Language:** TypeScript, strict mode.
- **Build:** Vite.
- **Routing:** `react-router` v7 (SPA / data mode).
- **Server state / data fetching:** `@tanstack/react-query` v5. One
  `useQuery` per endpoint. No global client-state library.
- **Forms:** `react-hook-form` for the submission form. Plain state for
  trivial inputs.
- **Styling:** plain CSS via `src/styles/global.css`, ported from
  mjrossi.com. Custom properties for design tokens. No Tailwind, no
  CSS-in-JS, no CSS modules in v1.
- **Fonts:** Fraunces (display), Source Serif 4 (body), Inter (UI), all via
  `@fontsource-variable/*`. Self-hosted, no external font requests.
- **Tests:** Vitest + React Testing Library.
- **Lint/format:** ESLint (flat config: `@eslint/js` + `typescript-eslint` +
  `eslint-plugin-react-hooks` + `eslint-plugin-react-refresh`) + Prettier
  (config inline in `package.json`'s `"prettier"` key).
- **Codegen:** `openapi-typescript` generates `src/lib/api.gen.ts` from
  `../api/openapi.yaml` via the `generate:api` npm script. All wire
  types in `src/lib/api.ts` import from there — **never hand-rolled**.
- **Package manager:** `npm`.

## Design language

The visual identity belongs to the *Urbanist Lexicon* family:

- Warm broadsheet palette (oklch warm off-white, amber accents `#8f5520` /
  `#c97d3e`)
- Newspaper masthead with double-rule and italic tagline between rules
- Small-caps section labels, drop caps on opening paragraphs
- Two-column "broadsheet body" for content-rich pages

The canonical CSS lives at
`~/dev/mjrossi-portfolio-website/src/styles/global.css`. When building UI
here, port the relevant slice rather than reinventing.

**Selected layouts:**
- Homepage: two-column broadsheet (search + lede on the left; "Browse by
  metro" and "Recently added" on the right).
- Results: classified-section list with explicit "Local" and "Regional"
  section labels; each entry is a row with name, description, tag chips,
  and outbound link.

## Data shape

See the plan for the full schema, but at a glance:

- `regions` form a directed acyclic graph (multi-parent allowed) with
  `scope_tier ∈ {local, regional}` driving result grouping. The
  taxonomy (kind, sort_priority, edges) is curated in
  `api/seed/regions_<cc>.toml`. See [`docs/region-graph.md`](./docs/region-graph.md)
  for modeling conventions and worked examples.
- `postal_codes` map postal codes to a single leaf region; ancestors
  are walked at lookup time via recursive CTE.
- `organizations` join many-to-many to `regions` via
  `organization_regions`; an org can attach to any node in the graph.
- `submissions` for the public submission queue, with bearer-token-
  gated admin endpoints.

## Wire contract

`api/openapi.yaml` is the source of truth for every request/response
shape, the error envelope, and admin-endpoint auth. Both halves
generate types from it:

- **Go:** `oapi-codegen` → `api/internal/httpapi/oapi/types.gen.go`
  (committed, regenerated via `just api-oapi-gen`). The spec is
  embedded into the binary via `//go:embed` and served at
  `GET /api/v1/openapi.yaml` (content-type `application/yaml`) so
  external consumers can discover the contract at runtime. Because
  `//go:embed` can't reach across package directories, a synchronized
  copy lives at `api/internal/httpapi/openapi.yaml`; a `//go:generate`
  directive keeps it in sync and `just api-check` fails if either
  copy drifts.
- **TS:** `openapi-typescript` → `web/src/lib/api.gen.ts` (committed,
  regenerated via `npm run generate:api`).

Spec edits are committed in their own PRs; both halves regenerate from
the new spec before the next feature commit.

## API surface

All under `/api/v1/` (with `/healthz` at the root). Success responses
are `application/json`; error responses are
[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)
`application/problem+json` with stable `type` URIs under
`https://urbanistatlas.com/problems/{slug}` and a `request_id`
extension echoing the `X-Request-ID` header. CORS allows
`urbanistatlas.com`, `*.pages.dev`, and `localhost:5173`. Admin
endpoints use a bearer token from `URBANIST_ADMIN_TOKEN`.

## Hosting

- **API:** Fly.io. Single Dockerfile, single binary, Fly Managed Postgres.
- **Web:** Cloudflare Pages connected to `web/`. PR preview deploys per
  branch.

## Launch strategy

The API ships in two phases — see roadmap slices #22–#28 for the
implementation slices.

- **Phase 1 — locked-down dogfooding (launch state).** CORS allowlist
  is restricted to `urbanistatlas.com` + `*.pages.dev`. A shared
  `X-Atlas-Client` secret header (bundled into the frontend build via
  `VITE_API_CLIENT_SECRET`, checked by the backend against
  `URBANIST_CLIENT_SECRET`) keeps casual scrapers out. Only `/healthz`
  and `/api/v1/openapi.yaml` are exempt. Goal: shake out schema +
  query bugs in a low-stakes window.
- **Phase 2 — public free-key (target state).** Self-serve free API
  keys (`api_keys` table, email-verified registration), tiered
  rate-limiting (tight for anonymous IP, generous for keyed),
  telemetry on key usage. CORS opens up, shared-secret middleware
  comes off.
- **Ongoing — ODbL attribution.** Every `/api/v1/**` success response
  carries `X-Data-License: ODbL-1.0` + `X-Data-Attribution` headers
  *and* a `meta` envelope on collection responses (`license`,
  `attribution_url`, `generated_at`) so downstream consumers see the
  share-alike obligation in-band. Source of truth for the dataset
  license is `LICENSE-DATA` at the repo root.
