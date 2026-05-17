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
- `mise` tooling — `mise.toml` + dev/ci overlays +
  `mise.local.toml.example`. Pinned: Go, Node, sqlc, goose,
  staticcheck, oapi-codegen.
- CI workflow with auto-detect-and-gate logic; both `api` and `web`
  jobs now active.
- Docker-based dev Postgres on `:55432` via `just pg-up` / `pg-down`
  / `pg-reset` / `pg-shell` / `pg-logs`. Same `postgres:17-alpine`
  image as the integration suite.
- **Wire contract:** `api/openapi.yaml` covers every v1 endpoint and
  schema. Errors use RFC 9457 `application/problem+json` with stable
  `type` URIs under `https://urbanistatlas.com/problems/{slug}` and
  a `request_id` extension. The spec is embedded into the binary and
  served at `GET /api/v1/openapi.yaml` for runtime discovery.
- **Backend foundation** (slices #1 + #2 + OpenAPI codegen):
  - `pkg/atlas` — types, `Store` interface, `MemStore`, `Lookup`,
    `LoadDevFixtures`, tests.
  - Postgres-backed `atlas.Store` (sqlc + pgx) behind
    `serve --store=memory|postgres` (postgres default).
  - `migrate up|down|status` backed by embedded goose;
    `migrations/0001_init.sql` creates all five tables.
  - `oapi-codegen` generates Go types from `openapi.yaml`; the
    `/lookup` handler returns the generated type; the recoverer
    middleware's 500 path emits problem+json too.
  - Testcontainers integration tests under `//go:build integration`.
- **Frontend foundation** (slices #8 + #9 + #10 + OpenAPI codegen):
  - Vite + React + TS SPA, strict tsconfig, flat ESLint config,
    Prettier inline in `package.json`, Vitest + RTL.
  - `openapi-typescript` → `src/lib/api.gen.ts`; `src/lib/api.ts`
    provides `apiFetch<T>`, an `ApiError` carrying the RFC 9457
    body, and a `lookup()` wrapper — all types from `api.gen.ts`.
  - Broadsheet CSS port from mjrossi.com + variable fonts via
    `@fontsource-variable/*`.
  - Layout shell: `Masthead` (amber "Atlas" + italic tagline
    between rules), `BroadsheetNav`, `Footer`. TanStack Query +
    React Router wired.
- **Data pipeline foundation (slices #3 + #4):** `loadpostal` CSV
  ingester + `seed` TOML loader + bundled fixtures. Original v0
  shape (4-tier postal + YAML orgs) was reshaped by slice #4.5;
  the current shape is documented in [`api/seed/README.md`](../api/seed/README.md).
- **Home + Results pages (slices #11 + #12):** Two-column broadsheet
  home with `SearchBox` + drop-cap lede; `/r/:postalCode` results
  with `Dateline` header + `EntryList` Local/Regional classified
  layout against `/api/v1/lookup`. Right-column placeholders for
  "Browse by metro" / "Recently added" until slice #6 lands the
  backing endpoints.
- **Region-graph refactor (slice #4.5):** regions become a multi-parent
  DAG; postal_codes point at the leaf; `scope_tier` is editorial;
  `RegionKind`/`Country` open to free-form strings; loaders move to
  TOML (`regions_<cc>.toml`, `orgs.toml`); SPA renders ancestry
  breadcrumb + "via X" subtitles. See
  [`docs/region-graph.md`](./region-graph.md) for the user-facing
  reference and `docs/superpowers/specs/2026-05-16-region-graph-design.md`
  for the design rationale.
- **Region-graph validation via Portugal (slice #4.6):** added
  `scope_tier='national'` (migration 0003), per-country editorial
  policy (US/CA local-first preserved; PT/UK/NL/MX use national tier
  for genuine country-wide umbrellas), 7-digit PT postal-code support
  in `loadpostal`, removal of the US|CA hardcoded country whitelist,
  and a 22-region / 7-postal-code / 4-org PT validation fixture that
  exercises multi-parent municípios, AML's cross-NUTS-II span,
  autonomous-region parallel hierarchy, and uniões de freguesias.
  See `docs/superpowers/specs/2026-05-17-region-graph-pt-validation-design.md`
  for the validation findings and forward-looking analysis for MX/NL/UK.
- `justfile` recipes: `api-*` (build / vet / test / sqlc-gen /
  oapi-gen / test-integration / gen-check), `migrate-*`, `pg-*`,
  `healthz`, `lookup`, `seed`, `loadregions`, `loadpostal`,
  `loaddata`, `ci`.

## Backend (Go)

| # | Slice | What lands |
|---|-------|------------|
| 4.7 | **Second EU country validation (Spain)** | Repeat the validation exercise for Spain. Adds `regions_es.toml`, `postal_codes_es.csv`, ~5 ES orgs. Specifically validates: autonomous communities (Catalonia, Basque Country with their own transit authorities), the comarca layer in some communities, and Ceuta/Melilla as the analogue of Açores/Madeira. Should be mostly mechanical given #4.6's conventions and loader changes. |
| 5 | **Submissions + admin queue** | `POST /api/v1/submissions` (rate-limited, optional honeypot/Turnstile); `GET /admin/submissions`, `POST /admin/submissions/{id}/approve\|reject` (bearer-token auth via `URBANIST_ADMIN_TOKEN`); the approval transaction promotes a submission row into an `organizations` row. Region attachment uses the same `region_slugs` machinery as `orgs.toml`, so submitted orgs can target any region kind in any supported country. |
| 6 | **Browse / recent endpoints** | `GET /api/v1/metros`, `GET /api/v1/metros/{slug}`, `GET /api/v1/recent` — feeds the homepage strip and `/browse`. "Metro" matches the country-prefixed metro-equivalent kinds (`us:metro`, `ca:cma`, `ca:regional-district`, `pt:area-metropolitana`, future siblings); the handler should derive the set from a stable kind suffix rather than a hardcoded enum. |
| 7 | **Handler tests** | `httptest`-based integration tests for `/lookup`, `/submissions`, the admin endpoints. Lookup coverage should include: the national-tier filter (orgs attached to `scope_tier='national'` regions must NOT appear in default results) and the unknown-country fall-through (`country=ZZ` with an unknown postal code returns 404, not 400) — both shipped in slice #4.6 with light coverage in `pipeline_test.go`. |
| 7.5 | **Full-country postal data ingest** | Replace the bundled fixture CSVs (~30 ZIPs) with real Census ZCTA / StatsCan PCCF reshapes (and OpenPLZ for PT when the directory expands). Out-of-band ETL → 3-column CSVs in the format `loadpostal` already consumes. Documented in [`api/seed/README.md`](../api/seed/README.md). |
| 7.6 | **Seed data growth** | Expand `orgs.toml` from the curated 23 (19 US/CA + 4 PT) to the planned ~30–50 across the supported countries. Editorial work, not engineering. |

## Frontend (React + Vite)

| # | Slice | What lands |
|---|-------|------------|
| 13 | **Submit form** | `/submit` with `react-hook-form`, broadsheet-style fieldsets, optional Turnstile, POSTs to `/api/v1/submissions`. |
| 14 | **Browse + metro pages** | `/browse` list of metros; `/m/:metroSlug` reusing the results layout. |
| 15 | **About + 404** | Single-column `.page` treatment; mission/methodology/criteria copy. |
| 16 | **Admin queue page** | `/admin/queue` — bearer token in `localStorage` for v1, approve/reject controls. Utilitarian, not for public eyes. |
| 17 | **Web CI tests (partial)** | `lint` / `test` / `build` already run in CI; pending pieces are dedicated `lib/api.ts` tests and the form-validation tests that land with slice #13. |
| 18 | **Web recipes in justfile** | `web-dev`, `web-build`, `web-test`, `web-lint`. |

## Gatekeeping, licensing & ops

The API ships in two phases (per
[launch strategy](../CLAUDE.md#hosting)): **Phase 1** is locked-down
dogfooding — only the project's own frontend can call it — and **Phase 2**
opens the API to the public behind a free-key + rate-limit model. ODbL
attribution travels in every response in both phases.

| # | Slice | What lands |
|---|-------|------------|
| 19 | **Dockerfile + fly.toml** | Multi-stage Go build (binary embeds migrations); `fly launch`; `release_command = "urbanist-atlas-server migrate up"`. |
| 20 | **Fly Managed Postgres** | Provision MPG, attach to the app, set `URBANIST_DB_URL` + `URBANIST_ADMIN_TOKEN` in Fly secrets. |
| 21 | **Cloudflare Pages** | Connect Pages to `web/`; production domain `urbanistatlas.com` + DNS; `api.urbanistatlas.com` → Fly. |
| 22 | **Production CORS (Phase 1 lockdown)** | Set `URBANIST_CORS_ORIGINS` to **only** `https://urbanistatlas.com` + `*.pages.dev`. No wildcard. |
| 23 | **Shared-secret gate (Phase 1)** | New middleware checking an `X-Atlas-Client` header against a server-side secret (`URBANIST_CLIENT_SECRET`); reject with RFC 9457 `unauthorized` problem if missing/wrong. Frontend build embeds the secret via `VITE_API_CLIENT_SECRET`. Cheap, defeats casual scrapers/bots. Bypassed for `/healthz` and `/api/v1/openapi.yaml`. |
| 24 | **ODbL attribution in responses** | Every `/api/v1/**` success response carries `X-Data-License: ODbL-1.0` + `X-Data-Attribution: https://urbanistatlas.com` headers, and a `meta` envelope on collection responses with `license`, `attribution_url`, `generated_at`. Document the requirement in the OpenAPI spec. Wraps the existing handlers via a single response middleware. |
| 25 | **End-to-end smoke (Phase 1)** | Hit prod `/healthz` + `/api/v1/lookup` (with shared secret) + `/submit` flow + admin approve. Confirm attribution headers + meta envelope are present. Confirm anonymous (no-secret) calls are rejected. |
| 26 | **API key model — schema & issuance (Phase 2)** | `api_keys` table (id, hashed key, owner_email, tier, created_at, revoked_at); admin endpoints to issue + revoke; a tiny `/keys/register` flow for self-serve free keys (email-verified). Migrations + sqlc + httpapi handlers. |
| 27 | **Tiered rate limiting (Phase 2)** | Token-bucket middleware keyed by API key (or IP for anonymous traffic). Tight anonymous budget; generous keyed budget; explicit `429 Too Many Requests` problem doc with `Retry-After`. |
| 28 | **Phase 2 cutover** | Loosen CORS to allow any origin; remove the shared-secret middleware; document the keyed-auth requirement in the public docs + landing-page section. Telemetry dashboard for key-tier traffic patterns. |

## Deferred (v1.1+)

Not blocking launch:

- Org detail pages (`/orgs/{slug}`)
- Email/Slack notifications on new submissions
- Multi-moderator auth (replaces the v1 shared bearer token)
- Map view
- Org self-service editing
- Housing / YIMBY scope expansion (deliberately deferred per scope)
- i18n beyond US/CA English
