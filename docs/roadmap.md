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
  layout against `/api/v1/lookup`.
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
- **Browse + recent endpoints (slice #6):** `GET /api/v1/metros`,
  `GET /api/v1/metros/{slug}`, `GET /api/v1/recent` — the backend
  half of the homepage Browse + Recently-added panels. The metro
  set is named by a single `atlas.IsMetroKind` predicate (us:metro,
  ca:cma, ca:regional-district, pt:area-metropolitana) so adding a
  country's metro-equivalent is a one-line append. SQL walks the
  region DAG downward via a recursive CTE — an org tagged only to
  Brooklyn counts toward NYC metro — and `/recent` excludes orgs
  whose only region attachments are `scope_tier='national'` (the
  slice-#4.6 filter, so MUBi-nacional stays out of the homepage
  strip). Handler-test coverage (httptest + MemStore) +
  testcontainers-backed integration tests against the production
  seed. See `docs/superpowers/specs/2026-05-18-browse-endpoints-design.md`.
- **Browse + metro pages (slice #14):** `/browse` lists every metro
  with org counts, ordered by org count desc + name asc tiebreak;
  `/m/:metroSlug` reuses the classified-section layout from
  `/r/:postalCode`. The homepage right-column asides ("Browse by
  metro" / "Recently added") now render real data via `useQuery`
  against `/api/v1/metros` and `/api/v1/recent` — no more "Coming
  soon" placeholders. See
  `docs/superpowers/specs/2026-05-18-browse-pages-design.md`.
- **About + 404 page (slice #15):** `/about` uses a single-column
  `.page` treatment with mission / methodology / criteria /
  acknowledgments copy linking to *Urbanist Lexicon* at
  `mjrossi.com/blog`. Newspaper-style 404 ("Page not in this
  edition.") wired via `errorElement` on the root route. See
  `docs/superpowers/specs/2026-05-18-about-404-design.md`.
- **ODbL attribution in responses (slice #24):** every
  `/api/v1/**` success response now carries `X-Data-License:
  ODbL-1.0` and `X-Data-Attribution: https://urbanistatlas.com`;
  collection responses (`/metros`, `/recent`) wrap their payload in
  a `{ meta, data }` envelope where `meta` adds `license`,
  `attribution_url`, and `generated_at` (RFC3339 UTC, second
  precision). The `respondCollection[T]` helper in
  `api/internal/httpapi/odbl.go` is the in-tree wrapping point; an
  `/api/v1`-scoped middleware sets the headers (with `/healthz`
  intentionally exempt). Frontend helpers unwrap `data`
  transparently. See
  `docs/superpowers/specs/2026-05-18-odbl-response-shape-design.md`.
- **Postal-code coverage at scale (slice #7.5, sub-slices #7.5.1–4):**
  Replaces the 47-row fixture with 33,774 US ZCTAs + 1,643 CA FSAs
  via the smallest-anchor model. New `internal/etl/{us,ca}` packages
  parse Census CBSA + ZCTA crosswalks (US) and StatsCan FSA + CMA
  boundary DBFs (CA), emitting deterministic `regions_us_msas.toml`
  (393 MSAs), `regions_ca_cmas.toml` (41 CMAs), and per-country
  `postal_codes_*.csv`. Region taxonomy gains state/province tier
  (52 US + 13 CA), multistate tier (3 US), and CMA/MSA tier (393 + 41).
  NYC `nyc` flips to a regional intermediate region above 5 borough
  leaves; the state edge moves from `nyc → ny` to the borough rows
  per region-graph rule §1. `loadpostal` switched to batched
  `unnest` upserts via raw `pgx.Exec` so 33k US rows load in ~3s
  instead of multi-minute per-row round-trips. `internal/loadregions`
  gains cross-file parent resolution so multi-tier TOML splits
  resolve. Design spec: `docs/superpowers/specs/2026-05-19-postal-coverage-design.md`.
- **Org-seed growth (slice #7.6):** Expanded `api/seed/orgs.toml` from
  23 curated entries (19 US/CA + 4 PT) to 111 via two independent
  coverage gates — a universal state/province floor (every US state +
  every CA province has ≥1 org or a documented `# gap`) and a top-30
  metro gate (≥1 metro-anchored org per metro in the 25 US + 5 CA
  canvas). Closing tally: 88 net-new orgs, 13 documented gaps (9 US:
  WV, AR, OK, KS, ND, SD, NV, WY, PR; 4 CA: PE, SK, NB, plus YT/NT/NU
  consolidated), and 1 multi-anchored org (The Street Trust). Design
  spec: `docs/superpowers/specs/2026-05-20-org-seed-growth-design.md`.
- **Top-20 metro depth pass (slice #7.7):** Raised the metro gate to
  ≥3 orgs per top-20 metro (top-21–30 stays at ≥1). Boston gets the
  showcase treatment at 5 metro orgs (TransitMatters, LivableStreets,
  Boston Cyclists Union, A Better City, MBTA Advisory Board) plus
  WalkMassachusetts at the state floor. LA, Chicago, Dallas, Houston,
  Philadelphia, Atlanta, SF Bay, Seattle, Minneapolis, Phoenix,
  Detroit, and St. Louis lift to ≥3 metro orgs each. Four top-20
  metros end the pass with documented third-org gaps (Miami at 2,
  Inland Empire at 2, Tampa at 1, Denver at 2) rather than padding
  with dormant or out-of-scope candidates. Final tally: 23 net-new
  orgs (orgs.toml grows from 111 → 134). Design spec gate language
  updated in the same spec.
- **X-Atlas-Client shared-secret gate (slice #23):**
  `api/internal/httpapi/clientsecret.go` middleware checks
  `X-Atlas-Client` against `URBANIST_CLIENT_SECRET` via
  `subtle.ConstantTimeCompare`; mismatch → 401 with the new
  `unauthorized` RFC 9457 problem type. `/healthz` and
  `/api/v1/openapi.yaml` are bypass-listed so liveness probes and
  contract discovery work without the secret; an empty secret turns
  the middleware into a no-op (preserves local-dev ergonomics).
  Frontend (`web/src/lib/api.ts`) injects the header from
  `VITE_API_CLIENT_SECRET` on every `apiFetch`.
- **Hosting cost spike (slice #19.5):** survey of May-2026 pricing
  across Fly Machines, Hetzner, Render, Cloudflare Workers
  Containers, and Heroku pivoted Phase 1 off Fly Managed Postgres
  (~$38/mo) to **Heroku Basic dyno + Heroku Postgres Essential-0
  (us, Virginia) at $12/mo total**. Decision recorded in
  `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` and
  expanded in `docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`.
- **Heroku deploy deliverables (slice #20) — code/config/docs only;
  runbook not yet executed:** `Procfile` at repo root declares the
  `release` (migrations) + `web` (serve) processes;
  `URBANIST_DB_URL` → `DATABASE_URL` rename across every
  Postgres-touching urfave/cli flag (`serve`, `migrate`, `seed`,
  `loadregions`, `loadpostal`, `loaddata`); new `loaddata`
  subcommand (`api/cmd/server/loaddata.go` +
  `api/internal/loaddata/loaddata.go`) chains regions → postal
  codes → orgs across every bundled country in dependency order,
  with testcontainers-backed integration coverage; `[group('heroku')]`
  recipes in the justfile (`heroku-deploy`, `heroku-logs`,
  `heroku-config`, `heroku-ssh`, `heroku-loaddata`, `db-backup`);
  end-to-end Heroku runbook at `docs/deploy.md`. The slice-#19
  `Dockerfile` + `fly.toml` deleted by the pivot. **Not yet
  executed against live Heroku** — no Heroku app exists, no
  Postgres add-on is provisioned, no deploy has been pushed.
- **Cloudflare Pages deliverables (slice #21) — code/docs only;
  runbook not yet executed:** `web/public/_redirects` ships the
  `/* /index.html 200` Pages SPA fallback so direct navigation to
  `/about`, `/browse`, `/m/:slug`, `/r/:postalCode` works in
  production; `docs/deploy.md` § Cloudflare Pages + DNS documents
  Pages project setup, DNS records, custom-domain attachment, and
  smoke tests for both halves. **Not yet executed** — no Pages
  project exists and no DNS records have been created.
- `justfile` recipes: `api-*` (build / vet / test / sqlc-gen /
  oapi-gen / test-integration / gen-check), `migrate-*`, `pg-*`,
  `healthz`, `lookup`, `seed`, `loadregions`, `loadpostal`,
  `loaddata`, `ci`.

## Deferred from this milestone

A few numbered slices were deferred during the v1.0 build:

- **#4.7 Second EU country validation (Spain)** — deferred
  2026-05-18. After Portugal (#4.6) the data model was deemed
  sufficient for v1.0; Spain becomes a v1.1+ candidate.
- **#5 Submissions + admin queue** and **#13 Submit form** —
  deferred to Phase 2 alongside slice #26 (API keys + email-verified
  registration). The reasoning: a public submission flow needs both
  an operational triage workflow and an account model; the natural
  home is the same account / email-verification machinery Phase 2
  already requires for API keys. Building accounts now just for
  submissions would mean building them twice. Slice #16 (Admin
  queue page) and the form-validation half of slice #17 ride this
  deferral.

The rows remain in the tables below for traceability.

## Backend (Go)

| # | Slice | What lands |
|---|-------|------------|
| 4.7 | **Second EU country validation (Spain)** | Repeat the validation exercise for Spain. Adds `regions_es.toml`, `postal_codes_es.csv`, ~5 ES orgs. Specifically validates: autonomous communities (Catalonia, Basque Country with their own transit authorities), the comarca layer in some communities, and Ceuta/Melilla as the analogue of Açores/Madeira. Should be mostly mechanical given #4.6's conventions and loader changes. |
| 5 | **Submissions + admin queue** | `POST /api/v1/submissions` (rate-limited, optional honeypot/Turnstile); `GET /admin/submissions`, `POST /admin/submissions/{id}/approve\|reject` (bearer-token auth via `URBANIST_ADMIN_TOKEN`); the approval transaction promotes a submission row into an `organizations` row. Region attachment uses the same `region_slugs` machinery as `orgs.toml`, so submitted orgs can target any region kind in any supported country. |
| 7 | **Handler tests** | `httptest`-based integration tests for `/lookup`, `/submissions`, the admin endpoints. Lookup coverage should include: the national-tier filter (orgs attached to `scope_tier='national'` regions must NOT appear in default results) and the unknown-country fall-through (`country=ZZ` with an unknown postal code returns 404, not 400) — both shipped in slice #4.6 with light coverage in `pipeline_test.go`. |
| 7.5 | **Full-country postal data ingest** *(broken into sub-slices below)* | The smallest-anchor design has every US ZIP and CA FSA resolve to the smallest curated region (city leaf → NYC borough → MSA → state/province) — schema unchanged, no app-level fallback logic. Sub-slices below. Design spec: [`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./superpowers/specs/2026-05-19-postal-coverage-design.md). |
| 7.5.1 | **Foundation: ETL scaffolding + states/provinces** | Design spec; `etl download`/`etl regenerate` subcommand stubs on the `urbanist` binary; `api/internal/etl/` package skeleton; hand-defined `regions_us_states.toml` (52: 50 states + DC + PR) and `regions_ca_provinces.toml` (13: 10 provinces + 3 territories), with existing state/province entries moved out of `regions_us.toml` and `regions_ca.toml` for cleaner separation. No data-scale change. |
| 7.5.2 | **NYC borough split** *(shipped)* | Migration `0004_split_nyc.sql` flips `nyc.scope_tier=regional`, drops the `nyc → ny` edge (boroughs carry the state edge per region-graph rule §1), keeps `nyc → nyc-metro`. Borough leaves keep their parents `[nyc, ny]`. Editorial decision: citywide NYC orgs (TransAlt, Riders Alliance, StreetsPAC) stay on `nyc` and bucket as Regional for borough lookups. Place-label heuristic in `pkg/atlas/lookup.go` updated to prefer `IsMetroKind` for the broad slot so labels like "Brooklyn, New York City — New York Metro" survive the regional-tier `nyc`. |
| 7.5.3 | **US MSAs + ~34k ZCTA postal codes** *(shipped)* | New `internal/etl/us` package parses Census CBSA delineation (xlsx → CSV via `etl/scripts/xlsx_to_csv.py`) + ZCTA-to-place + ZCTA-to-county. Generates `regions_us_msas.toml` (393 entries) using `regions_us_msa_overrides.toml` for the 7 known metros (nyc-metro, chicago-metro, sf-bay-area, greater-boston, greater-miami, seattle-metro, greater-la). Generates `postal_codes_us.csv` (~33.7k rows) via smallest-anchor crosswalk. `loadpostal` switched to batched `unnest` upserts via raw `pgx.Exec` to avoid 33k per-row round-trips on Heroku. New `regions_us_multistate.toml` carved out of `regions_us.toml` to break the circular load order between MSAs and curated leaves. Integration tests passing in ~36s. |
| 7.5.4 | **CA CMAs + 1,643 FSA postal codes** *(shipped)* | New `internal/etl/ca` package parses the StatsCan FSA + CMA boundary file DBF tables (extracted from the boundary zips inside the ETL; shapefile geometry ignored). Generates `regions_ca_cmas.toml` (41 CMAs filtered to type='B', with overrides for toronto-cma/montreal-cma/metro-vancouver/ottawa-gatineau-cma) and `postal_codes_ca.csv` (1,643 rows). FSA→CMA mapping uses a coarse FSA-prefix table (M, L1/3/4/5/6 → Toronto; H → Montréal; V5-7 → Vancouver; K1-2 + J8-9 → Ottawa-Gatineau; T2-3 → Calgary; T5-6 → Edmonton; L8-9 → Hamilton) in lieu of the restricted-licence PCCF. Minimal stdlib-only DBF reader; Latin-1 → UTF-8 decoding for accented CMA names. Anchor distribution: 10 city-leaf, 522 CMA, 1111 province. Closes #7.5. |
| 7.5.5 | **Non-ZCTA ZIP fallback** | Census ZCTA excludes P.O. Box-only ZIPs, single-building ZIPs, and APO/FPO ZIPs — so `/lookup?postal_code=20811` (Bethesda P.O. Box) returns `postal-code-not-found` today. Add HUD's quarterly USPS ZIP Code Crosswalk as a second US ETL source; emit fallback rows only for ZIPs absent from ZCTA. Preference order: HUD-supplied CBSA → curated county leaf if one exists → state floor. For 20811: HUD maps to Montgomery County MD inside CBSA 47900 → anchors at `washington-dc-metro`. Pin HUD vintage in `etl/SOURCES.md`; integration-test regression on 20811. CA likely needs no equivalent — FSA-prefix → province fallback in #7.5.4 already covers P.O. Box FSAs. Update [`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./superpowers/specs/2026-05-19-postal-coverage-design.md) §Out-of-coverage UX to describe the two-source pipeline. |
| 7.6 | **Seed data growth** | Expand `orgs.toml` from the curated 23 (19 US/CA + 4 PT) to the planned **~100–120** across the supported countries via two independent coverage gates: a **universal state/province floor** (every US state + every CA province has ≥1 org or a documented `# gap`) plus a **top-30 metro gate** (25 US CBSAs + 5 CA CMAs each get ≥1 org). Editorial work, not engineering. Design spec: [`docs/superpowers/specs/2026-05-20-org-seed-growth-design.md`](./superpowers/specs/2026-05-20-org-seed-growth-design.md). |
| 7.7 | **Top-20 metro depth pass** | Brings Boston to 4–6 orgs (showcase) and lifts each top-20 metro to ≥3 metro-level orgs or a documented third-org gap. Updates the 7.6 design-spec gate language. Editorial work, not engineering. |

## Frontend (React + Vite)

| # | Slice | What lands |
|---|-------|------------|
| 13 | **Submit form** | `/submit` with `react-hook-form`, broadsheet-style fieldsets, optional Turnstile, POSTs to `/api/v1/submissions`. |
| 16 | **Admin queue page** | `/admin/queue` — bearer token in `localStorage` for v1, approve/reject controls. Utilitarian, not for public eyes. |
| 17 | **Web CI tests (partial)** | `lint` / `test` / `build` already run in CI; pending pieces are dedicated `lib/api.ts` tests and the form-validation tests that land with slice #13. |
| 18 | **Web recipes in justfile** | `web-dev`, `web-build`, `web-test`, `web-lint`. |
| 18.5 | **Dev-loop env wiring** | Default `VITE_API_BASE` and `VITE_API_CLIENT_SECRET` for the local dev loop so `just web-dev` talks to `just api-run` out of the box. Preferred home is `mise.development.toml` (mirroring how `DATABASE_URL` is set for the server); alternative is a committed `web/.env.development`. Small ergonomic follow-up to slice #18 — `web-build`/`web-test` are affected too. |

## Gatekeeping, licensing & ops

The API ships in two phases (per
[launch strategy](../CLAUDE.md#hosting)): **Phase 1** is locked-down
dogfooding — only the project's own frontend can call it — and **Phase 2**
opens the API to the public behind a free-key + rate-limit model. ODbL
attribution travels in every response in both phases.

Status legend below: ✅ = deliverables landed in the repo; ▶ = runbook
execution against live infra; ⏳ = not yet started.

| # | Slice | What lands | Status |
|---|-------|------------|--------|
| 19 | **Dockerfile + fly.toml** *(retired by #19.5)* | Multi-stage Go build (binary embeds migrations); `fly launch`; `release_command = "urbanist-atlas-server migrate up"`. Shipped, then deleted by the slice #19.5 Heroku pivot — Heroku uses the `heroku/go` buildpack + `Procfile` instead. | ✅ (then retired) |
| **19.5** | **Hosting cost spike** | Verify Fly Managed Postgres pricing (~$38/mo Basic before storage), survey alternatives, pick a Phase 1 DB host that lands ≤ $5/mo all-in. Deliverable: `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md`, with the decision recorded in its **Decision** section (Heroku Basic + Postgres Essential-0, $12/mo total). | ✅ |
| 20 | **Heroku deploy + Postgres Essential-0** | **Deliverables (shipped):** `Procfile`, `URBANIST_DB_URL` → `DATABASE_URL` env rename across every cli flag, `loaddata` subcommand, `heroku-*` justfile recipes, `docs/deploy.md` end-to-end runbook. **Runbook execution (pending):** create the Heroku app (region `us`, Common Runtime, `heroku/go` buildpack), provision Heroku Postgres Essential-0 add-on (auto-sets `DATABASE_URL`), set `URBANIST_ADMIN_TOKEN` + `URBANIST_CLIENT_SECRET` + non-secret config via `heroku config:set`, push to deploy (release-phase Procfile entry runs migrations), seed via `heroku run urbanist-atlas-server loaddata`. | ✅ deliverables / ▶ execute runbook |
| 21 | **Cloudflare Pages + DNS** | **Deliverables (shipped):** `web/public/_redirects` SPA fallback; `docs/deploy.md` § Cloudflare Pages + DNS documenting Pages project setup, DNS records, custom-domain attachment, smoke tests. **Runbook execution (pending):** create the Pages project (build cmd `cd web && npm ci && npm run build`, output `web/dist`, env vars `VITE_API_BASE` / `VITE_API_CLIENT_SECRET` / `NODE_VERSION=22` on Production + Preview), wire Cloudflare DNS (`qa` CNAME proxy ON, `qa-api` CNAME proxy OFF), attach `qa.urbanistatlas.com` as Pages custom domain, `heroku domains:add qa-api.urbanistatlas.com` + `heroku domains:wait` for ACM. | ✅ deliverables / ▶ execute runbook |
| 22 | **Production CORS (Phase 1 lockdown)** | Tighten `URBANIST_CORS_ORIGINS` from local-dev defaults to **only** `https://qa.urbanistatlas.com` + `*.pages.dev` once Phase 1 dogfooding is live (and later, on prod cutover, swap in `https://urbanistatlas.com`). No wildcard. Set via `heroku config:set` during slice #20 execution; this slice is the audit that confirms it. | ⏳ |
| 23 | **Shared-secret gate (Phase 1)** | Middleware checking `X-Atlas-Client` against `URBANIST_CLIENT_SECRET`; mismatch → 401 RFC 9457 `unauthorized`. Frontend bundles the secret via `VITE_API_CLIENT_SECRET`. Bypass list: `/healthz`, `/api/v1/openapi.yaml`. | ✅ |
| 25 | **End-to-end smoke (Phase 1)** | Hit dogfood `/healthz` + `/api/v1/lookup` (with shared secret). Confirm attribution headers + meta envelope are present. Confirm anonymous (no-secret) calls are rejected. Submissions + admin smoke deferred to Phase 2 with their slices. | ⏳ |
| 26 | **API key model — schema & issuance (Phase 2)** | `api_keys` table (id, hashed key, owner_email, tier, created_at, revoked_at); admin endpoints to issue + revoke; a tiny `/keys/register` flow for self-serve free keys (email-verified). Migrations + sqlc + httpapi handlers. | ⏳ |
| 27 | **Tiered rate limiting (Phase 2)** | Token-bucket middleware keyed by API key (or IP for anonymous traffic). Tight anonymous budget; generous keyed budget; explicit `429 Too Many Requests` problem doc with `Retry-After`. | ⏳ |
| 28 | **Phase 2 cutover** | Add `urbanistatlas.com` as a second Pages custom domain + `api.urbanistatlas.com` to Heroku; loosen CORS to include the prod origin; remove the shared-secret middleware; document the keyed-auth requirement in the public docs + landing-page section. Telemetry dashboard for key-tier traffic patterns. | ⏳ |

## Deferred (v1.1+)

Not blocking launch:

- Org detail pages (`/orgs/{slug}`)
- Email/Slack notifications on new submissions
- Multi-moderator auth (replaces the v1 shared bearer token)
- Map view
- Org self-service editing
- Housing / YIMBY scope expansion (deliberately deferred per scope)
- i18n beyond US/CA English
