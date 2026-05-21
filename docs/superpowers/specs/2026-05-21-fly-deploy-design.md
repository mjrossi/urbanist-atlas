# Fly deploy design — Phase 1 dogfood host

**Status:** Shipped and live (2026-05-21). Both Fly apps
(`urbanist-atlas`, `urbanist-atlas-db`), the Cloudflare Workers +
Pages project, and the `qa.urbanistatlas.com` /
`qa-api.urbanistatlas.com` hostnames are operating. Several bring-up
bugs were discovered and fixed in follow-up commits on the bring-up
branch (release_command ENTRYPOINT doubling, ext4 lost+found
collision with initdb, Fly volume root permissions blocking postgres
user, `[restart]` schema, Workers Static Assets unification
discovery, `_redirects` validator stricter than Pages, build-context
mismatch for the postgres Dockerfile, multi-line paste hazard for
`DATABASE_URL`); all symptoms + remedies are captured in
`docs/deploy.md` Troubleshooting. The R2 backup workflow setup is
the only remaining bring-up task at write time.

**Related:**
- [`../../roadmap.md`](../../roadmap.md) — Phase 1 deploy row
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §Hosting — Fly + sibling
  Postgres summary
- [`../../deploy.md`](../../deploy.md) — operator runbook

## Why this exists

The Phase 1 dogfood needs a real, public-internet home for the API
and the SPA so the maintainer can shake out schema + query bugs with
real postal-code lookups before the repo flips public. The design
target is **~$5/mo all-in**, vendor-portable data (our own Postgres
image, our own `pg_dump`), and a topology where the API host can be
replaced without touching the database half.

**The hosting picks:**

- **API**: Fly app, region `iad` (Virginia, US East). Multi-stage
  Alpine Docker build. Region picked for proximity to US East
  population centroid + the Cloudflare PoP that fronts the SPA.
- **Database**: a *sibling* Fly app running plain
  `postgres:17-alpine` with a 1 GB volume. Same image as the
  testcontainers integration suite, so dev and prod share an
  identical wire.
- **Web**: Cloudflare Workers + Pages (Static Assets), prod branch
  `main`, configured by `wrangler.jsonc` at the repo root.
- **Backups**: nightly GitHub Actions cron does `pg_dump | gzip`
  via `flyctl ssh console`, uploads to Cloudflare R2 with a 30-day
  retention rule.

The DB-as-sibling-app choice (rather than a managed Postgres add-on)
is the load-bearing decision: it keeps Postgres a plain container
image we control, dumps with vanilla `pg_dump`, and decouples API
deploys from DB state.

## Strategic goal

Get Urbanist Atlas to `qa.urbanistatlas.com` /
`qa-api.urbanistatlas.com` on Fly, with a runbook the maintainer can
execute in one sitting.

The bring-up absorbs the original deploy + CORS-lockdown +
end-to-end-smoke work:

- Deploy code: `Dockerfile` + `fly.toml` + `infra/postgres/`
- DNS retargeted to Fly's edge for the API; Cloudflare for the SPA
- Production CORS lockdown audit
- End-to-end smoke as a `just smoke` recipe

The Postgres connection string uses the universal `DATABASE_URL`
env-var name — every managed-Postgres host (Fly MPG, Render, Neon,
Railway) sets the same name automatically, and Fly secrets honor
whatever name we choose.

## Design

### Architecture — Fly app + sibling Postgres app

| Component | Resource | Initial hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` (Virginia, US East), shared-cpu-1x / 256 MB, `min_machines_running = 1` | `urbanist-atlas.fly.dev` + `qa-api.urbanistatlas.com` (Fly-issued Let's Encrypt cert) |
| DB | Fly app `urbanist-atlas-db`, region `iad`, shared-cpu-1x / 1 GB, image `postgres:17-alpine`, 1 GB volume `pgdata` at `/var/lib/postgresql/data`, always-on | `urbanist-atlas-db.internal:5432` (private 6PN, no public exposure) |
| Web | Cloudflare Workers + Pages project `urbanist-atlas` (Static Assets), prod branch `main`, configured by `wrangler.jsonc` at repo root | `qa.urbanistatlas.com` + `<version>-urbanist-atlas.<account>.workers.dev` (versioned previews) |
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
intermediaries.

The app is named *without* a `-qa` suffix — same Fly app hosts
production traffic later; only the hostnames mapped to it are
environment-flavored. QA → prod is a `flyctl certs add
api.urbanistatlas.com` + CORS update, not a re-architecture.

### Build mechanism — multi-stage Dockerfile

The Dockerfile lives at the **repo root** (Fly's standard) and uses
a two-stage Alpine build:

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
  `release_command = "migrate up"`).

### API `fly.toml`

```toml
app = "urbanist-atlas"
primary_region = "iad"

[build]
  dockerfile = "Dockerfile"

[deploy]
  release_command = "migrate up"

[env]
  URBANIST_PORT = "8080"
  URBANIST_LOG_FORMAT = "json"
  URBANIST_STORE = "postgres"
  URBANIST_SEED_DIR = "/app/seed"
  URBANIST_CORS_ORIGINS = "https://qa.urbanistatlas.com,*.<account>.workers.dev"

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
  every deploy; failures block the deploy.
- **`min_machines_running = 1`**: keeps one machine warm so dogfooders
  don't hit a cold-start. `auto_stop_machines = "suspend"` keeps any
  extra machine billing-efficient on idle.
- **CORS** is set in `[env]` (non-secret); the Phase 1 lockdown is
  `qa.urbanistatlas.com` + `*.<account>.workers.dev` (matches CLAUDE.md launch
  strategy).
- **`URBANIST_SEED_DIR=/app/seed`** points the binary at where the
  Dockerfile COPYs the seed files.
- **`URBANIST_PORT=8080`** matches the Dockerfile EXPOSE.

### Sibling Postgres app — `infra/postgres/fly.toml`

```toml
app = "urbanist-atlas-db"
primary_region = "iad"

[build]
  # See infra/postgres/Dockerfile + entrypoint-fly.sh for why we wrap
  # postgres:17-alpine instead of using it directly.
  dockerfile = "Dockerfile"

[env]
  POSTGRES_DB = "urbanist_atlas"
  POSTGRES_USER = "urbanist"
  # POSTGRES_PASSWORD is set via `flyctl secrets set` (write-only;
  # captured locally at provision time for DATABASE_URL construction).
  # PGDATA points at a SUBDIRECTORY of the mount, not the mount root:
  # Fly's ext4 volumes auto-include a `lost+found` directory which
  # would otherwise trip initdb's "directory exists but is not empty"
  # guard on first start.
  PGDATA = "/var/lib/postgresql/data/pgdata"

[mounts]
  source = "pgdata"
  destination = "/var/lib/postgresql/data"

# Default `on-failure max_retries=10` would let a transient blip
# strand the DB stopped indefinitely. `always` keeps Fly restarting;
# the data lives on the pgdata volume so restarts are safe.
# Fly's schema requires `restart` as an array of tables, hence [[ ]].
[[restart]]
  policy = "always"

[[vm]]
  cpu_kind = "shared"
  cpus = 1
  memory_mb = 1024
```

No `[[services]]` block — the DB is internal-only via Fly 6PN.
No `[http_service]` for the same reason. No `release_command` — the
`postgres:17-alpine` image initializes the database from the
`POSTGRES_*` / `PGDATA` env vars on first start.

The volume `pgdata` is created out-of-band: `flyctl volumes create
pgdata -a urbanist-atlas-db -r iad -s 1`.

### Postgres entrypoint wrapper

Fly mounts volumes as root:root mode 0755. The upstream
`postgres:17-alpine` `docker-entrypoint.sh` demotes to the `postgres`
user very early — before creating the PGDATA subdir — and `postgres`
can't write into a root-owned mount root.

`infra/postgres/Dockerfile` is a thin wrapper that runs as root (Fly
machine init starts as root), pre-creates the PGDATA subdir, chowns
the mount root to `postgres`, then exec's the upstream
`docker-entrypoint.sh` as before. The upstream entrypoint's own
demotion + initdb sequence then runs normally against a writable
PGDATA.

`infra/postgres/fly.toml` flips from `image = "postgres:17-alpine"`
to `dockerfile = "Dockerfile"` so Fly builds the wrapper rather than
pulling the upstream image directly.

### Secrets

Four Fly secrets on the API app (`flyctl secrets set ... -a urbanist-atlas`):

| Key | Value | Notes |
|---|---|---|
| `DATABASE_URL` | `postgres://urbanist:${PG_PASSWORD}@urbanist-atlas-db.internal:5432/urbanist_atlas?sslmode=disable` | Constructed from the password set on the DB app |
| `URBANIST_ADMIN_TOKEN` | `openssl rand -hex 32` | Phase 2 pre-stage; no-op until admin endpoints land |
| `URBANIST_CLIENT_SECRET` | `openssl rand -hex 32` | Phase 1 lockdown secret; must match `VITE_API_CLIENT_SECRET` in the Cloudflare Workers + Pages project's Build variables |
| `URBANIST_CORS_ORIGINS` | (already in `[env]`) | Set in fly.toml, not a secret |

One Fly secret on the DB app (`flyctl secrets set ... -a urbanist-atlas-db`):

| Key | Value | Notes |
|---|---|---|
| `POSTGRES_PASSWORD` | `openssl rand -hex 32` | Capture locally at set time — Fly secrets are write-only; needed for `DATABASE_URL` construction above |

Rotation procedure lives in `docs/deploy.md` §Secrets rotation.

### Backups — GHA cron + R2

A deterministic nightly logical dump backstops the data:

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

Combined with Fly's daily volume snapshots (5-day retention), this
gives us off-Fly durability + a deterministic pg_restore path.

### TLS, custom hostname

`qa-api.urbanistatlas.com` attaches to the Fly app via `flyctl certs
add qa-api.urbanistatlas.com -a urbanist-atlas`. Fly issues a Let's
Encrypt cert; `flyctl certs show` polls until Active (typically 1–5
min, can be up to 15). The Cloudflare DNS record for `qa-api` is a
CNAME to `urbanist-atlas.fly.dev` with **proxy OFF** (Fly handles
TLS termination at the edge).

### Justfile

`[group('fly')]` recipes wrap the day-to-day flyctl invocations:

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

## Files added by this slice

| File | Role |
|---|---|
| `Dockerfile` (repo root) | Multi-stage Go build (per § Build mechanism) |
| `fly.toml` (repo root) | API app config (per § API `fly.toml`) |
| `infra/postgres/fly.toml` | Sibling DB app config (per § Sibling Postgres app) |
| `infra/postgres/Dockerfile` + `entrypoint-fly.sh` | Thin wrapper over `postgres:17-alpine`; root-stage `chown` so the `postgres` user can write the PGDATA subdir on a Fly volume |
| `.github/workflows/backup.yml` | Nightly cron + workflow_dispatch (per § Backups) |
| `wrangler.jsonc` (repo root) | Cloudflare Workers + Pages Static Assets config (assets dir = `web/dist`, SPA fallback via `not_found_handling`) |
| `docs/deploy.md` | Operator runbook (executable companion to this design) |

## Files that survive unchanged

The bring-up does not touch:

- All Go code: `pkg/atlas`, `internal/store/postgres`,
  `internal/loaddata`, every `cmd/server/*` subcommand,
  `internal/httpapi/`
- `api/openapi.yaml` and generated types on both halves
- `api/seed/`
- The CORS handler, the `X-Atlas-Client` middleware
- The Cloudflare Workers + Pages project's Build env vars
  (`VITE_API_BASE`, `VITE_API_CLIENT_SECRET`, `NODE_VERSION`)
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

### CORS audit

1. `flyctl secrets list -a urbanist-atlas` shows `URBANIST_CORS_ORIGINS`
   set in `[env]`.
2. `curl -H "Origin: https://evil.example.com" -i .../api/v1/lookup?...`
   → no `Access-Control-Allow-Origin` header.
3. `curl -H "Origin: https://qa.urbanistatlas.com" -i ...` →
   `Access-Control-Allow-Origin: https://qa.urbanistatlas.com`.
4. `curl -H "Origin: https://abc123-urbanist-atlas.<account>.workers.dev" -i ...`
   → `Access-Control-Allow-Origin: https://abc123-urbanist-atlas.<account>.workers.dev`.

## Future migration: QA → prod

Configuration-only:

1. **Cloudflare Workers + Pages**: in the `urbanist-atlas` project
   Settings → Custom Domains, add `urbanistatlas.com` (apex) as a
   second custom domain. Cloudflare auto-creates the apex + `www`
   DNS records.
2. **Fly**: `flyctl certs add api.urbanistatlas.com -a urbanist-atlas`;
   add A + AAAA records for `api` pointing at the Fly anycast IPs
   (proxy OFF).
3. Update `URBANIST_CORS_ORIGINS` in `fly.toml`'s `[env]` (or via
   `flyctl secrets set` for deploy-decoupled rollout) to include
   `https://urbanistatlas.com`, then `flyctl deploy`.
4. Workers project's Build variables: point `VITE_API_BASE` at
   `https://api.urbanistatlas.com`; trigger a redeploy.
5. After prod verification, remove QA hostnames (`flyctl certs remove
   qa-api.urbanistatlas.com`, drop the QA custom domain from the
   Workers project, drop `qa.urbanistatlas.com` from
   `URBANIST_CORS_ORIGINS`, drop the `qa-api` A/AAAA records).

No code change for the transition. The Fly apps, the Workers
project, the Postgres volume, and every commit hash are reused
as-is.

## Non-goals (deliberately out of scope)

- **Multi-region Fly.** The user base is US/CA per scope; the
  maintainer accepts personal latency from Europe during Phase 1
  dogfooding. Multi-region is a Phase 2+ scaling question.
- **Fly Pipelines / Review Apps equivalents.** Workers' versioned
  preview URLs (`<version>-urbanist-atlas.<account>.workers.dev`)
  cover the web-side per-PR isolation; for the API, dogfood-scale
  change velocity doesn't justify the per-PR app spin-up overhead.
  Add in Phase 2 if it becomes valuable.
- **HA / Postgres failover.** Single-node, no failover. If the DB
  machine restarts we have a minute or two of downtime. Acceptable
  for Phase 1 dogfood.
- **Cross-vendor backup duplication.** R2 nightly dumps + Fly's daily
  volume snapshots are adequate; no S3-mirror.
- **Custom log drain.** `flyctl logs --tail` is enough for Phase 1.
  Papertrail / Logflare add-on is a Phase 2 question.
- **API keys / rate limiting / public CORS cutover** — Phase 2.
