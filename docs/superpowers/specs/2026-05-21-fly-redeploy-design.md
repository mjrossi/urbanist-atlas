# Fly redeploy design — Phase 1 dogfood host (supersedes the Heroku pivot)

**Status:** Shipped (deliverables) / pending execution. Slice #20.6
lands the code, config, and runbook in a single PR; no Fly resources
are provisioned yet at the time of writing.
**Supersedes:** [`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md)
(itself superseded the original Fly Managed Postgres path in
[`2026-05-18-qa-deploy-design.md`](./2026-05-18-qa-deploy-design.md)
§Slice #20).
**Related:**
- [`2026-05-18-hosting-cost-spike.md`](./2026-05-18-hosting-cost-spike.md)
  — the spike that originally compared Fly sibling Postgres against
  Heroku. This design picks up the spike's Finalist 1 recommendation,
  reversing the Heroku pivot before any Heroku resources were
  provisioned.
- [`2026-05-18-qa-deploy-design.md`](./2026-05-18-qa-deploy-design.md)
  — Architecture row "DB" + Slice #20 section get rewritten against
  this design.
- [`../../roadmap.md`](../../roadmap.md) slice #20 (retired) + new row
  #20.6.
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §Hosting — rewritten
  against Fly + sibling Postgres.
- [`../../deploy.md`](../../deploy.md) — operator runbook, rewritten
  end-to-end against Fly.

## Why this exists

The Heroku pivot ([`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md))
committed the Phase 1 dogfood to Heroku Basic dyno + Postgres Essential-0
at $12/mo, traded off against the spike's recommended Fly-sibling
Postgres ($5/mo) primarily on operator familiarity. The deliverables
shipped on 2026-05-19 (PR #14 / #12) but the runbook was never executed
— no Heroku app, no add-on, no Pages project, no DNS records.

In the meantime two concerns crystallized:

1. **Vendor risk on Heroku.** The maintainer lacks confidence in
   Heroku's long-term direction (post-Salesforce ownership turbulence,
   layoffs, free-tier removal history). Anchoring a long-lived
   directory project to a vendor of uncertain future trumps the
   familiarity dividend.
2. **No reason to host elsewhere "managed".** Fly Managed Postgres at
   $38/mo floor is still indefensible at this scale (the original
   spike's finding holds). But the *Fly platform itself* is fine — the
   maintainer used it positively before; Fly's flaw is MPG pricing,
   not Fly the platform.

The pivot is back to **the spike's original Finalist 1 recommendation**:
a Fly app for the API + a sibling Fly app running `postgres:17-alpine`
with a 1 GB volume. ~$5/mo total, vendor-portable data (our own
Postgres image, our own pg_dump), and the API host could be replaced
without touching the database half.

This is the third hosting design doc in three days. We're confident in
this one because:

- It's the spike's *original* recommendation; we have all the
  comparison work and have now lived with Heroku's tradeoffs in
  imagination long enough to know they don't fit.
- It minimizes vendor lock-in: Postgres is a plain
  `postgres:17-alpine` container with a volume; the data dumps with
  vanilla `pg_dump`. If Fly itself becomes a concern later, only the
  *API host* needs replacing — the data move is a routine restore.
- The cost is half of Heroku's; the spike's exhaustive comparison
  still holds.

## Strategic goal

Get Urbanist Atlas to `qa.urbanistatlas.com` / `qa-api.urbanistatlas.com`
on Fly, with the smallest viable code change relative to the Heroku
deliverables and a runbook the maintainer can execute in one sitting.
The slice (#20.6) absorbs:

- **#20** — rewritten end-to-end against Fly (Heroku artifacts deleted)
- **#21** — Pages code unchanged; DNS retargeted from Heroku CNAME to
  Fly's edge
- **#22** — production CORS lockdown audit
- **#25** — end-to-end smoke test as a `just smoke` recipe

The `DATABASE_URL` env-var rename (PR #14, slice #20) survives the
pivot — it's universally correct and Fly secrets honor the same name.

## Design

### Architecture — Fly app + sibling Postgres app

| Component | Resource | Initial hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` (Virginia, US East), shared-cpu-1x / 256 MB, `min_machines_running = 1` | `urbanist-atlas.fly.dev` + `qa-api.urbanistatlas.com` (Fly-issued Let's Encrypt cert) |
| DB | Fly app `urbanist-atlas-db`, region `iad`, shared-cpu-1x / 1 GB, image `postgres:17-alpine`, 1 GB volume `pgdata` at `/var/lib/postgresql/data`, always-on | `urbanist-atlas-db.internal:5432` (private 6PN, no public exposure) |
| Web | Cloudflare Pages project `urbanist-atlas`, prod branch `main` | `qa.urbanistatlas.com` + `<branch>.urbanist-atlas.pages.dev` (preview deploys per branch) |
| Backups | GitHub Actions cron `0 7 * * *` (02:00 ET nightly): `flyctl ssh console -a urbanist-atlas-db -C "pg_dump -U urbanist urbanist_atlas \| gzip"` → R2 via aws-cli with R2 endpoint; 30-day retention via R2 bucket lifecycle rule | Cloudflare R2 bucket `urbanist-atlas-backups` |

Two Fly apps in one Fly org, one Fly region. The API and DB are
separate apps because Fly volumes mount to a specific machine and
co-locating them would couple deploys; running Postgres as a sibling
keeps API deploys completely decoupled from DB state.

API ↔ DB connection (set as Fly secret `DATABASE_URL` on the API app):

```
postgres://urbanist:${PG_PASSWORD}@urbanist-atlas-db.internal:5432/urbanist_atlas?sslmode=disable
```

No TLS on the internal 6PN — private network, app-to-app traffic, no
intermediaries. Matches the spike's design.

The app is named *without* a `-qa` suffix — same Fly app hosts
production traffic later; only the hostnames mapped to it are
environment-flavored. QA → prod is a `flyctl certs add
api.urbanistatlas.com` + CORS update, not a re-architecture.

### Build mechanism — multi-stage Dockerfile

Replaces the Heroku buildpack. The Dockerfile lives at the **repo
root** (Fly's standard) and uses a two-stage Alpine build:

- **Stage 1 (builder):** `golang:1.26.3-alpine`. `CGO_ENABLED=0
  GOOS=linux go build -ldflags="-s -w"` → static binary at
  `/out/urbanist-atlas-server`. Same build flags as `just
  api-build-prod` (parity is a code-review concern; we don't install
  `just` inside the build stage).
- **Stage 2 (runtime):** `alpine:3.20` + `ca-certificates` + non-root
  `app` user. The binary lands at `/usr/local/bin/urbanist-atlas-server`;
  the seed directory copies to `/app/seed` from the repo's
  `api/seed/`.
- `ENTRYPOINT ["urbanist-atlas-server"]`, `CMD ["serve"]`. The
  `fly.toml` overrides the CMD where needed (e.g.,
  `release_command = "urbanist-atlas-server migrate up"`).

The historical slice-#19 Dockerfile (commit `e37c12f`) is the
reference for the structure; the only fix on restoration is dropping
any `URBANIST_DB_URL` references in favor of `DATABASE_URL`.

### Connection-string env: keep `DATABASE_URL`

PR #14 (slice #20) renamed `URBANIST_DB_URL` → `DATABASE_URL` across
every cli flag and dev tooling. **The rename was a portability win,
not Heroku-specific**, and Fly secrets honor any name we set. Keep the
rename. CLAUDE.md's convention note ("Postgres connection string
follows the universal `DATABASE_URL` convention") stays intact.

### Procfile → fly.toml

Replaces the Heroku `Procfile`. The API `fly.toml` at the repo root
declares:

```toml
app = "urbanist-atlas"
primary_region = "iad"

[build]
  dockerfile = "Dockerfile"

[deploy]
  release_command = "urbanist-atlas-server migrate up"

[env]
  URBANIST_PORT = "8080"
  URBANIST_LOG_FORMAT = "json"
  URBANIST_STORE = "postgres"
  URBANIST_SEED_DIR = "/app/seed"
  URBANIST_CORS_ORIGINS = "https://qa.urbanistatlas.com,*.pages.dev"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "suspend"
  auto_start_machines = true
  min_machines_running = 1
  processes = ["app"]

  [[http_service.checks]]
    grace_period = "10s"
    interval = "30s"
    method = "GET"
    timeout = "5s"
    path = "/healthz"

[[vm]]
  cpu_kind = "shared"
  cpus = 1
  memory_mb = 256
```

- **`release_command`**: runs `migrate up` in a one-off machine on
  every deploy; failures block the deploy. Functional parity with
  Heroku's release phase.
- **`min_machines_running = 1`**: keeps one machine warm so dogfooders
  don't hit a cold-start. `auto_stop_machines = "suspend"` keeps any
  extra machine billing-efficient on idle.
- **CORS** is set in `[env]` (non-secret); the Phase 1 lockdown is
  `qa.urbanistatlas.com` + `*.pages.dev` (matches CLAUDE.md launch
  strategy).
- **`URBANIST_SEED_DIR=/app/seed`** points the binary at where the
  Dockerfile COPYs the seed files.
- **`URBANIST_PORT=8080`** matches the Dockerfile EXPOSE.

### Sibling Postgres app — `infra/postgres/fly.toml`

```toml
app = "urbanist-atlas-db"
primary_region = "iad"

[build]
  image = "postgres:17-alpine"

[env]
  POSTGRES_DB = "urbanist_atlas"
  POSTGRES_USER = "urbanist"
  # POSTGRES_PASSWORD is set via `flyctl secrets set` (write-only;
  # captured locally at provision time for DATABASE_URL construction).
  # PGDATA hint: postgres:17-alpine defaults to /var/lib/postgresql/data,
  # which matches the [mounts] destination below — no override needed.

[mounts]
  source = "pgdata"
  destination = "/var/lib/postgresql/data"

[[vm]]
  cpu_kind = "shared"
  cpus = 1
  memory_mb = 1024
```

No `[[services]]` block — the DB is internal-only via Fly 6PN.
No `[http_service]` for the same reason. No `release_command` — the
`postgres:17-alpine` image initializes the database from the
`POSTGRES_*` env vars on first start.

The volume `pgdata` is created out-of-band: `flyctl volumes create
pgdata -a urbanist-atlas-db -r iad -s 1`.

### Secrets

Four Fly secrets on the API app (`flyctl secrets set ... -a urbanist-atlas`):

| Key | Value | Notes |
|---|---|---|
| `DATABASE_URL` | `postgres://urbanist:${PG_PASSWORD}@urbanist-atlas-db.internal:5432/urbanist_atlas?sslmode=disable` | Constructed from the password set on the DB app |
| `URBANIST_ADMIN_TOKEN` | `openssl rand -hex 32` | Phase 2 pre-stage; no-op until admin endpoints land |
| `URBANIST_CLIENT_SECRET` | `openssl rand -hex 32` | Phase 1 lockdown secret; must match `VITE_API_CLIENT_SECRET` in the Cloudflare Pages dashboard |
| `URBANIST_CORS_ORIGINS` | (already in `[env]`) | Set in fly.toml, not a secret |

One Fly secret on the DB app (`flyctl secrets set ... -a urbanist-atlas-db`):

| Key | Value | Notes |
|---|---|---|
| `POSTGRES_PASSWORD` | `openssl rand -hex 32` | Capture locally at set time — Fly secrets are write-only; needed for `DATABASE_URL` construction above |

Rotation procedure lives in `docs/deploy.md` §Secrets rotation
(rewritten for Fly).

### Backups — GHA cron + R2

Heroku Essential-0 included near-PITR via Aurora WAL off-premise; we
lose that. The replacement is a deterministic nightly logical dump:

- **GitHub Actions workflow** `.github/workflows/backup.yml`:
  - Cron: `0 7 * * *` (02:00 ET, 07:00 UTC)
  - `workflow_dispatch` for manual triggering (first-run verification)
  - Steps: setup flyctl → `flyctl ssh console -a urbanist-atlas-db -C
    "pg_dump -U urbanist urbanist_atlas | gzip" > backup.sql.gz` →
    install awscli → `aws s3 cp backup.sql.gz
    s3://urbanist-atlas-backups/$(date +%Y-%m-%d).sql.gz
    --endpoint-url https://${CF_ACCOUNT_ID}.r2.cloudflarestorage.com`
  - Required GHA secrets: `FLY_API_TOKEN`, `CF_ACCOUNT_ID`,
    `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`

- **Retention**: 30 days, configured at the R2 bucket level via
  Cloudflare's object lifecycle rule. The GHA workflow uploads only;
  R2 handles expiration.

- **On-demand**: `just db-backup` wraps the same pipeline but writes
  to a local timestamped file (no R2 upload) for the maintainer's
  ad-hoc snapshot habit.

- **Restore**: `just db-restore <file>` placeholder + usage comment
  pointing at `pg_restore --dbname="$DATABASE_URL"`. Manual exercise
  at first restore.

At expected dogfood scale (<100 MB), `pg_dump` over `flyctl ssh
console` is fine. If the dataset grows past ~500 MB, switch the
workflow to `flyctl proxy 15432:5432 -a urbanist-atlas-db &; pg_dump
postgresql://...:15432/...` to avoid SSH timeouts.

### TLS, custom hostname

`qa-api.urbanistatlas.com` attaches to the Fly app via `flyctl certs
add qa-api.urbanistatlas.com -a urbanist-atlas`. Fly issues a Let's
Encrypt cert; `flyctl certs show` polls until Active (typically 1–5
min, can be up to 15). The Cloudflare DNS record for `qa-api` is a
CNAME to `urbanist-atlas.fly.dev` with **proxy OFF** (Fly handles
TLS termination at the edge).

### Justfile

Drop the `[group('heroku')]`. Add a `[group('fly')]` with the same
verbs, plus a refreshed `[group('smoke')]`:

| Verb | Fly command |
|---|---|
| `fly-deploy` | `flyctl deploy -a urbanist-atlas` |
| `fly-deploy-db` | `flyctl deploy -a urbanist-atlas-db -c infra/postgres/fly.toml` |
| `fly-logs` | `flyctl logs -a urbanist-atlas` |
| `fly-logs-db` | `flyctl logs -a urbanist-atlas-db` |
| `fly-secrets` | `flyctl secrets list -a urbanist-atlas` |
| `fly-ssh` | `flyctl ssh console -a urbanist-atlas` |
| `fly-loaddata` | `flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server loaddata"` |
| `db-backup` | local `pg_dump | gzip` via `flyctl ssh console` |
| `db-restore` | `pg_restore` placeholder + usage note |
| `smoke` | curl checks against `qa-api.urbanistatlas.com` |

The justfile's existing dev recipes (`pg-up`, `pg-reset`, `api-run`,
`migrate-up`, etc.) are unaffected — they target the local docker
Postgres on `:55432`.

## PR disposition

This slice lands as a single PR off `main`:

- Branch: `slice-20-6-fly-redeploy`
- Atomic commit on the slice's deliverables (no intermediate broken
  states — leaving stale Heroku justfile recipes without `Procfile`
  would be churn)
- Conventional-commit subject: `feat(deploy): slice #20.6 — pivot
  from Heroku to Fly + sibling Postgres`

## Cascading doc + config rewrites

Files updated by the same PR:

### Docs

| File | Section | What changes |
|---|---|---|
| `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` | **Decision** | Append a paragraph noting the 2026-05-21 pivot back to Finalist 1; link to this design doc |
| `docs/superpowers/specs/2026-05-18-heroku-deploy-design.md` | top | Add `**Status:** Superseded by [`2026-05-21-fly-redeploy-design.md`](./2026-05-21-fly-redeploy-design.md) (2026-05-21).` under the existing Status line |
| `docs/superpowers/specs/2026-05-18-qa-deploy-design.md` | Architecture row DB + Slice #20 section | Replace Heroku Essential-0 with Fly sibling Postgres; replace `Procfile` references with `fly.toml` |
| `CLAUDE.md` | §Hosting | Replace Heroku paragraph with Fly description (region `iad`, multi-stage Dockerfile, sibling postgres:17-alpine app, R2 backups) |
| `README.md` | §Deploy | Update host references; point at this design doc |
| `docs/roadmap.md` | Gatekeeping table | Retire #20 (mirror #19 retirement pattern); insert #20.6 row; update #21/#22/#25 status |
| `docs/deploy.md` | §Heroku, §Pages DNS, §Secrets, §QA→prod, §Troubleshooting | Rewrite end-to-end against Fly + sibling Postgres |

### Code + config

| File | What changes |
|---|---|
| `Dockerfile` (new, repo root) | Multi-stage Go build (per § Build mechanism) |
| `fly.toml` (new, repo root) | API app config (per § Procfile → fly.toml) |
| `infra/postgres/fly.toml` (new) | Sibling DB app config (per § Sibling Postgres app) |
| `.github/workflows/backup.yml` (new) | Nightly cron + workflow_dispatch (per § Backups) |
| `justfile` | Delete `[group('heroku')]` lines 274-316; add `[group('fly')]` + refreshed `[group('smoke')]` (per § Justfile) |
| `Procfile` | **Delete** — Heroku-only artifact |

## Files that survive unchanged

The hosting pivot does not touch:

- All Go code: `pkg/atlas`, `internal/store/postgres`,
  `internal/loaddata`, every `cmd/server/*` subcommand,
  `internal/httpapi/`
- `api/openapi.yaml` and generated types on both halves
- `api/seed/`
- The CORS handler, the `X-Atlas-Client` middleware
- Cloudflare Pages config: `web/public/_redirects`, Pages dashboard
  env vars
- `.github/workflows/ci.yml` — CI tests run the same testcontainers
  Postgres
- All dev `pg-*` justfile recipes — local Postgres lifecycle on `:55432`
- `api/migrations/` — same migrations run via Fly's release_command

## Verification

### Pre-merge (code/config in repo)

1. `just api-check` clean (vet, race tests, sqlc-gen-no-diff,
   oapi-no-diff).
2. `just api-test-integration` clean (testcontainers uses
   `postgres:17-alpine`, exact same image as the Fly DB).
3. `just web-check` clean.
4. `docker build -t urbanist-atlas-test .` builds clean (multi-stage
   Dockerfile compiles + Alpine runtime stage succeeds).
5. `grep -ri "heroku" .` returns only superseded-design-doc status
   notes, the cost-spike comparison table (history), commit messages.
6. `grep -ri "URBANIST_DB_URL" .` returns nothing in code/config/docs.
7. `grep -ri "Procfile" .` returns only its deletion in git history.

### Post-provisioning (operator runs after the runbook)

1. `flyctl status -a urbanist-atlas` shows machine in `iad`,
   health-check passing.
2. `flyctl status -a urbanist-atlas-db` shows Postgres machine
   running, volume attached, no public services.
3. `flyctl logs -a urbanist-atlas | head -50` shows clean startup
   (no DB-connect errors, no missing-config panics).
4. `curl -i https://urbanist-atlas.fly.dev/healthz` → 200.
5. `curl -i ".../api/v1/lookup?postal_code=10001&country=US"` → 401
   `application/problem+json` (`X-Atlas-Client` gate).
6. With header: 200 JSON; response includes `X-Data-License:
   ODbL-1.0`, `X-Data-Attribution`, and `meta.license /
   attribution_url / generated_at`.
7. After DNS + certs: `curl -i https://qa-api.urbanistatlas.com/healthz`
   → 200. Same lookup → 200 with attribution.
8. `https://qa.urbanistatlas.com` loads in a browser, performs a real
   lookup.
9. `just smoke` → all PASS.
10. Manually trigger `backup.yml` → workflow succeeds → R2 bucket
    contains a dated `.sql.gz`; downloaded file's `pg_restore --list`
    shows expected tables.

### CORS audit (slice #22 absorbed)

1. `flyctl secrets list -a urbanist-atlas` shows `URBANIST_CORS_ORIGINS`
   set in `[env]`.
2. `curl -H "Origin: https://evil.example.com" -i .../api/v1/lookup?...`
   → no `Access-Control-Allow-Origin` header.
3. `curl -H "Origin: https://qa.urbanistatlas.com" -i ...` →
   `Access-Control-Allow-Origin: https://qa.urbanistatlas.com`.
4. `curl -H "Origin: https://abc123.urbanist-atlas.pages.dev" -i ...`
   → `Access-Control-Allow-Origin: https://abc123.urbanist-atlas.pages.dev`.

## Reversibility — migration back to Heroku

The Heroku deploy never executed; no live state to migrate. Reverting
is a `git revert` of this slice's commit (Heroku-shaped files —
`Procfile`, `heroku-*` justfile recipes, `docs/deploy.md` Heroku
sections — are restored from git history). The
`heroku-deploy-design.md` doc is still in the tree, just marked
Superseded; reverting flips it back to Active.

The deadline for cheap reversibility is **before any prod hostname
attaches**. Up to that point, `qa.urbanistatlas.com` and
`qa-api.urbanistatlas.com` are CNAMEs we own; the cutover is a DNS
flip. After prod attaches, reversal becomes a customer-visible
migration project.

## Future migration: QA → prod

Configuration-only:

1. **Cloudflare Pages**: add `urbanistatlas.com` (apex) as a second
   custom domain to the same project; add DNS CNAMEs for apex + `www`.
2. **Fly**: `flyctl certs add api.urbanistatlas.com -a urbanist-atlas`;
   add DNS CNAME for `api` → `urbanist-atlas.fly.dev` (proxy OFF).
3. Update `URBANIST_CORS_ORIGINS` via `flyctl secrets set` to include
   `https://urbanistatlas.com`. Fly redeploys automatically on
   secret change.
4. Pages dashboard: point `VITE_API_BASE` at
   `https://api.urbanistatlas.com`; redeploy.
5. After prod verification, remove QA hostnames (`flyctl certs remove
   qa-api.urbanistatlas.com`, drop Pages QA custom domain, drop
   `qa.urbanistatlas.com` from `URBANIST_CORS_ORIGINS`, drop DNS
   records).

No code change for the transition. The Fly apps, the Pages project,
the Postgres volume, and every commit hash are reused as-is.

## Non-goals (deliberately out of scope)

- **Multi-region Fly.** The user base is US/CA per scope; the
  maintainer accepts personal latency from Europe during Phase 1
  dogfooding. Multi-region is a Phase 2+ scaling question.
- **Fly Pipelines / Review Apps equivalents.** Pages `*.pages.dev`
  previews cover the web-side per-PR isolation; for the API,
  dogfood-scale change velocity doesn't justify the per-PR app
  spin-up overhead. Add in Phase 2 if it becomes valuable.
- **HA / Postgres failover.** Single-node, no failover. If the DB
  machine restarts we have a minute or two of downtime. Acceptable
  for Phase 1 dogfood.
- **Cross-vendor backup duplication.** R2 nightly dumps + Fly's daily
  volume snapshots are adequate; no S3-mirror.
- **Custom log drain.** `flyctl logs --tail` is enough for Phase 1.
  Papertrail / Logflare add-on is a Phase 2 question.
- **API keys / rate limiting / public CORS cutover** (#26 / #27 / #28)
  — Phase 2.
