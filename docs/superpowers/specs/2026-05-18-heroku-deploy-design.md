# Heroku deploy design — Phase 1 dogfood host

**Status:** **Superseded by [`2026-05-21-fly-redeploy-design.md`](./2026-05-21-fly-redeploy-design.md) (2026-05-21).**
Slice #20's Heroku deliverables shipped on 2026-05-19 (PR #14) but the
runbook was never executed. Slice #20.6 reverses the Heroku decision
back to the spike's Finalist 1 (Fly app + sibling
`postgres:17-alpine`); the Heroku-shaped artifacts (`Procfile`,
`heroku-*` justfile recipes, the original Heroku-targeted
`docs/deploy.md`) are deleted, while the Go-side portability wins
(`DATABASE_URL` rename, `loaddata` subcommand) survive unchanged.
This document is kept for history; do **not** treat it as the current
deploy contract.
**Supersedes:** the Fly Managed Postgres path documented in
[`2026-05-18-qa-deploy-design.md`](./2026-05-18-qa-deploy-design.md)
§Slice #20 (which targets `flyctl mpg`).
**Related:**
- [`2026-05-18-hosting-cost-spike.md`](./2026-05-18-hosting-cost-spike.md)
  — the spike whose Decision section pivots to Heroku and points here.
- [`2026-05-18-qa-deploy-design.md`](./2026-05-18-qa-deploy-design.md)
  — Architecture row "DB" + Slice #20 section get rewritten against
  this design.
- [`../../roadmap.md`](../../roadmap.md) slice #20 — rewritten against
  Heroku.
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §Hosting — rewritten
  against Heroku.
- Open PRs [#11](https://github.com/mjrossi/urbanist-atlas/pull/11)
  (close + reopen on new branch) and
  [#12](https://github.com/mjrossi/urbanist-atlas/pull/12) (small
  rebase to retarget DNS + content).
- Already-merged: PR [#9](https://github.com/mjrossi/urbanist-atlas/pull/9)
  (slice #19, `Dockerfile` + `fly.toml` — both files get deleted by
  this pivot) and PR [#10](https://github.com/mjrossi/urbanist-atlas/pull/10)
  (slice #23, `X-Atlas-Client` middleware — unaffected).

## Why this exists

The spike at
[`2026-05-18-hosting-cost-spike.md`](./2026-05-18-hosting-cost-spike.md)
verified that Fly Managed Postgres at ~$38/mo is indefensible for a
locked-down Phase 1 dogfood. After re-verifying May-2026 pricing
across Fly Machines, Hetzner, Render, Cloudflare Workers Containers,
and Heroku, the maintainer chose **Heroku Basic dyno + Heroku Postgres
Essential-0 (us region, Virginia)** at **$12/mo total**.

The choice trades ~$7/mo over the spike's recommended Fly-sibling
Postgres path for:

- Operator familiarity (the maintainer's prior employer is Heroku).
- Included near-PITR backups (Aurora-backed Essential-0 with
  continuous WAL off-premise + scheduled logical backups) instead of
  a DIY GHA-cron to R2.
- A simpler Phase 2 transition (dashboard upgrade vs build-our-own-HA).

The host is deployed to Heroku's `us` region (Virginia, Common
Runtime) to match the project's US/CA user base. The maintainer
accepts the ~85ms personal latency from PT/ES during Phase 1
dogfooding; multi-region is a Phase 2+ conversation.

The full trade-off matrix is in the spike doc's comparison table; this
design assumes the choice is made and focuses on *how* the pivot lands
in the repo.

## Strategic goal

Get Urbanist Atlas to `qa.urbanistatlas.com` / `qa-api.urbanistatlas.com`
on the chosen host with the smallest viable code change and a runbook
the maintainer can execute from muscle memory. PR #11's loaddata code
and integration test survive untouched; only the deploy surface
(`Dockerfile`, `fly.toml`, `docs/deploy.md`, the `fly-*` justfile
recipes) gets replaced.

## Design

### Architecture — Heroku Basic dyno + Postgres Essential-0

| Component | Resource | Initial hostname |
|---|---|---|
| API | Heroku app `urbanist-atlas`, region `us` (Virginia, Common Runtime), Basic dyno | `qa-api.urbanistatlas.com` (custom domain on Heroku SSL endpoint) |
| DB | Heroku Postgres Essential-0 add-on, attached to the API app | Private to the app via `DATABASE_URL` |
| Web | Cloudflare Pages project `urbanist-atlas`, prod branch `main` | `qa.urbanistatlas.com` |
| Web previews | `<branch>.urbanist-atlas.pages.dev` | Automatic per non-`main` branch |

Single Heroku app, single attached Postgres add-on, single Heroku
region. The app is named *without* a `-qa` suffix — same resource will
host production traffic later; only the hostnames mapped to it are
environment-flavored. QA → prod is a configuration change (add custom
domain `api.urbanistatlas.com`, update CORS env), not a re-architecture.

### Build mechanism — buildpacks, not containers

Heroku supports both classic buildpacks and container deploys. This
design uses the **`heroku/go` buildpack** because:

- The maintainer knows the buildpack flow cold.
- The slice-#19 `Dockerfile` is the only thing it currently does for
  us; under Heroku we don't need a Dockerfile in the slug — the
  buildpack handles `go.mod`, compiles the binary, and assembles the
  slug. The `Dockerfile` and `fly.toml` get deleted.
- Smaller surface: no `heroku.yml`, no registry push step.

The buildpack auto-detects `go.mod` at the repo root. The repo is
already a monorepo with `api/` and `web/` — Heroku's
`heroku/go` buildpack needs the `go.mod` at the build root, so we'll
set `GO_INSTALL_PACKAGE_SPEC=./api/cmd/server` (or its modern
equivalent) and let the buildpack build the binary into the slug at
`bin/urbanist-atlas-server`.

*Verification at implementation time:* Heroku's Go buildpack docs
([heroku/go on devcenter](https://devcenter.heroku.com/articles/go-support))
spell out the current env-var name for monorepo build targets — that
name is the load-bearing detail. Confirm before writing the runbook.

### Connection-string env: consolidate to `DATABASE_URL`

Currently every Postgres-touching subcommand (`serve`, `migrate`,
`seed`, `loadregions`, `loadpostal`) reads its connection string from
`URBANIST_DB_URL`. The `URBANIST_*` prefix is the project's convention
for *app-specific* config (see CLAUDE.md §Tech conventions); the
Postgres connection string isn't app-specific — it's the universal
name every managed-Postgres host (Heroku, Fly MPG, Render, Neon,
Railway) sets automatically.

This design **renames the env variable from `URBANIST_DB_URL` to
`DATABASE_URL`** across the codebase rather than mirroring or falling
back. Rationale:

- Heroku's add-on sets `DATABASE_URL` and **rotates it automatically**
  on credential rotation. Mirroring to `URBANIST_DB_URL` via
  `heroku config:set` would break on every rotation; a fallback adds
  per-command branching for no gain.
- `DATABASE_URL` is the de-facto convention. Any future managed-
  Postgres host Just Works with no config glue.
- CLAUDE.md's convention note gets a one-line addendum: "Postgres
  connection string follows the universal `DATABASE_URL` convention;
  all other config remains `URBANIST_*`-prefixed." Cleaner forever.

**Surface of the change** (in this PR):

- 5 Go files: `api/cmd/server/{migrate,serve,seed,loadregions,loadpostal}.go`
  — flip `cli.EnvVars("URBANIST_DB_URL")` to `cli.EnvVars("DATABASE_URL")`
  and update the matching error messages ("--db-url or DATABASE_URL is
  required").
- `justfile` — any recipe that exports or references `URBANIST_DB_URL`
  flips to `DATABASE_URL`. Inspect the `loaddata`, `loadregions`,
  `loadpostal`, `seed`, `migrate-*`, and `pg-*` recipes during the
  rename.
- `mise.development.toml` — rename the env entry.
- `CLAUDE.md` §Tech conventions — update the env-var list and add the
  one-line addendum above.
- `api/README.md` — update any usage examples that name
  `URBANIST_DB_URL`.
- `docs/deploy.md` — written fresh against `DATABASE_URL`.
- `mise.local.toml.example` (if it names the var) — update.

This is the *only* breaking change for local dev: contributors will
need to rename `URBANIST_DB_URL` to `DATABASE_URL` in any
`mise.local.toml` overrides. The README note covers this; the dev
loop is otherwise unchanged.

### Procfile — the only new infra file

Replaces `fly.toml` end-to-end:

```
release: urbanist-atlas-server migrate up
web: urbanist-atlas-server serve --port=$PORT
```

- **`release` phase**: runs migrations on every deploy in a
  short-lived release dyno against the attached database, exactly
  mirroring `fly.toml`'s `release_command`. Migration failures block
  the deploy — a feature, not a bug.
- **`web` process**: long-running server bound to Heroku's injected
  `$PORT`. The `--port` flag already exists; we just pass `$PORT`
  through.
- **`loaddata`**: stays a one-off, invoked as
  `heroku run urbanist-atlas-server loaddata`. Not a Procfile entry.

### Config — non-secret env

Set once via `heroku config:set`:

| Key | Value | Notes |
|---|---|---|
| `URBANIST_LOG_FORMAT` | `json` | Production logging |
| `URBANIST_STORE` | `postgres` | The only viable prod setting |
| `URBANIST_CORS_ORIGINS` | `https://qa.urbanistatlas.com,*.pages.dev` | Same allowlist as the Fly path; CORS handler at `api/internal/httpapi/cors.go` already supports the `*.suffix` form |
| `URBANIST_SEED_DIR` | `./seed` | Slug-relative; buildpack output places the binary at the slug root, so `./seed` is `api/seed/` after the buildpack copies it (see § "Seed files in the slug") |

`URBANIST_PORT` is **not** set on Heroku — the dyno's `$PORT` is
injected per request and the `serve --port=$PORT` Procfile entry wins.

### Secrets

Same shape as the Fly path — two app-level secrets:

```sh
heroku config:set \
    URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" \
    URBANIST_CLIENT_SECRET="$(openssl rand -hex 32)" \
    -a urbanist-atlas
```

- `URBANIST_ADMIN_TOKEN` — Phase 2 pre-stage; no-op until admin
  endpoints land.
- `URBANIST_CLIENT_SECRET` — Phase 1 lockdown secret; the same value
  must be set as `VITE_API_CLIENT_SECRET` in the Cloudflare Pages
  dashboard. Rotation procedure is documented in
  `docs/deploy.md` § Secrets rotation (rewritten for Heroku).

`DATABASE_URL` is set automatically by the add-on; no manual handling
required.

### Seed files in the slug

The `api/seed/` directory ships in the slug because the `heroku/go`
buildpack copies the whole repo into the build context. `URBANIST_SEED_DIR`
points the binary at the right path. If the buildpack's relative
working directory turns out to differ from what we expect, the
fallback is the existing `--seed-dir` flag — but the env var is
preferred because it survives the Procfile.

*Verification at implementation time:* run `heroku run ls` after
first deploy to confirm the working directory layout and the path to
`api/seed/`. Adjust `URBANIST_SEED_DIR` accordingly.

### Backups — Heroku-managed

Heroku Postgres Essential-0 includes:

- **Scheduled and on-demand logical backups** via `heroku pg:backups`
  (retention is Heroku-managed; confirm current policy in the
  dashboard at provisioning time).
- **Continuous protection / off-premise WAL** on the Aurora-backed
  Essential plans — effectively PITR for any single-point
  catastrophic failure.

Operationally:

```sh
heroku pg:backups:schedule DATABASE_URL --at "02:00 America/New_York" -a urbanist-atlas
heroku pg:backups -a urbanist-atlas              # list
heroku pg:backups:capture -a urbanist-atlas      # ad-hoc snapshot
heroku pg:backups:download -a urbanist-atlas     # pull most recent dump
```

A `just db-backup` recipe wraps `heroku pg:backups:capture` for the
maintainer's one-key ad-hoc snapshot habit. **No GHA cron, no R2
bucket** — the spike doc's DIY backup plan retires.

### TLS, custom hostname

`qa-api.urbanistatlas.com` attaches to the Heroku app via the
Heroku SSL endpoint (automatic ACM cert on paid dynos — Basic
qualifies). The Cloudflare DNS record for `qa-api` is a CNAME to the
Heroku-managed DNS target (proxy **OFF** — direct, Heroku-issued TLS).
Concretely the wiring lives in slice #21's docs/deploy.md addition.

### Justfile

Drop the `fly-*` group. Add a `heroku-*` group with the same verbs:

| Verb | Heroku command |
|---|---|
| `heroku-deploy` | `git push heroku main` |
| `heroku-logs` | `heroku logs --tail -a urbanist-atlas` |
| `heroku-config` | `heroku config -a urbanist-atlas` |
| `heroku-ssh` | `heroku run bash -a urbanist-atlas` |
| `heroku-loaddata` | `heroku run urbanist-atlas-server loaddata -a urbanist-atlas` |
| `db-backup` | `heroku pg:backups:capture -a urbanist-atlas` |

The justfile's existing dev recipes (`pg-up`, `pg-reset`, `api-run`,
`migrate-up`, etc.) are unaffected.

## PR disposition

### PR #11 — close + reopen on new branch

The slice-20 PR (`slice-20-fly-deploy-loaddata`) was held pending the
spike. Its loaddata code is correct and platform-portable; its
deploy-runbook half is Fly-specific.

**Action:** close PR #11. Cherry-pick the loaddata commits onto a
fresh branch (`slice-20-heroku-deploy-loaddata`). On the fresh
branch:

- Add `Procfile`.
- Add the `DATABASE_URL` fallback to the urfave/cli flag config (one
  small commit in `api/cmd/server/main.go` or a shared helper).
- Delete `fly.toml`.
- Delete `Dockerfile`.
- Rewrite `docs/deploy.md` end-to-end against Heroku (the slice-20
  branch wrote a Fly-targeted version — discard it, write the
  Heroku version fresh).
- Replace `fly-*` justfile recipes with `heroku-*` + `db-backup`.

The `urbanist-atlas-server loaddata` subcommand and its integration
test (`TestPipeline_LoaddataLoadAll`) survive unchanged.

### PR #12 — rebase + retarget

PR #12 (slice #21, Pages/DNS) is mostly unaffected — it appends a
Cloudflare Pages section to `docs/deploy.md` and adds
`web/public/_redirects`. After the Heroku PR lands, PR #12 needs:

- Rebase on the new `docs/deploy.md` (the file content is different
  now, but the slice-#21 append is structurally identical).
- DNS target for `qa-api` updated: was a CNAME to
  `urbanist-atlas.fly.dev`, becomes a CNAME to the Heroku-managed
  DNS target (`heroku domains:add` prints it).
- The Fly cert creation step is replaced by Heroku's
  `heroku domains:add qa-api.urbanistatlas.com` (ACM is automatic).

`web/public/_redirects` and the Pages env-var setup are unchanged.

### PR #9 — already merged; files become dead weight on this pivot

PR #9 (slice #19) added `Dockerfile` + `fly.toml`. Both are deleted
by the Heroku pivot. No revert needed — the deletes land naturally on
the new slice-20 branch's first commit. The PR-#9 commit stays in
history as the audit trail for the path we considered.

### PR #10 — unaffected

PR #10 (slice #23, `X-Atlas-Client` shared-secret middleware) is
host-agnostic and merges in either world.

## Cascading doc + config rewrites

Files updated by the same PR (or a sibling decision-only PR) that
records the Heroku choice. Note the split: the **`DATABASE_URL` rename**
also lands in this surface and touches the Go code + dev tooling.

### Docs

| File | Section | What changes |
|---|---|---|
| `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` | **Decision** | Filled in with "Heroku Basic dyno + Postgres Essential-0 (us)" and a one-paragraph rationale pointing at this design doc |
| `CLAUDE.md` | §Hosting + §Tech conventions | §Hosting: "Fly.io. Single Dockerfile, single binary, Fly Managed Postgres." → Heroku Basic dyno + Postgres Essential-0 (us region), buildpack-based deploys via `git push heroku main`. §Tech conventions: update the env-var list (`URBANIST_DB_URL` → `DATABASE_URL`) and add the one-line addendum that Postgres connection string follows the universal convention |
| `README.md` | §Deploy + the `api/` summary bullet | Drop the "Fly Managed Postgres" mention; point at this design doc + the rewritten `docs/deploy.md` |
| `api/README.md` | env-var usage examples | Rename `URBANIST_DB_URL` → `DATABASE_URL` in any inline examples |
| `docs/roadmap.md` | Slice #20 row | Rewrite the "what it ships" cell against Heroku |
| `docs/superpowers/specs/2026-05-18-qa-deploy-design.md` | Architecture table DB row + Slice #20 section | Replace MPG references with Heroku Essential-0; replace `URBANIST_DB_URL` with `DATABASE_URL` where mentioned |

### Code + config (the `DATABASE_URL` rename)

| File | What changes |
|---|---|
| `api/cmd/server/migrate.go` | `cli.EnvVars("URBANIST_DB_URL")` → `cli.EnvVars("DATABASE_URL")`; error message |
| `api/cmd/server/serve.go` | Same |
| `api/cmd/server/seed.go` | Same |
| `api/cmd/server/loadregions.go` | Same |
| `api/cmd/server/loadpostal.go` | Same |
| `justfile` | Any recipe exporting/referencing `URBANIST_DB_URL` |
| `mise.development.toml` | Rename the env entry |
| `mise.local.toml.example` | Rename if present |

## Files that survive **any** hosting outcome unchanged

- `api/cmd/server/loaddata.go` — orchestrates `LoadAll`; doesn't open
  a DB connection itself, so unaffected by the env-var rename
- `api/internal/loaddata/loaddata.go`
- `api/internal/store/postgres/loaddata_test.go` (testcontainers sets
  the connection string explicitly)
- `web/public/_redirects` (lands via PR #12)
- `.github/workflows/ci.yml`
- Every dev `pg-*` justfile recipe (they reference the dev Postgres
  port `:55432` directly, not via the env var)

Note: `mise.development.toml` *is* touched (env entry rename), so
removed from this list — see the rewrite table above.

## Verification (post-decision)

1. `just api-check` clean on the new slice-20 branch (vet, race tests,
   gen-no-diff, oapi sync).
2. `just api-test-integration` clean (testcontainers still uses
   `postgres:17-alpine`; the wire is unchanged).
3. Local smoke against `DATABASE_URL`:
   - With `DATABASE_URL` exported (and `URBANIST_DB_URL` unset),
     `cd api && go run ./cmd/server migrate up && go run ./cmd/server loaddata && go run ./cmd/server serve`
     all succeed (proves the env-var rename is complete across every
     subcommand).
   - `grep -rn "URBANIST_DB_URL" .` returns no Go-code, justfile, or
     mise.toml hits (only the cost-spike doc's "alternatives
     considered" history and any release-notes mention).
4. First Heroku deploy:
   - `heroku create urbanist-atlas --region us`
   - `heroku buildpacks:set heroku/go`
   - `heroku addons:create heroku-postgresql:essential-0`
   - `heroku config:set` for the four non-secret env vars + two
     secrets (commands from § Config above).
   - `git push heroku main` → release phase runs `migrate up`,
     deploy completes.
   - `heroku run urbanist-atlas-server loaddata` seeds.
5. Live smoke:
   - `curl -i https://urbanist-atlas.herokuapp.com/healthz` → 200
   - `curl -i https://urbanist-atlas.herokuapp.com/api/v1/lookup\?postal_code=10001\&country=US`
     → 401 problem+json (X-Atlas-Client gate active)
   - With the header: 200 with `X-Data-License: ODbL-1.0` and
     `X-Data-Attribution` response headers.
6. After PR #12 + DNS:
   `curl -i -H "X-Atlas-Client: $SECRET" https://qa-api.urbanistatlas.com/healthz`
   → 200.
7. `grep -ri "Fly Managed Postgres\|MPG\|flyctl mpg" .` returns
   only this doc's history section + the spike doc's comparison
   table + the qa-deploy-design.md's superseded-by note.

## Non-goals (deliberately out of scope)

- **Heroku Pipelines + separate QA/prod apps.** The original Fly plan
  was a *single app with environment-flavored hostnames* — QA URLs
  attach now, prod URLs attach later, QA URLs retire at prod cutover.
  This design preserves that intent on Heroku: one app, one DB
  add-on, no double bill, no environment-drift bugs, no data
  migration at cutover. Pipelines would add: a second app, a second
  DB add-on, a second secrets surface, and a `promote` button that
  doesn't buy anything `git push heroku main` doesn't already give
  us for a dogfood you control. **If Phase 2 traffic ever justifies
  isolation between QA and prod, the conversion is**: create a
  pipeline, fork the app to `urbanist-atlas-prod`, attach prod
  hostnames there, leave the original app as QA. No code change.
- **Heroku Review Apps per PR.** Tied to Pipelines, so out for the
  same reason. Cloudflare Pages' `*.pages.dev` previews cover the
  web-side per-PR isolation; for the API, dogfood-scale change
  velocity doesn't justify the per-PR app spin-up overhead. Add in
  Phase 2 if PR review against ephemeral API envs becomes valuable.
- **Multi-region (`us` + `eu` dynos).** The user base is US/CA per
  scope; the maintainer accepts personal latency from Europe during
  Phase 1 dogfooding. Multi-region is a Phase 2+ scaling question.
- **Heroku Postgres add-on backup → R2 sync.** Heroku's own retention
  + WAL off-premise is adequate; cross-vendor backup duplication
  isn't justified at Phase 1 scale.
- **Custom log drain.** Heroku's `logs --tail` is enough for Phase 1.
  Papertrail / Logflare add-on is a Phase 2 question.

## Reversibility — migration back to Fly

This section exists because the Heroku pivot is a Phase 1 decision
that we want to be able to walk back from if dogfood reveals a fit
problem. The cost of reversal is bounded as long as we hold the line
on prod hostnames; the decision deadline is **before
`api.urbanistatlas.com` attaches to the Heroku app**. Once the prod
hostname is live, cutover becomes customer-visible and a real
migration project.

### What stays unchanged

- All Go code: `pkg/atlas`, `internal/store/postgres`,
  `internal/loaddata`, the `loaddata` subcommand and its integration
  test.
- The OpenAPI contract (`api/openapi.yaml`) and generated types on
  both halves.
- Seed files in `api/seed/`.
- The CORS handler, the `X-Atlas-Client` middleware (PR #10).
- The Cloudflare Pages project, every Pages env var, the
  `web/public/_redirects`, every dev `pg-*` justfile recipe.
- `DATABASE_URL` as the connection-string env name — Fly's MPG also
  sets `DATABASE_URL`, and on a Fly sibling Postgres we'd construct
  the URL and set it as a Fly secret named `DATABASE_URL`. **The
  consolidation in this design makes us *more* portable, not less.**

### What needs reversing

- **Re-add `Dockerfile`** — git revert the deletion or restore from
  PR #9. Unchanged content.
- **Re-add `fly.toml`** — git revert the deletion or restore from
  PR #9. Update the `[env]` block to use `DATABASE_URL` (not
  `URBANIST_DB_URL`) if the original committed file still had the
  old name.
- **Add the sibling Postgres infra** per the spike's Finalist 1:
  `infra/postgres/{fly.toml,Dockerfile}` running `postgres:17-alpine`
  with a 1 GB volume. Spike doc has the design.
- **Delete `Procfile`** — Fly uses `fly.toml` + Dockerfile CMD.
- **Replace `heroku-*` justfile recipes with `fly-*`** — restore from
  PR #9's justfile diff or rewrite (5–10 thin wrappers around
  `flyctl`).
- **Rewrite `docs/deploy.md`** against the Fly sibling Postgres
  topology. The spike doc's Finalist 1 description is the source
  material.
- **Update CLAUDE.md §Hosting + README.md §Deploy** to point back at
  Fly.

### Data migration

```sh
# On Heroku — capture a full dump.
heroku pg:backups:capture -a urbanist-atlas
heroku pg:backups:download -a urbanist-atlas    # → latest.dump

# On Fly — restore into the new sibling Postgres.
# (After the Fly DB app is provisioned per spike Finalist 1.)
pg_restore --no-owner --no-acl --dbname="$FLY_DATABASE_URL" latest.dump
```

At Phase 1 dogfood scale (<100 MB likely), the dump + restore takes
minutes. Submissions or admin edits made between the capture and the
DNS cutover are the only lossy window — schedule the cutover during
low activity, or take a second capture immediately before the DNS
flip.

### DNS cutover

- Repoint the `qa-api.urbanistatlas.com` CNAME from the Heroku-managed
  target to `urbanist-atlas.fly.dev`.
- TTL-bounded (5 min on Cloudflare unless raised).
- Tear down the Heroku app + add-on (`heroku apps:destroy`) after
  verifying Fly serves traffic.

### Estimate

1–2 evening sessions, dominated by the Fly sibling Postgres setup +
GHA-cron backup wiring (which the spike doc has speced out). The
Heroku → Fly mechanics are mostly file restores from PR #9 plus the
data move.

### Decision deadline

**Before attaching `api.urbanistatlas.com` to the Heroku app.** Up
to that point, the only customer-visible surface is
`qa.urbanistatlas.com` (Pages, unchanged) and
`qa-api.urbanistatlas.com` (a CNAME we own and can repoint). After
prod attaches, a migration involves either a prod-hostname cutover
window or running Heroku + Fly in parallel temporarily — both
real projects, not weekend work.

## Future migration: QA → prod

Configuration-only, same as the Fly path was:

1. **Cloudflare Pages**: add `urbanistatlas.com` (apex) as a second
   custom domain to the same project; add DNS CNAMEs for apex +
   `www`.
2. **Heroku**: `heroku domains:add api.urbanistatlas.com -a urbanist-atlas`;
   add DNS CNAME for `api` pointing at the Heroku-managed target.
3. Update `URBANIST_CORS_ORIGINS` via `heroku config:set` to include
   `https://urbanistatlas.com`. Heroku redeploys automatically when
   config changes.
4. Pages dashboard: point `VITE_API_BASE` at
   `https://api.urbanistatlas.com`; redeploy.
5. After prod verification, remove QA hostnames (drop Pages custom
   domain, `heroku domains:remove qa-api.urbanistatlas.com`, drop
   `qa.urbanistatlas.com` from `URBANIST_CORS_ORIGINS`, drop DNS
   records).

No code change for the transition. The Heroku app, the Pages project,
the Postgres add-on, and every commit hash are reused as-is.
