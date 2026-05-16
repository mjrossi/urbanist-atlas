# Roadmap

Implementation slices to get Urbanist Atlas to v1. Each row is a
self-contained chunk that could be its own session or PR.

The architectural plan this list is derived from lives at
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (local to the
maintainer); the load-bearing pieces are mirrored into
[`CLAUDE.md`](../CLAUDE.md). This file is the *execution* view —
the plan is the *design* view.

## Status

**Done:**

- Monorepo bones (`api/`, `web/`, `.github/`, READMEs, `.gitignore`)
- `mise` tooling — `mise.toml` + dev/ci overlays + `mise.local.toml.example`
- CI workflow with auto-detect-and-gate logic so each half lights up
  as it gets scaffolded
- Go API skeleton against an in-memory store:
  - `pkg/atlas` — types, `Store` interface, `MemStore`, `Lookup`,
    `LoadDevFixtures`, tests
  - `cmd/server` — `urfave/cli` root + `serve` + stubs for
    `migrate`/`loadpostal`/`seed`
  - `internal/httpapi` — chi router, request-ID + recoverer + access
    log + CORS middleware, `/healthz`, `/api/v1/lookup`
- `justfile` with `api-*`, `migrate-*`, `seed`, `loadpostal`, `healthz`,
  `lookup`, and `ci` recipes

**Not yet started:** everything below.

## Backend (Go)

| # | Slice | What lands |
|---|-------|------------|
| 1 | **Postgres store** | `migrations/0001_init.sql` (regions, postal_codes, organizations, organization_regions, submissions); `internal/store/postgres/queries/*.sql` + `sqlc.yaml`; an adapter implementing `atlas.Store`; `serve --store=postgres` (in-memory becomes opt-in). |
| 2 | **`migrate` for real** | Replace the "not yet implemented" stub: embed goose + `migrations/*.sql` into the binary, expose `migrate up\|down\|status`. |
| 3 | **`loadpostal` for real** | Pick free sources (US Census ZCTA→CBSA crosswalk; StatsCan FSA), write the CSV ingester, populate `regions` + `postal_codes`. |
| 4 | **`seed` for real** | Generate `api/seed/orgs.yaml` (~30–50 LLM-drafted-then-human-reviewed orgs), write the YAML loader (`gopkg.in/yaml.v3`), wire the subcommand to upsert into Postgres. |
| 5 | **Submissions + admin queue** | `POST /api/v1/submissions` (rate-limited, optional honeypot/Turnstile); `GET /admin/submissions`, `POST /admin/submissions/{id}/approve\|reject` (bearer-token auth via `URBANIST_ADMIN_TOKEN`); the approval transaction promotes a submission row into an `organizations` row. |
| 6 | **Browse / recent endpoints** | `GET /api/v1/metros`, `GET /api/v1/metros/{slug}`, `GET /api/v1/recent` — feeds the homepage strip and `/browse`. |
| 7 | **Handler tests** | `httptest`-based integration tests for `/lookup`, `/submissions`, the admin endpoints. |

## Frontend (React + Vite)

| # | Slice | What lands |
|---|-------|------------|
| 8 | **SPA bootstrap** | `npm create vite@latest` (TS + strict), ESLint + Prettier config, `lib/api.ts` typed client, `QueryClient` + `RouterProvider` in `main.tsx`. |
| 9 | **Broadsheet CSS port** | Bring the relevant slice of `mjrossi.com/src/styles/global.css` into `web/src/styles/global.css`; install Fraunces / Source Serif 4 / Inter via `@fontsource-variable/*`. |
| 10 | **Layout shell** | `Masthead`, `BroadsheetNav`, `Footer`. Masthead with "Atlas" in `--accent-surname` amber + italic tagline between double-rules. |
| 11 | **Home page** | Two-column broadsheet — `SearchBox` + drop-cap lede on the left; "Browse by metro" + "Recently added" on the right (TanStack Query against #6). |
| 12 | **Results page** | `/r/:postalCode` route; `Dateline` header treatment; `EntryList` rendering the "Local" / "Regional" classified-section layout against `/api/v1/lookup`. |
| 13 | **Submit form** | `/submit` with `react-hook-form`, broadsheet-style fieldsets, optional Turnstile, POSTs to `/api/v1/submissions`. |
| 14 | **Browse + metro pages** | `/browse` list of metros; `/m/:metroSlug` reusing the results layout. |
| 15 | **About + 404** | Single-column `.page` treatment; mission/methodology/criteria copy. |
| 16 | **Admin queue page** | `/admin/queue` — bearer token in `localStorage` for v1, approve/reject controls. Utilitarian, not for public eyes. |
| 17 | **Web CI** | ESLint, Vitest + RTL tests (API client + form validation), `npm run build`. The `web` job in CI auto-lights-up once `web/package.json` exists. |
| 18 | **Web recipes in justfile** | `web-dev`, `web-build`, `web-test`, `web-lint`. |

## Deployment & ops

| # | Slice | What lands |
|---|-------|------------|
| 19 | **Dockerfile + fly.toml** | Multi-stage Go build (binary embeds migrations); `fly launch`; `release_command = "urbanist-atlas-server migrate up"`. |
| 20 | **Fly Managed Postgres** | Provision MPG, attach to the app, set `URBANIST_DB_URL` + `URBANIST_ADMIN_TOKEN` in Fly secrets. |
| 21 | **Cloudflare Pages** | Connect Pages to `web/`; production domain `urbanistatlas.com` + DNS; `api.urbanistatlas.com` → Fly. |
| 22 | **Production CORS** | Add `https://urbanistatlas.com` and `*.pages.dev` to `URBANIST_CORS_ORIGINS` in Fly secrets. |
| 23 | **End-to-end smoke** | Hit prod `/healthz` and `/api/v1/lookup`; submit a real org via `/submit`; approve via `/admin/queue`; confirm it appears in subsequent lookups. |

## Deferred (v1.1+)

Not blocking launch:

- Org detail pages (`/orgs/{slug}`)
- Email/Slack notifications on new submissions
- Multi-moderator auth (replaces the v1 shared bearer token)
- Map view
- Org self-service editing
- Housing / YIMBY scope expansion (deliberately deferred per scope)
- i18n beyond US/CA English
