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
the repo root provisions everything (Go, Node, and later sqlc / goose /
staticcheck — pinned in `mise.toml`).

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

## Tech conventions

### Go (`api/`)

- **Standard library first.** Pull a dependency only when stdlib genuinely
  can't do it. Approved exceptions:
  - `github.com/go-chi/chi/v5` — HTTP router
  - `github.com/urfave/cli/v3` — CLI / startup
  - `github.com/jackc/pgx/v5` — Postgres driver (used via sqlc)
  - `github.com/sqlc-dev/sqlc` — type-safe SQL codegen
  - `github.com/pressly/goose/v3` — migrations, embedded as a library
  - `gopkg.in/yaml.v3` — YAML loading for seed data
  - `github.com/google/go-cmp/cmp` — diff-friendly test assertions
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
- **Lint/format:** ESLint (official React + TS configs) + Prettier.
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

- `regions` (city / county / metro / state / province / multi-state) with a
  `scope_tier` of `local` or `regional` that drives result grouping.
- `postal_codes` maps US ZIPs and Canadian FSAs (3-char) to region IDs.
- `organizations` joined many-to-many to `regions` via
  `organization_regions`.
- `submissions` for the public submission queue, with bearer-token-gated
  admin endpoints to approve/reject.

## API surface

All under `/api/v1/`. JSON only. CORS allows `urbanistatlas.com`,
`*.pages.dev`, and `localhost:5173`. Admin endpoints use a bearer token
from env.

## Hosting

- **API:** Fly.io. Single Dockerfile, single binary, Fly Managed Postgres.
- **Web:** Cloudflare Pages connected to `web/`. PR preview deploys per
  branch.
