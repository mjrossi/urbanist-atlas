# QA-first deploy chunk (slices #19 + #20 + #21 + #23)

**Status:** Active — implementation of the Phase 1 launch chunk that
takes Urbanist Atlas from "works on localhost" to "live at QA URLs."
**Supersedes:** none.
**Related:**
- [`docs/roadmap.md`](../../roadmap.md) (slice rows #19, #20, #21, #23)
- [`CLAUDE.md`](../../../CLAUDE.md) §Hosting + §Launch strategy
- [`api/openapi.yaml`](../../../api/openapi.yaml) (gains an
  `unauthorized` problem type in slice #23)
- [`api/internal/httpapi/cors.go`](../../../api/internal/httpapi/cors.go)
  (allowlist semantics that #19 leans on)
- [`web/src/lib/api.ts`](../../../web/src/lib/api.ts) (gains
  `X-Atlas-Client` header injection in slice #23)

## Why this exists

All Phase 1a feature work is shipped: every v1 endpoint, every SPA
page, ODbL attribution in every response, the region-DAG schema with
PT validation. The codebase has no half-built features and no
flagged tech debt. What's missing is operational — there is no
container, no Fly app, no Pages project, no DNS, and no
`X-Atlas-Client` shared-secret gate to keep the dogfood window
locked down per CLAUDE.md's "Phase 1: locked-down dogfooding"
framing.

This chunk lands those four pieces in a way that maps cleanly to
the launch strategy: deploy under **QA URLs only** first
(`qa.urbanistatlas.com`, `qa-api.urbanistatlas.com`), validate the
full stack in production conditions, then attach the production
hostnames to the same infrastructure when ready. The QA hostnames
retire at prod-launch; ongoing test environments shift to
ephemeral Pages `*.pages.dev` previews + Fly review apps.

## Strategic goal

Reach Phase 1 launch readiness with one focused chunk, leaving
only the end-to-end smoke runbook (slice #25) and the editorial
seed-data growth (slice #7.6) between this chunk and inviting
the first dogfooders to `qa.urbanistatlas.com`.

The chunk is sequenced so that infrastructure provisioning and
the shared-secret middleware can develop in parallel worktrees,
and so that the QA → prod transition is a configuration change
rather than a re-architecture.

## Design

### Architecture — single env, environment-flavored hostnames

| Component | Resource | Initial hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` | `qa-api.urbanistatlas.com` (Fly cert) |
| DB | Fly Managed Postgres `urbanist-atlas-db` | (private to the Fly app) |
| Web | Cloudflare Pages project `urbanist-atlas`, prod branch `main` | `qa.urbanistatlas.com` (custom domain) |
| Web previews | `<branch>.urbanist-atlas.pages.dev` | Automatic per non-`main` branch |

The Fly app and Pages project are named *without* a `-qa` suffix.
They are the same resources that will host production traffic
later; only the hostnames mapped to them are environment-flavored.
This keeps the QA → prod transition to "attach prod custom
domains, drop QA ones."

`URBANIST_CORS_ORIGINS` in `fly.toml` is set to
`https://qa.urbanistatlas.com,*.pages.dev` for this chunk. Both
forms are already supported by the in-tree CORS handler
(`api/internal/httpapi/cors.go:23-29` for the `*.suffix` matcher).

### URL / DNS plan

| Hostname | Target | Cloudflare proxy | When |
|---|---|---|---|
| `qa.urbanistatlas.com` | Pages project `urbanist-atlas` | **ON** (Pages requires) | This chunk |
| `qa-api.urbanistatlas.com` | Fly app `urbanist-atlas` | **OFF** (direct, Fly TLS) | This chunk |
| `*.urbanist-atlas.pages.dev` | Cloudflare-managed | n/a | Automatic |
| `urbanistatlas.com` | — | — | Prod launch (future) |
| `api.urbanistatlas.com` | — | — | Prod launch (future) |

The API subdomain is **not** Cloudflare-proxied: putting Cloudflare
in front of a JSON API adds a hop without meaningful caching
benefit (responses are personalized per postal code) and Fly
already terminates TLS.

### Slice breakdown

#### Slice #19 — Dockerfile + fly.toml (code only)

Containerize the existing Go binary; declare the Fly deployment
shape. **No** Fly account, DNS, secrets, or services are touched
in this slice — it's just files in the repo.

Deliverables:

- Multi-stage `Dockerfile` (Go build stage + alpine runtime),
  copying `api/seed/` into the runtime image so slice #20 can run
  the one-time data load.
- `.dockerignore` to keep the build context lean.
- `fly.toml` at the repo root declaring app `urbanist-atlas`,
  region `iad`, `release_command = "urbanist-atlas-server migrate up"`,
  `URBANIST_CORS_ORIGINS = "https://qa.urbanistatlas.com,*.pages.dev"`,
  `[http_service]` with auto-stop and a `/healthz` check.
- `[group('fly')]` in the `justfile` with `fly-deploy`, `fly-status`,
  `fly-logs`, `fly-secrets`, `fly-ssh` recipes.
- Cross-reference link from `CLAUDE.md` §Hosting to this spec.

The slice **reuses** the existing `migrate up` subcommand
(`api/cmd/server/migrate.go`) and embedded migration FS
(`api/migrations/embed.go`) — release_command is just the binary
running its own migrate subcommand. No code path is new.

#### Slice #23 — X-Atlas-Client shared-secret middleware (code only)

Adds the Phase 1 lockdown gate. Cheap deterrent against casual
scrapers; not a security boundary against motivated attackers.

Deliverables:

- New middleware `api/internal/httpapi/clientsecret.go` comparing
  the `X-Atlas-Client` request header against
  `URBANIST_CLIENT_SECRET` via `subtle.ConstantTimeCompare`. Empty
  secret → middleware is a no-op (preserves local-dev ergonomics).
- Bypass paths: `/healthz` and `/api/v1/openapi.yaml`. Per the
  roadmap row and CLAUDE.md §Launch strategy, these are exempt so
  liveness probes and contract discovery work without the secret.
- Table-driven tests
  (`api/internal/httpapi/clientsecret_test.go`) covering: missing
  header, wrong secret, correct secret, bypass paths, empty-secret
  no-op.
- Mismatch responds with an `unauthorized` RFC 9457 problem
  (`https://urbanistatlas.com/problems/unauthorized`), 401 status.
  New problem-type entry added to `api/openapi.yaml`;
  `oapi-codegen` regenerated.
- Router wiring after CORS (CORS must be first so preflights still
  receive the right headers), before route handlers
  (`api/internal/httpapi/router.go`).
- `Access-Control-Allow-Headers` extended to include
  `X-Atlas-Client` (`api/internal/httpapi/cors.go:51`).
- `--client-secret` CLI flag in `serve.go` sourced from
  `URBANIST_CLIENT_SECRET`, threaded through `httpapi.Config`.
- Frontend: `web/src/lib/api.ts` injects `X-Atlas-Client` from
  `import.meta.env.VITE_API_CLIENT_SECRET` on every `apiFetch`
  call. Header omitted if env unset (mirrors the backend's empty-
  secret no-op so localhost dev needs no extra env wiring).
- `web/.env.example` documents both `VITE_API_BASE` and
  `VITE_API_CLIENT_SECRET`.

#### Slice #20 — Fly MPG + first deploy (operational + optional code)

After #19 merges. Provisions Managed Postgres, sets secrets,
deploys, seeds the dataset. Mostly `flyctl` CLI invocations; one
small optional code change.

Deliverables:

- New runbook at `docs/deploy.md` documenting the operational
  steps (auth, launch, MPG create + attach, secrets set, deploy,
  seed).
- Three Fly secrets staged:
  - `URBANIST_DB_URL` (required) — manual mapping from MPG's
    `DATABASE_URL` because the binary reads our env name.
  - `URBANIST_ADMIN_TOKEN` (Phase 2 pre-stage; no-op until admin
    endpoints land).
  - `URBANIST_CLIENT_SECRET` (required by slice #23 in prod).
- Optional `urbanist-atlas-server loaddata` subcommand
  (`api/cmd/server/loaddata.go`) wrapping the existing
  loadregions / loadpostal / seed chain in dependency order, so
  every redeploy seed step is a single
  `flyctl ssh console -C "urbanist-atlas-server loaddata"`. The
  existing chain lives in `justfile:139-146`; the Go subcommand
  mirrors it. Recommended in.

#### Slice #21 — Cloudflare Pages + DNS (operational + tiny code)

After #19 merges, in parallel with #20. Connects Pages, attaches
the QA custom domain, wires DNS for both halves, provisions the
Fly TLS cert for `qa-api.urbanistatlas.com`.

Deliverables:

- New `web/public/_redirects` SPA-fallback file: `/* /index.html 200`.
  Without it, direct navigation to `/about`, `/browse`,
  `/m/:slug`, or `/r/:postalCode` hits Pages' default 404 instead
  of react-router's `errorElement`.
- DNS records (Cloudflare):
  - `qa` → CNAME → `<project>.pages.dev` (proxy ON; Pages
    requires it).
  - `qa-api` → CNAME → `urbanist-atlas.fly.dev` (proxy OFF).
- Fly TLS cert: `flyctl certs create qa-api.urbanistatlas.com`
  plus DNS challenge records.
- Pages env vars (set in dashboard, NOT in repo): `VITE_API_BASE
  = https://qa-api.urbanistatlas.com`, `VITE_API_CLIENT_SECRET =
  <same as Fly URBANIST_CLIENT_SECRET>`, `NODE_VERSION = 22`.
- `README.md` and `web/README.md` updated to reflect QA URL is
  live; `docs/deploy.md` extended with the Pages/DNS section.

### Worktree strategy

Slices land as four independent PRs:

1. **Worktree A → slice #19** lands first (small PR, no
   dependencies).
2. **Worktree D → slice #23** can develop in parallel with
   worktree A — touches different files.
3. After #19 is on `main`:
   - **Worktree B → slice #20** (Fly side).
   - **Worktree C → slice #21** (Pages side).
   - B and C are mutually independent; only the shared
     `docs/deploy.md` file is appended in both, and the conflict
     is a trivial trailing-section append.

The `superpowers:using-git-worktrees` skill is invoked per
worktree; the `superpowers:test-driven-development` skill is
invoked specifically for slice #23 (the new middleware is the
only meaningful piece of testable logic in this chunk).

## Non-goals (deliberately out of scope)

- **Slice #22 — production CORS audit.** The CORS env value is
  set in slice #19; dedicated audit/tests/docs come in a later
  chunk.
- **Slice #25 — E2E smoke runbook.** Next chunk; depends on the
  full live stack.
- **Slice #4.7 — Spain validation.** Deferred to v1.1+.
- **Slice #7.5 — Full postal data ETL.** Out-of-band data
  engineering; QA can dogfood on the existing fixture CSVs.
- **Slice #7.6 — Seed-data growth.** Editorial drip work; happens
  between this chunk and inviting dogfooders, separately from
  these PRs.
- **Phase 2 (slices #26–#28)** — API keys, rate limiting, prod
  cutover.

## Future migration: QA → prod

When the maintainer is ready to launch prod, the transition is
configuration-only:

1. Cloudflare Pages: add `urbanistatlas.com` (apex) as a second
   custom domain to the same Pages project; add DNS CNAMEs for
   apex + `www`.
2. Fly: `flyctl certs create api.urbanistatlas.com`; add DNS
   CNAME for `api`.
3. Update Fly secret `URBANIST_CORS_ORIGINS` to include
   `https://urbanistatlas.com`; redeploy.
4. Pages dashboard: point `VITE_API_BASE` at
   `https://api.urbanistatlas.com`; redeploy.
5. After prod verification, remove the QA hostnames (drop Pages
   custom domain, drop Fly cert, drop CORS entry, drop DNS
   records).
6. Going forward, rely on Pages' `*.pages.dev` previews + Fly
   review apps for ephemeral test environments.

No application code change is required for this transition.

## Verification per slice

Verification details live in the implementation plan
(`~/.claude/plans/ok-let-s-pick-this-sorted-treasure.md`) and on
each PR. Headline checks:

- **#19:** `docker build .` succeeds; `docker run --rm urbanist-atlas serve --store=memory`
  responds on `:8080`; `flyctl config validate` passes.
- **#23:** new middleware tests pass; local curl without the
  header → 401 problem+json; with the header → 200; `/healthz`
  and `/api/v1/openapi.yaml` succeed regardless.
- **#20:** `curl https://urbanist-atlas.fly.dev/healthz` → 200;
  `curl -H "X-Atlas-Client: $SECRET" .../api/v1/lookup?...`
  returns a `LookupResult` with ODbL headers.
- **#21:** `https://qa.urbanistatlas.com` loads the home page;
  direct navigation to `/browse` works (proves `_redirects`); a
  known seed ZIP renders local + regional org results; CORS allows
  `*.pages.dev` previews.
