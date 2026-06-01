# Urbanist Atlas — project conventions

This file is the contract for how code in this repo is written. Read it
before making non-trivial changes. The full approved design lives in
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (maintainer's
machine); the load-bearing pieces are summarized here so this repo is
self-explanatory.

## What this is

A directory of transit + safe-streets advocacy organizations, searchable
by US ZIP or Canadian postal code. Two halves:

- `api/` — Go service on Fly.io, stateless (file-backed FileStore reads
  the TOML/CSV bundle at boot), exposes `/api/v1`.
- `web/` — React + Vite SPA on Cloudflare Workers + Pages (Static Assets).

Companion to the maintainer's publication, *Urbanist Lexicon*
(mjrossi.com). Visual language deliberately mirrors that site.

## Scope (v1)

- Transit + safe-streets organizations only. Housing/YIMBY is out of scope.
- **US + Canada are the only shipping countries in v1.** Slice #4.6
  loaded Portugal as a region-graph validation fixture to stress the
  multi-parent DAG model; slice #25 dropped PT from the user-facing
  seed loader (the default `countries` set in
  `api/internal/seedfiles/build.go`) once the validation had served
  its purpose. The PT seed files (`api/seed/regions_pt.toml`,
  `postal_codes_pt.csv`) stay in the repo as a reference and for the
  integration suite (which loads them explicitly), and a future
  v1.1+ slice can reintroduce PT — or ES, MX, NL, UK — when the
  editorial coverage is ready to ship. See
  [`docs/region-graph.md`](./docs/region-graph.md) for the per-country
  conventions.
- Results return three presentational tiers: **local** (city/county),
  **regional** (metro/CMA/transit-federation/multi-state), and
  **state / provincial** (state/province/territory *kinds* — `us:state`,
  `us:territory`, `ca:province`, `ca:territory`). The state/provincial
  split is derived from region *kind* (`atlas.IsStateKind`), not a new
  `scope_tier` value — the wire enum stays `{local, regional, national}`;
  the `/lookup` and region-detail responses carry a `statewide` array
  alongside `local` and `regional`. Multi-state coalitions stay in
  regional. DC (`us:federal-district`) is treated as a city-state and
  stays regional, not statewide — it's coextensive with one city/metro
  and is split across the `washington-dc` local leaf and the `dc`
  district node, so its orgs bucket local or regional by slug choice. The schema supports a third
  `scope_tier='national'` tier (slice #4.6) for country-wide umbrellas
  (e.g. Portugal's MUBi in the validation fixture, or future Living
  Streets / MUBi-equivalent national orgs when other countries
  reactivate); the default `/lookup` filters national-tier orgs out
  so the surface stays local-first. US/CA editorial policy forbids
  creating national regions in v1 seed data, preserving the
  no-national-orgs default behavior for those countries.

## Tooling: mise

Language runtimes and project tools are managed by
[mise](https://mise.jdx.dev). Install mise once, then `mise install` at
the repo root provisions everything pinned in `mise.toml`: Go, Node,
staticcheck, oapi-codegen.

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

The dev loop has no external dependencies: `mise install`, then
`just api-run` (which is `urbanist-atlas-server serve` against
`api/seed/`) is the entire local setup. No Postgres, no Docker.

## Tech conventions

### Go (`api/`)

- **Standard library first.** Pull a dependency only when stdlib genuinely
  can't do it. Approved exceptions:
  - `github.com/go-chi/chi/v5` — HTTP router
  - `github.com/urfave/cli/v3` — CLI / startup
  - `github.com/oapi-codegen/oapi-codegen/v2` — Go types generated from `api/openapi.yaml` (types-only; no chi-server stubs)
  - `github.com/pelletier/go-toml/v2` — TOML loading for hand-curated seed data (regions + orgs)
  - `github.com/google/go-cmp/cmp` — diff-friendly test assertions
- **Logging:** `log/slog` (stdlib). JSON in prod, text in dev.
- **Errors:** stdlib `errors` + `fmt.Errorf("...: %w", err)`. No third-party
  errors libraries.
- **Config:** all via urfave/cli flags with env-var fallbacks
  (`URBANIST_ADMIN_TOKEN`, `URBANIST_PORT`, `URBANIST_LOG_FORMAT`,
  `URBANIST_LOG_LEVEL`, `URBANIST_CORS_ORIGINS`, `URBANIST_STORE`,
  `URBANIST_SEED_DIR`, `URBANIST_CLIENT_SECRET`, etc.). No `viper`.
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

`serve` accepts `--store=file|memory` (file default) and `--seed-dir`
(default `./seed`; env `URBANIST_SEED_DIR`). The file store reads
the bundled TOML/CSV at boot into an in-memory `atlas.MemStore`; the
memory store loads the small `LoadDevFixtures` set and exists for
demos and ad-hoc CLI testing without a seed directory on disk.

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
- Results: classified-section list with explicit "Local", "Regional",
  and "State / Provincial" section labels; each entry is a row with
  name, description, tag chips, and outbound link. Empty sections render
  nothing.

## Data shape

See the plan for the full schema, but at a glance:

- `regions` form a directed acyclic graph (multi-parent allowed) with
  `scope_tier ∈ {local, regional, national}` driving result grouping.
  `national` is filtered from the default `/lookup` (see
  [`docs/region-graph.md`](./docs/region-graph.md) §5 for the
  per-country editorial policy). The taxonomy splits across multiple
  TOML files per country, loaded in dependency order (see
  [`api/seed/README.md`](./api/seed/README.md) for the canonical list):
    - `regions_<cc>_states.toml` / `_provinces.toml` — top-tier hand-defined
      states/provinces/territories (US: 52, CA: 13).
    - `regions_us_multistate.toml` — US multi-state advocacy regions
      and transit federations (nyc-tristate, chicagoland,
      rta-service-area).
    - `regions_us_msas.toml` / `regions_ca_cmas.toml` — generated by
      the `urbanist-atlas-server etl regenerate` subcommand from
      Census CBSA + StatsCan CMA boundary data (US: 393, CA: 41).
      Editorial overrides for slug/name/parents live in
      `regions_us_msa_overrides.toml` (US) and
      `api/internal/etl/ca/mappings.go` (CA).
    - `regions_<cc>.toml` — hand-curated city/borough/county leaves.
- Postal codes map to whatever the smallest curated region for that
  area is — a city leaf where one exists, an NYC borough leaf via
  county lookup, an MSA/CMA region, or finally the state/province.
  Anchor distribution is a *data* decision baked into
  `postal_codes_<cc>.csv` by the ETL pipeline; the in-memory ancestor
  walk in `MemStore.AncestorRegions` is unchanged across tiers — it
  walks ancestors of whatever region the postal code points at. The
  **US pipeline runs two sources**: Census ZCTA crosswalks (primary,
  ~33,700 ZIPs with city-leaf precision where curated) and HUD's
  quarterly USPS ZIP-County crosswalk (additive backfill for the ~9k
  operational ZIPs Census omits — P.O. Box-only, single-building,
  APO/FPO). See
  [`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./docs/superpowers/specs/2026-05-19-postal-coverage-design.md)
  for the smallest-anchor design rationale, two-source merge, and
  editorial conventions.
- Organizations attach to any node in the region graph (many-to-many
  via the `region_slugs` array in each `[[org]]` entry of
  `orgs.toml`). At boot the FileStore resolves those slugs to the
  in-memory region IDs.
- **Submissions:** the public submission queue lives in a small
  SQLite store at `/data/atlas.db` on a 1 GiB Fly volume (no
  Postgres). Public `POST /api/v1/submissions` is rate-limited per
  IP; admin `GET/POST /api/v1/admin/submissions/...` is bearer-gated
  (`URBANIST_ADMIN_TOKEN`). Approving a submission queues an async
  GitHub PR worker (`internal/githubpr/`) that appends the new
  `[[org]]` block to `api/seed/orgs.toml`. The PR is the
  editorial-review surface; on merge, the next API deploy ships
  the new bundle. See
  [`docs/superpowers/specs/2026-05-27-submissions-sqlite-design.md`](./docs/superpowers/specs/2026-05-27-submissions-sqlite-design.md)
  for the full design.

### ETL pipeline (operator-side)

The MSA/CMA region rows and the full postal-code datasets are
generated from upstream geographic reference data by the
`urbanist-atlas-server etl` subcommand. Concrete plans live in
`api/internal/etl/us/` and `api/internal/etl/ca/`. Workflow:

```sh
# Refresh upstream sources (manual, only when bumping vintages — see etl/SOURCES.md).
mise install
pip install -r etl/scripts/requirements.txt      # one time, for the xlsx→csv Python step

# Regenerate seed outputs from staged sources.
urbanist-atlas-server etl regenerate --country=US
urbanist-atlas-server etl regenerate --country=CA

# Restart the dev server — there is no DB to reload.
just api-run
```

Source files live under `etl/sources/<cc>/` (gitignored). Generated
outputs live under `api/seed/` and are committed. ETL is deterministic:
the same upstream vintage produces byte-identical output, so git diffs
are signal-rich.

## Wire contract

`api/openapi.yaml` is the source of truth for every request/response
shape, the error envelope, and admin-endpoint auth. Both halves
generate types from it:

- **Go:** `oapi-codegen` → `api/internal/httpapi/oapi/types.gen.go`
  (committed, regenerated via `just api-gen`). The spec is
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
`urbanistatlas.com`, `*.<account>.workers.dev`, and `localhost:5173`. Admin
endpoints use a bearer token from `URBANIST_ADMIN_TOKEN`.

## Hosting

- **API:** Fly.io, region `iad` (Virginia, US East). A single Fly app
  (`urbanist-atlas`); a multi-stage `Dockerfile` at the repo root
  builds the Go binary and bakes `api/seed/` into the image. The
  **read path** is stateless: at boot the binary loads the bundled
  TOML/CSV into an in-memory `atlas.MemStore` and serves all reads
  from memory. The **write path** (public submissions) lands in a
  small SQLite database at `/data/atlas.db` on a 1 GiB Fly volume
  (`atlas_data`) — see slice β design at
  [`docs/superpowers/specs/2026-05-27-submissions-sqlite-design.md`](./docs/superpowers/specs/2026-05-27-submissions-sqlite-design.md).
  Approved submissions open a GitHub PR appending the new org to
  `api/seed/orgs.toml`; the next deploy ships the merged bundle.
  The previous sibling `urbanist-atlas-db` Postgres app and its
  Postgres-era nightly backup workflow were retired when this
  changed; SQLite gets its own
  [`backup-sqlite.yml`](./.github/workflows/backup-sqlite.yml)
  nightly cron to R2.
- **Web:** Cloudflare Workers + Pages (Static Assets) configured by
  `wrangler.jsonc` at the repo root. The Vite build at `web/dist/`
  is uploaded as static assets; SPA fallback is handled by
  `assets.not_found_handling = "single-page-application"` (replaces
  the `_redirects` file Pages would have used). Production builds
  deploy via `npx wrangler deploy`; non-`main` branches use
  `npx wrangler versions upload` for versioned previews on
  `<version>-urbanist-atlas.<account>.workers.dev`.

See [`docs/superpowers/specs/2026-05-21-fly-deploy-design.md`](./docs/superpowers/specs/2026-05-21-fly-deploy-design.md)
for the original (Postgres-backed) design and
[`docs/deploy.md`](./docs/deploy.md) for the current runbook.

## Launch strategy

The API ships in two phases — see roadmap slices #22–#28 for the
implementation slices.

- **Phase 1 — locked-down dogfooding (launch state).** CORS allowlist
  is restricted to `urbanistatlas.com` + `*.<account>.workers.dev`. A shared
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
