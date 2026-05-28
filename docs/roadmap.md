# Roadmap

Implementation slices to get Urbanist Atlas to v1. Each row is a
self-contained chunk that could be its own session or PR.

The architectural plan this list is derived from lives at
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (local to the
maintainer); the load-bearing pieces are mirrored into
[`CLAUDE.md`](../CLAUDE.md). This file is the *execution* view —
the plan is the *design* view.

## Status

**Terminology — what "v1.0" means here.** v1.0 is the Phase 1 QA
deliverable: every feature below in this table marked `Done` plus the
gating/security pieces in the *Gatekeeping* section. The apex
domains (`urbanistatlas.com`, `api.urbanistatlas.com`) wait on the
Phase 2 work (slices #26–#28 — API keys, rate limiting, public CORS).
"Production-ready" therefore means the QA stack is on the apex DNS
once Phase 2 lands; the QA gate itself stays on for safety until
then.

**Phase 1 QA dogfooding is LIVE (2026-05-21):**
`qa.urbanistatlas.com` (SPA on Cloudflare Workers + Pages) +
`qa-api.urbanistatlas.com` (API on Fly.io, region `iad`, behind the
`X-Atlas-Client` shared-secret gate). DB is a sibling Fly app
running `postgres:17-alpine` on a 1 GB volume. 130 orgs and 35,417
postal codes seeded. Nightly R2 backups are live (workflow at
`.github/workflows/backup.yml`, bucket `urbanist-atlas-backups`
with a 30-day lifecycle), with the enablement steps documented at
[`docs/runbooks/r2-backups.md`](./runbooks/r2-backups.md).

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
  set was named by a single `atlas.IsMetroKind` predicate (us:metro,
  ca:cma, ca:regional-district, pt:area-metropolitana). SQL walks
  the region DAG downward via a recursive CTE — an org tagged only
  to Brooklyn counts toward NYC metro — and `/recent` excludes orgs
  whose only region attachments are `scope_tier='national'` (the
  slice-#4.6 filter, so MUBi-nacional stays out of the homepage
  strip). Handler-test coverage (httptest + MemStore) +
  testcontainers-backed integration tests against the production
  seed. See `docs/superpowers/specs/2026-05-18-browse-endpoints-design.md`.
  *Superseded by the "Browse goes generic" slice (see below) — the
  endpoint is now `/api/v1/regions`, but the descendant-walk +
  national-filter semantics described here are unchanged.*

- **Browse goes generic — `/api/v1/regions` + slug-based detail
  (2026-05-26):** Browse went `/metros` → `/places` → `/regions` so
  the endpoint name matches the underlying DAG. The list returns
  the editorial default browse set (metros + cities, per
  `atlas.DefaultBrowseKinds`) — no query parameters; the right
  filter axis (taxonomy via `kind`, DAG via `ancestor`, …) stays
  open until a concrete browse UI use case appears. The detail
  endpoint resolves any non-national region — `/regions/cook-county`
  (county), `/regions/chicagoland` (multi-state), `/regions/ny`
  (state), `/regions/brooklyn` (borough) all return the region
  plus its descendant orgs. National-tier slugs still 404. The
  narrower `atlas.MetroKinds` predicate survives in
  `metro_kinds.go` — `/lookup`'s `placeLabel` still uses it so a
  Brooklyn ZIP renders "Brooklyn, NYC — New York Metro" rather
  than picking the city as the broad ancestor. SPA route flipped
  `/p/:placeSlug` → `/region/:regionSlug`; `Place.tsx` →
  `Region.tsx`; the kind-label helper expanded to cover the
  broader kinds the detail endpoint now resolves (State / County /
  Borough / Multi-state region / etc.). The `/lookup` upward-walk
  + scope-tier-partitioning semantics are deliberately unchanged —
  a Naperville ZIP still excludes Chicago-city-only orgs because
  they live in a sibling DAG subtree, not an ancestor.

- **Region detail unifies with Lookup scope (2026-05-26):** The
  remaining UX gap after the Browse nesting slice: clicking a city
  from Browse returned only orgs literally tagged to that city's
  slug, while a ZIP-lookup for the same area returned the city's
  orgs PLUS the metro / state / multi-state ancestors' orgs. Users
  hit a less-useful list on Browse than they did via Lookup. This
  slice extracts a shared `pkg/atlas.BucketOrgsByScope` helper
  used by both endpoints; `/lookup` keeps walking upward only
  (its "I'm at this point" semantic) while `/regions/{slug}` now
  walks both directions, builds an in-scope region set, and
  buckets by attachment `scope_tier`. Result: `/regions/sf`
  returns the same 3 local + 4 regional orgs `/lookup?postal_code=94110`
  does. Wire-level: `RegionDetail` drops `orgs: Org[]`, adds
  `local: LookupOrg[]` + `regional: LookupOrg[]` (each with
  `matched_region_slugs`). New `DescendantRegions` SQL query
  parallel to the existing `AncestorRegions`. SPA: `Region.tsx`
  adopts `EntryList` (same component `Results.tsx` uses), so the
  Browse → Region and Lookup → Region experiences render
  identically below the kicker. The Browse list's `org_count`
  stays at the downward-walk number — preserves the editorial
  differentiation between cities and their parent metros on the
  index.

- **Browse nesting + region ancestry + Results/Region rendering
  consolidation (2026-05-26):** Three UX gaps in one slice. (1) The
  `/regions` list response now carries `browse_parent_slug` per row
  (computed in `ListRegions` via a recursive CTE walking parents
  upward to the nearest browseable-kind ancestor); the SPA's
  `Browse.tsx` uses it to nest cities under their parent metro, so
  "New York City" sits indented beneath "New York Metro" instead of
  rendering as a side-by-side row that reads like a duplicate.
  Same fix for "Chicago" under "Chicago Metro," "Toronto" under
  "Toronto CMA," "Montréal" under "Montréal CMA," and the
  Boston-area cities under "Greater Boston." (2) `RegionDetail`
  gains `ancestry: Region[]` (closest-first walk excluding self +
  national); a new shared `RegionBreadcrumb` component renders it
  as a clickable kicker breadcrumb on the Region page —
  `Atlas / Browse / NYC Tri-State / New York Metro / New York City
  / Kings County / Brooklyn`. The `Results.tsx` (postal-code
  lookup) page adopts the same `RegionBreadcrumb` from
  `resolved_ancestry`, so the path-shape of the page reads
  consistently with the Region detail page. (3) Both pages drop
  their inline org-row + section duplicates and consume the shared
  `Entry` / `EntryList` components (rewritten in this slice to use
  the actual broadsheet CSS — they were orphan dead code before).
  Dead `TagChip` component deleted. The narrower `metroKinds`
  predicate and `/lookup`'s `placeLabel` semantics stay unchanged.

- **Browse + region pages (slice #14):** `/browse` lists every
  default-browse region (metros + cities, post-rename) with org
  counts, ordered by org count desc + name asc tiebreak; the
  detail page (now `/region/:regionSlug`) reuses the classified-
  section layout from `/r/:postalCode`. The homepage right-column
  asides ("Browse the atlas" / "Recently added") render real data
  via `useQuery` against `/api/v1/regions` and `/api/v1/recent` —
  no more "Coming soon" placeholders. See
  `docs/superpowers/specs/2026-05-18-browse-pages-design.md` for
  the original design; the Browse generic surface slice (above)
  documents the later `/metros` → `/places` → `/regions` rename
  and the broadening to surface cities as their own browseable
  entries.
- **About + 404 page (slice #15):** `/about` uses a single-column
  `.page` treatment with mission / methodology / criteria /
  acknowledgments copy linking to *Urbanist Lexicon* at
  `mjrossi.com/blog`. Newspaper-style 404 ("Page not in this
  edition.") wired via `errorElement` on the root route. See
  `docs/superpowers/specs/2026-05-18-about-404-design.md`.
- **ODbL attribution in responses (slice #24):** every
  `/api/v1/**` success response now carries `X-Data-License:
  ODbL-1.0` and `X-Data-Attribution: https://urbanistatlas.com`;
  collection responses (`/regions`, `/recent`) wrap their payload
  in a `{ meta, data }` envelope where `meta` adds `license`,
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
- **Non-ZCTA ZIP backfill (slice #7.5.5) — code shipped, data diff
  deferred to operator:** adds HUD's USPS ZIP-to-County crosswalk as
  a second US ETL source for ZIPs Census ZCTA omits (P.O. Box-only,
  single-building, APO/FPO). New `api/internal/etl/us/hud.go` stdlib
  CSV parser + `CrosswalkHUDBackfill` sibling to the existing 6-tier
  `Crosswalk`. Per ZIP, picks `max(TOT_RATIO)` row (correct for
  P.O. Box-only ZIPs where `RES_RATIO=0`) and walks county FIPS
  through `nyc-borough → countyToLeaf → countyToMSA → stateFIPSToSlug`;
  writer merges + dedups with ZCTA winning any tie. 20811 (Bethesda
  P.O. Box) now resolves to `washington-dc-metro`. HUD source is
  HUDUser-account-gated, so the orchestrator degrades to ZCTA-only
  when the CSV is absent; the operator pins the sha256 in
  `etl/SOURCES.md` + `api/internal/etl/us/us.go` and re-runs
  `etl regenerate --country=US` to materialize the ~5–10k net-new
  rows in `api/seed/postal_codes_us.csv`.
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
- **Org-seed broadening / geographic reach (slice #7.8):** Extended
  the canvas beyond v1 launch floors. New gates: **top-31–50 US
  metro gate** (≥1), **CA CMA #6–10 gate** (≥1), **big-state depth**
  (CA/NY/TX to ≥3, FL/PA/MI to ≥2 where genuinely-distinct
  candidates exist), and a **city-leaf canvas** (Madison, Boise,
  Anchorage, Ann Arbor, Boulder, New Haven, Tucson, Albany NY,
  Spokane, Tallahassee, Charleston SC, Grand Rapids, Fresno,
  Albuquerque, plus Halifax + Mississauga as CA bonuses). City-leaf
  orgs anchor at existing MSA slugs (city dominates MSA) — no
  region-tree changes. Albuquerque is covered by multi-anchoring the
  pre-existing BikeABQ entry at [albuquerque-nm-metro, nm] per the
  Street Trust precedent, rather than adding a new city-leaf row.
  Final tally: +73 net-new orgs (orgs.toml grows from 130 → 203)
  plus 3 top-31–50 metro gaps (Jacksonville, Oklahoma City,
  Birmingham) and 8 city-leaf gaps documented inline.
  New precedents: university-housed advocacy programs and state-
  org sub-committees are not separately admitted (extends the slice
  7.7 chapter/affiliate rule). Design spec:
  [`docs/superpowers/specs/2026-05-22-org-seed-broadening-design.md`](./superpowers/specs/2026-05-22-org-seed-broadening-design.md).
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
- **Phase 1 deploy (Fly + sibling Postgres + Cloudflare Workers +
  Pages):** API on a Fly app in region `iad` via a multi-stage
  Alpine Dockerfile (`release_command = "migrate up"`); database on
  a sibling Fly app running plain `postgres:17-alpine` with a 1 GB
  volume (same image as the testcontainers integration suite, so
  the wire is identical); SPA on Cloudflare Workers + Pages (Static
  Assets), SPA fallback via `wrangler.jsonc`'s
  `assets.not_found_handling = "single-page-application"`; nightly
  `pg_dump | gzip` → Cloudflare R2 via a GitHub Actions cron with a
  30-day bucket lifecycle. `qa.urbanistatlas.com` +
  `qa-api.urbanistatlas.com` live behind the `X-Atlas-Client` gate.
  Design: [`docs/superpowers/specs/2026-05-21-fly-deploy-design.md`](./superpowers/specs/2026-05-21-fly-deploy-design.md);
  runbook: [`docs/deploy.md`](./deploy.md).
- `justfile` recipes: `api-*` (build / vet / test / sqlc-gen /
  oapi-gen / test-integration / gen-check), `migrate-*`, `pg-*`,
  `healthz`, `lookup`, `seed`, `loadregions`, `loadpostal`,
  `loaddata`, `ci`.
- **Hygiene pass between Phase 1 and Phase 2 (slice #25):** evergreen
  architecture docs to dig into when extending the API or adding a
  country — `docs/api-architecture.md` (library-first split, Store
  abstraction, wire-contract codegen, RFC 9457 problem+json, ODbL
  response envelope, Phase 1 gate), `docs/etl-architecture.md`
  (per-country ETL flow, US 2-source merge, source pinning +
  determinism, add-a-country checklist), and
  `docs/testing-strategy.md` (when to write unit vs. handler vs.
  integration vs. frontend tests). API-side dedup of `toOAPIOrg` /
  `toOAPILookupOrg` adapters into `oapi_adapters.go`; consistent
  `fmt.Errorf`-wrapping in `pkg/atlas.Lookup`; new
  `middleware_test.go` covers requestID + recoverer + logging +
  statusRecorder (the four middlewares that had zero direct
  coverage). Seed rename `chicagoland-multistate` →
  `chicagoland` (locals don't say "multi-state"). Web cleanups:
  add the missing `.visually-hidden` CSS rule, drop the unused
  `.entry-list-wrap` wrapper, extract a generic `<AsideCard>`
  component out of `Home.tsx`'s parallel renderer pair, replace
  misleading "Coming soon" status pills with honest per-state
  labels ("Temporarily unavailable" / "Nothing indexed yet"),
  drop the empty `emptyRegionMap` placeholder in `Metro.tsx`, and
  extract `groupCountLabel(n)` into a new `web/src/lib/format.ts`
  shared by Home + Browse. Drop the auto/US/CA country override
  `<select>` from `SearchBox` (the digit/letter heuristic is
  unambiguous for v1) and change the input placeholder away from
  the NYC-centric `11217` example. Copy refinements: link
  *Urbanist Lexicon* in the Footer colophon, add a "Source on
  GitHub →" link in the Footer row, add Inter to the font credit,
  rewrite the About contact line to point at GitHub issues for
  Phase 1, soften the 404 closing line. Add a `/submit` Phase 2
  placeholder route so the nav-linked URL stops falling through to
  the 404 errorElement. Drop PT from the user-facing seed pipeline
  (`loaddata.LoadAll`) and remove the four PT orgs from `orgs.toml`;
  the PT seed files stay under `api/seed/` as a region-graph
  validation fixture and migration
  `0005_drop_pt_user_facing_seed.sql` cleans existing PT rows on
  deploy.
- **Open-source readiness:** the standard community files so the
  repo can flip to public. `CONTRIBUTING.md` (scope guardrails,
  dev-loop quick start, PR conventions, three contribution paths
  with no separate CLA), `SECURITY.md` (routes vulnerability
  reports through GitHub Private Vulnerability Reporting; honest
  one-maintainer SLA), `CODE_OF_CONDUCT.md` (Contributor Covenant
  2.1 with enforcement via the same PVR channel),
  `.github/ISSUE_TEMPLATE/` (bug report + feature request + the
  high-volume org-correction-or-addition template with a
  disclosure field, plus a `config.yml` that disables blank issues
  and surfaces security / conventions / roadmap as contact links),
  and `.github/PULL_REQUEST_TEMPLATE.md` (summary / related /
  test plan / reviewer notes). README updated to retire the stale
  "visit /submit" line in favor of a Contributing section
  pointing at the new files.
- **Lookup-side handler tests (slice #7, lookup half):**
  `httptest`-based handler coverage via the `newTestServer(t)`
  helper in `api/internal/httpapi/lookup_test.go:19-31` (a
  MemStore + `httptest` pattern reused across the package).
  Sibling files: `api/internal/httpapi/regions_test.go` (detail
  handler patterns + non-national slug resolution coverage) and
  `recent_test.go` — including
  `TestListRecent_ExcludesNationalTier` at lines 62-90, which
  pins the slice-#4.6 national-tier filter. Submissions + admin
  half of #7 stays deferred to Phase 2 alongside #5 / #13 / #16.
- **`lib/api.ts` tests (slice #17, lib/api.ts half):**
  `web/src/lib/api.test.ts` (~640 lines) covers `apiFetch`,
  `ApiError`, and every typed wrapper, stubbing the network via
  `vi.stubGlobal('fetch', ...)`. Form-validation tests stay
  deferred and will land with slice #13.
- **Web recipes in `justfile` (slice #18):** the
  `[group('web')]` block in `justfile:248-294` provides
  `web-dev`, `web-deps`, `web-lint`, `web-test`, `web-build`,
  `web-oapi-gen`, `web-gen-check`, and the aggregate
  `web-check`. Follow-up: ship a committed `web/.env.development`
  for zero-step local dev ergonomics; the loop already works
  without it because `web/.env.example` provides the keys,
  `web/src/lib/api.ts` defaults `VITE_API_BASE` to
  `http://localhost:8080`, and an empty `URBANIST_CLIENT_SECRET`
  no-ops the `X-Atlas-Client` middleware on the API side.
- **R2 backups (workflow + runbook + live bucket):**
  `.github/workflows/backup.yml` runs nightly against QA Postgres
  and uploads `pg_dump | gzip` to the `urbanist-atlas-backups` R2
  bucket (30-day lifecycle). `docs/runbooks/r2-backups.md` covers
  enablement (nine numbered steps from Fly token generation through
  verifying the first manual run), the restore path
  (`just db-restore`), rotation + maintenance cadence (6–12 month
  token rotation, quarterly retention audit, half-yearly restore
  drill), and troubleshooting for the common failure modes (SSH
  permission errors, R2 403s on PutObject, endpoint URL typos,
  partial dumps, stragglers past 30 days). `docs/deploy.md` §7
  collapses to a pointer at the runbook so the deep content has
  one home.

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
| 5 | **Submissions + admin queue** *(slice β — shipped)* | `POST /api/v1/submissions` (per-IP rate-limited via `internal/httpapi/ratelimit.go`, 5/hour default); `GET /admin/submissions`, `POST /admin/submissions/{id}/approve\|reject` (bearer auth via `URBANIST_ADMIN_TOKEN`); SQLite store at `/data/atlas.db` on a 1 GiB Fly volume (modernc.org/sqlite + sqlc + goose). Approval queues an async GitHub PR worker (`internal/githubpr/`) appending the new `[[org]]` block to `api/seed/orgs.toml`; the PR is the editorial-review surface and the next deploy ships the merged bundle — no live mutation of the served orgs. Region attachment uses the same `region_slugs` machinery as `orgs.toml`, so submitted orgs can target any region kind in any supported country. Design: [`docs/superpowers/specs/2026-05-27-submissions-sqlite-design.md`](./superpowers/specs/2026-05-27-submissions-sqlite-design.md). |
| 7 | **Handler tests (submissions + admin half)** *(slice β — shipped)* | `httptest` coverage for `/submissions` and the admin endpoints lands with slice β: in-memory SQLite store + chi router + bearer/rate-limit middlewares exercised end-to-end. The PR worker has its own `httptest`-faked GitHub-API integration suite in `internal/githubpr/worker_test.go`. |
| 7.5 | **Full-country postal data ingest** *(broken into sub-slices below)* | The smallest-anchor design has every US ZIP and CA FSA resolve to the smallest curated region (city leaf → NYC borough → MSA → state/province) — schema unchanged, no app-level fallback logic. Sub-slices below. Design spec: [`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./superpowers/specs/2026-05-19-postal-coverage-design.md). |
| 7.5.1 | **Foundation: ETL scaffolding + states/provinces** | Design spec; `etl download`/`etl regenerate` subcommand stubs on the `urbanist` binary; `api/internal/etl/` package skeleton; hand-defined `regions_us_states.toml` (52: 50 states + DC + PR) and `regions_ca_provinces.toml` (13: 10 provinces + 3 territories), with existing state/province entries moved out of `regions_us.toml` and `regions_ca.toml` for cleaner separation. No data-scale change. |
| 7.5.2 | **NYC borough split** *(shipped)* | Migration `0004_split_nyc.sql` flips `nyc.scope_tier=regional`, drops the `nyc → ny` edge (boroughs carry the state edge per region-graph rule §1), keeps `nyc → nyc-metro`. Borough leaves keep their parents `[nyc, ny]`. Editorial decision: citywide NYC orgs (TransAlt, Riders Alliance, StreetsPAC) stay on `nyc` and bucket as Regional for borough lookups. Place-label heuristic in `pkg/atlas/lookup.go` updated to prefer `IsMetroKind` for the broad slot so labels like "Brooklyn, New York City — New York Metro" survive the regional-tier `nyc`. |
| 7.5.3 | **US MSAs + ~34k ZCTA postal codes** *(shipped)* | New `internal/etl/us` package parses Census CBSA delineation (xlsx → CSV via `etl/scripts/xlsx_to_csv.py`) + ZCTA-to-place + ZCTA-to-county. Generates `regions_us_msas.toml` (393 entries) using `regions_us_msa_overrides.toml` for the 7 known metros (nyc-metro, chicago-metro, sf-bay-area, greater-boston, greater-miami, seattle-metro, greater-la). Generates `postal_codes_us.csv` (~33.7k rows) via smallest-anchor crosswalk. `loadpostal` switched to batched `unnest` upserts via raw `pgx.Exec` to avoid 33k per-row round-trips against the production Postgres. New `regions_us_multistate.toml` carved out of `regions_us.toml` to break the circular load order between MSAs and curated leaves. Integration tests passing in ~36s. |
| 7.5.4 | **CA CMAs + 1,643 FSA postal codes** *(shipped)* | New `internal/etl/ca` package parses the StatsCan FSA + CMA boundary file DBF tables (extracted from the boundary zips inside the ETL; shapefile geometry ignored). Generates `regions_ca_cmas.toml` (41 CMAs filtered to type='B', with overrides for toronto-cma/montreal-cma/metro-vancouver/ottawa-gatineau-cma) and `postal_codes_ca.csv` (1,643 rows). FSA→CMA mapping uses a coarse FSA-prefix table (M, L1/3/4/5/6 → Toronto; H → Montréal; V5-7 → Vancouver; K1-2 + J8-9 → Ottawa-Gatineau; T2-3 → Calgary; T5-6 → Edmonton; L8-9 → Hamilton) in lieu of the restricted-licence PCCF. Minimal stdlib-only DBF reader; Latin-1 → UTF-8 decoding for accented CMA names. Anchor distribution: 10 city-leaf, 522 CMA, 1111 province. Closes #7.5. |
| 7.5.5 | **Non-ZCTA ZIP fallback** *(shipped — code; data diff deferred to operator)* | Census ZCTA excludes P.O. Box-only ZIPs, single-building ZIPs, and APO/FPO ZIPs — so `/lookup?postal_code=20811` (Bethesda P.O. Box) returned `postal-code-not-found` pre-#7.5.5. Adds HUD's quarterly USPS ZIP-to-County crosswalk as a second US ETL source via `api/internal/etl/us/hud.go` + `CrosswalkHUDBackfill` (sibling to the existing untouched `Crosswalk`); emits fallback rows only for ZIPs absent from ZCTA, picking max-`TOT_RATIO` row (correct for P.O. Box-only ZIPs where `RES_RATIO=0`) and walking county FIPS through the existing `nyc-borough → county-leaf → msa → state` chain. Writer merges + dedups with ZCTA winning. CA needs no equivalent — FSA-prefix → province fallback in #7.5.4 already covers P.O. Box FSAs. HUD pin in `etl/SOURCES.md` (sha256 TBD by operator on first HUDUser download); integration-test regression on 20811 via synthetic anchor fixture. §Out-of-coverage UX of [`docs/superpowers/specs/2026-05-19-postal-coverage-design.md`](./superpowers/specs/2026-05-19-postal-coverage-design.md) updated. Operator follow-up: run `etl regenerate --country=US` after pinning the HUD sha256 to materialize the ~5–10k net-new rows in `api/seed/postal_codes_us.csv`. |
| 7.6 | **Seed data growth** *(shipped)* | Expanded `orgs.toml` from the curated 23 (19 US/CA + 4 PT) to **111** across the supported countries via two independent coverage gates: a **universal state/province floor** (every US state + every CA province has ≥1 org or a documented `# gap`) plus a **top-30 metro gate** (25 US CBSAs + 5 CA CMAs each get ≥1 org). Closing tally: 88 net-new orgs, 13 documented gaps (9 US: WV, AR, OK, KS, ND, SD, NV, WY, PR; 4 CA: PE, SK, NB, plus YT/NT/NU consolidated), 1 multi-anchored org (The Street Trust). Editorial work, not engineering. Design spec: [`docs/superpowers/specs/2026-05-20-org-seed-growth-design.md`](./superpowers/specs/2026-05-20-org-seed-growth-design.md). |
| 7.7 | **Top-20 metro depth pass** *(shipped)* | Raised the metro gate to ≥3 orgs per top-20 metro (top-21–30 stays at ≥1). Boston gets the showcase treatment at 5 metro orgs plus WalkMassachusetts at the state floor; LA, Chicago, Dallas, Houston, Philadelphia, Atlanta, SF Bay, Seattle, Minneapolis, Phoenix, Detroit, and St. Louis lift to ≥3. Four top-20 metros end the pass with documented third-org gaps (Miami at 2, Inland Empire at 2, Tampa at 1, Denver at 2). Final tally: 23 net-new orgs (orgs.toml grows from 111 → 134). Updates the 7.6 design-spec gate language. Editorial work, not engineering. |

## Frontend (React + Vite)

| # | Slice | What lands |
|---|-------|------------|
| 13 | **Submit form** *(slice β — shipped)* | `/submit` rewired from the Phase-1 GitHub-issue fallback to a live `POST /api/v1/submissions`. Receipt card shows the short submission ID; 429 / 4xx / 5xx surface inline; 5xx keeps the GitHub-issue fallback link so a determined submitter still has a path through. Mobile-formatting pass on the slip (hint widths, textarea padding, full-width submit button, disclosure radios stack vertically). |
| 16 | **Admin triage — CLI subcommand** *(reshaped 2026-05-22 from `/admin/queue` web page; partial — slice β)* | `urbanist-atlas-server submissions retry-pr --id=<uuid>` ships with slice β for the operator who needs to re-run a failed promotion PR without flipping status back. `list` / `approve` / `reject` CLIs are deferred — the admin HTTP endpoints already cover those flows via `curl`, so the CLI subcommand only fills the synchronous "I need to see the PR open or fail right now" gap. A `/admin/queue` web page becomes a v1.1+ candidate if submission volume warrants a second moderator. Tracked alongside the rest of Phase 2 in the launch umbrella issue. |
| 17 | **Web CI tests (form-validation half)** *(shipped — slice β)* | `lint` / `test` / `build` run in CI; Vitest covers the submit-form happy path + 400/429/500 problem-document handling + the GitHub-issue fallback link on 5xx. |

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
| Bring-up | **Phase 1 deploy: Fly + sibling Postgres + Cloudflare Workers + Pages + DNS + CORS + smoke** *(merged + live 2026-05-21)* | **Shipped:** `Dockerfile` + `fly.toml` at the repo root (multi-stage Alpine Go build, `release_command = "migrate up"`); `infra/postgres/{Dockerfile, entrypoint-fly.sh, fly.toml}` for the sibling `urbanist-atlas-db` app — postgres:17-alpine wrapped with a root-stage `chown` so the postgres user can write the PGDATA subdir on Fly's root-owned mount, with `[[restart]] policy = "always"` for resilience; `[group('fly')]` justfile recipes (`fly-deploy`, `fly-deploy-db`, `fly-logs`, `fly-logs-db`, `fly-secrets`, `fly-ssh`, `fly-loaddata`, `db-backup`, `db-restore`); `just smoke` recipe in `[group('smoke')]` (curl checks against `/healthz`, `/api/v1/lookup` with + without `X-Atlas-Client`, ODbL headers, meta envelope, OpenAPI YAML); `wrangler.jsonc` at repo root for Cloudflare Workers + Pages (Static Assets) with SPA fallback via `not_found_handling = "single-page-application"`; `.github/workflows/backup.yml` nightly cron + workflow_dispatch (pg_dump → R2, 30-day retention); `docs/deploy.md` operator runbook; `docs/superpowers/specs/2026-05-21-fly-deploy-design.md` design doc. **Live:** `qa.urbanistatlas.com` (Workers + Pages) + `qa-api.urbanistatlas.com` (Fly app, region `iad`) behind the `X-Atlas-Client` gate; 130 orgs, 35,417 postal codes seeded; release_command, PGDATA, restart, Workers/Pages unified bring-up bugs all caught and fixed with docs/runbook patches. R2 backups live (bucket + workflow + first run verified). | ✅ |
| CORS | **Production CORS (Phase 1 lockdown)** | `URBANIST_CORS_ORIGINS` locked to `https://qa.urbanistatlas.com` + `*.<account>.workers.dev` in `fly.toml`'s `[env]` block; verified by `just smoke`. (On prod cutover, swap in `https://urbanistatlas.com`.) | ✅ |
| Gate | **Shared-secret gate (Phase 1)** | Middleware checking `X-Atlas-Client` against `URBANIST_CLIENT_SECRET`; mismatch → 401 RFC 9457 `unauthorized`. Frontend bundles the secret via `VITE_API_CLIENT_SECRET`. Bypass list: `/healthz`, `/api/v1/openapi.yaml`. | ✅ |
| Smoke | **End-to-end smoke (Phase 1)** | `just smoke` recipe hits the live QA endpoint: `/healthz` (200), `/api/v1/lookup?postal_code=10001&country=US` without secret (401), with secret (200 + `X-Data-License: ODbL-1.0` + `X-Data-Attribution` + `meta` envelope), `/api/v1/openapi.yaml` (200). Submissions + admin smoke deferred to Phase 2 with their slices. | ✅ |
| 26 | **API key model — schema & issuance (Phase 2)** | `api_keys` table (id, hashed key, owner_email, tier, created_at, revoked_at); admin endpoints to issue + revoke; a tiny `/keys/register` flow for self-serve free keys (email-verified). Migrations + sqlc + httpapi handlers. | ⏳ |
| 27 | **Tiered rate limiting (Phase 2)** | Token-bucket middleware keyed by API key (or IP for anonymous traffic). Tight anonymous budget; generous keyed budget; explicit `429 Too Many Requests` problem doc with `Retry-After`. | ⏳ |
| 28 | **Phase 2 cutover** | Add `urbanistatlas.com` as a second Workers + Pages custom domain + `flyctl certs add api.urbanistatlas.com -a urbanist-atlas` + A/AAAA records for `api`; loosen CORS to include the prod origin; remove the shared-secret middleware; document the keyed-auth requirement in the public docs + landing-page section. Telemetry dashboard for key-tier traffic patterns. | ⏳ |

## Deferred (v1.1+)

Not blocking launch:

- Email/Slack notifications on new submissions
- Multi-moderator auth (replaces the v1 shared bearer token)
- Map view
- Org self-service editing
- Housing / YIMBY scope expansion (deliberately deferred per scope)
- i18n beyond US/CA English
- **Shared "preview" Fly app for full-stack PR review** — one
  extra `urbanist-atlas-preview` machine (~$5/mo) that auto-deploys
  the API from any non-main branch on push, paired with Cloudflare
  preview URLs reading a `VITE_API_BASE` env that points at it. The
  Workers ↔ Fly asymmetry today means Cloudflare previews only
  fully work for frontend-only PRs; `just preview` covers full-stack
  review locally in the meantime (see
  [`CONTRIBUTING.md`](../CONTRIBUTING.md#full-stack-pr-review)).
  Promote when full-stack PR volume justifies the extra machine.
- **Browse filter — pick the axis when a use case appears.** The
  list endpoint ships without query parameters today. Two axes are
  in the design space when a concrete UI need arrives:
  - **Taxonomy filter** (`?kind=us:state,us:multi-state`) — easy
    to wire but binds the editorial kind vocabulary to the wire
    contract (rename a kind, break the client).
  - **DAG filter** (`?ancestor=<slug>`) — uses slugs (stable
    identifiers) and matches the mental model "show me everything
    under California." Costs an upward CTE in the SQL.

  When a real Browse UI need shows up (segmented control,
  state-level page, "everything in this region" view, etc.), pick
  the axis that fits the actual use case rather than shipping a
  speculative filter. Until then, the default browse set + the
  detail endpoint's slug-based resolution covers the directory.
  Resolving `national`-tier slugs is a separate deferred decision
  about national-org browse experience.
- **`loaddata --prune` flag** — `seed.LoadFile` is upsert-only:
  removing an `[[org]]` block from `orgs.toml` does NOT delete the
  corresponding row in production (the slug just stops being touched).
  Surfaced 2026-05-23 during the pre-launch URL audit when STAR
  (`sacramento-transit-advocates-and-riders`) had to be dropped after
  its domain got hijacked — required a manual `DELETE FROM
  organizations WHERE slug=...` against the prod DB on top of
  `just fly-loaddata`. The slice: add an opt-in `--prune` flag to
  the `loaddata` subcommand that deletes any org whose slug isn't in
  the loaded file, inside the same transaction; default off so a
  malformed file can't wipe prod data. FK cascades
  (`organization_regions ON DELETE CASCADE`,
  `submissions.promoted_org_id ON DELETE SET NULL`) make the DELETE
  safe. Forward the flag through `just fly-loaddata` and document the
  workflow in `docs/deploy.md`. Small, single-package change
  (`api/internal/loaddata/`).
