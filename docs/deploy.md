# Deploy runbook

Operational guide for `urbanist-atlas`. The original Postgres-backed
design lives at
[`docs/superpowers/specs/2026-05-21-fly-deploy-design.md`](./superpowers/specs/2026-05-21-fly-deploy-design.md);
the runtime has since moved to a stateless, file-backed shape — this
file is the current playbook.

> **Status: live on apex since 2026-05-27.** Fly API + Cloudflare
> Workers + DNS + the Phase 1 `X-Atlas-Client` shared-secret gate
> serve `urbanistatlas.com` + `api.urbanistatlas.com`. The sibling
> Postgres app and its nightly backup workflow were retired with the
> file-store cutover; reads come straight from the `api/seed/`
> bundle baked into the API image. The shared-secret gate is a
> Phase 1 holdover; it comes off with slices #26–#28 (API keys +
> rate limiting + secret removal). The `qa.urbanistatlas.com` /
> `qa-api.urbanistatlas.com` hostnames were the pre-launch
> dogfooding origins; their retirement is documented at the bottom
> of this file (§ QA hostname retirement).

## Hosting topology

| Component | Resource | Hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` (Virginia, US East), shared-cpu-1x / 256 MB. Read path is stateless: `api/seed/` is baked into the image and loaded into an in-memory FileStore at boot. Writes (submissions only) land in a SQLite DB at `/data/atlas.db` on the `atlas_data` Fly volume (1 GiB, ~$0.15/mo). | `api.urbanistatlas.com` |
| Web | Cloudflare Workers + Pages project `urbanist-atlas` (Static Assets), prod branch `main`, configured by `wrangler.jsonc` at repo root | `urbanistatlas.com` |
| Web previews | `<version>-urbanist-atlas.<account>.workers.dev` | Auto-provisioned per non-`main` deploy via `wrangler versions upload` |

The apex hostnames attach directly to the same apps/project; nothing
carries a `-qa` or `-prod` suffix, so future hostname moves are pure
DNS + Workers custom-domain swaps with no rebuilds or data migration.

## Deploys

Day-to-day, deploys are automated.

| Component | Trigger | Mechanism |
|---|---|---|
| **API (Fly)** | push to `main` | [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) → `deploy-api` job runs `flyctl deploy --remote-only`. The image bundles `api/seed/`, so a deploy is a code+data deploy in one shot — no release command, no migrations. |
| **Web (Cloudflare Workers + Pages)** | push to `main` | Cloudflare dashboard git integration (`npx wrangler deploy` on `main`; `npx wrangler versions upload` for previews). Configured outside this repo. |
| **Seed data** | push to `main` (along with code) | Editing `api/seed/**` + merging is the entire workflow. The next API deploy ships the new bundle. There is no separate `loaddata` step. |
| **Weekly vuln scan** | scheduled | [`.github/workflows/govulncheck.yml`](../.github/workflows/govulncheck.yml) on Mondays 12:00 UTC. Non-blocking. |

### When to deploy manually

- **API.** Default path is `git push → merge`. Fall back to
  `just fly-deploy` when GitHub Actions is degraded, when you need to
  deploy a non-`main` branch for a hot-fix, or when you want to watch
  the build locally. For an Actions re-deploy of current `main` without
  an empty commit: `gh workflow run ci.yml --ref main`.
- **Web.** Cloudflare dashboard → Workers & Pages → `urbanist-atlas` →
  Deployments → Retry latest. Or `cd web && npx wrangler deploy` from
  a maintainer machine with `wrangler` authenticated.

### Rollback

- **API.** `flyctl releases list -a urbanist-atlas` shows version
  history; `flyctl deploy --image registry.fly.io/urbanist-atlas:<tag>`
  rolls back to a prior image. Because seed data ships *in* the image,
  a rollback also rolls back the data — useful if a bad seed edit hit
  prod.
- **Web.** Cloudflare dashboard → Deployments → "Promote to
  production" on a prior successful build.

### GitHub Actions secrets

| Secret | Scope | Used by | How to issue |
|---|---|---|---|
| `FLY_API_TOKEN_DEPLOY` | deploy-only on `urbanist-atlas` | `ci.yml` → `deploy-api` | `flyctl tokens create deploy -a urbanist-atlas --expiry 8760h` |

Rotate by re-issuing the token and `gh secret set FLY_API_TOKEN_DEPLOY`
with the new value; flyctl picks up the new token on the next workflow
run.

## Application secrets (Fly)

| Secret | Purpose | How to set |
|---|---|---|
| `URBANIST_ADMIN_TOKEN` | bearer token for `/api/v1/admin/*` endpoints (submission moderation). Empty → admin endpoints return 503. | `flyctl secrets set URBANIST_ADMIN_TOKEN=<value> -a urbanist-atlas` |
| `URBANIST_CLIENT_SECRET` | Phase 1 shared-secret `X-Atlas-Client` gate; mirrored to the SPA build as `VITE_API_CLIENT_SECRET` | `flyctl secrets set URBANIST_CLIENT_SECRET=<value> -a urbanist-atlas` |
| `URBANIST_GITHUB_TOKEN` | Fine-grained PAT scoped to this repo only (Contents R/W + Pull requests R/W). Drives the promotion-PR worker on submission approval. Empty → approval still flips status but `promotion_error="worker disabled (no token configured)"`. | `flyctl secrets set URBANIST_GITHUB_TOKEN=<pat> -a urbanist-atlas` |

Generate the two random secrets with `openssl rand -hex 32` (pipe
directly into `flyctl secrets set`, don't capture into a variable
first — fewer places the value sits in shell history). The client
secret must match the value built into the SPA bundle.

The GitHub PAT is issued at
[`github.com/settings/personal-access-tokens/new`](https://github.com/settings/personal-access-tokens/new):
"Only select repositories" → `mjrossi/urbanist-atlas`; "Repository
permissions" → Contents (Read and write) + Pull requests (Read and
write); leave everything else at "No access". A 1-year expiry is
the default; rotate on schedule.

Non-secret runtime config lives in `fly.toml`'s `[env]` block (where
each var carries an explanatory comment), not in Fly secrets:
`URBANIST_DB_PATH` (`/data/atlas.db`), `URBANIST_SUBMISSIONS_RATE_PER_HOUR`
(per-IP submission cap), `URBANIST_COVERAGE_SAMPLE_RATE` (empty-result
sampling), `URBANIST_CORS_ORIGINS`, `URBANIST_PORT`, and
`URBANIST_LOG_FORMAT`. Edit them there and redeploy. The private
Prometheus scrape port is set by `fly.toml`'s `[metrics]` block, not
`[env]` (`URBANIST_METRICS_PORT` exists as an env override but is
unset in production).

## Prerequisites

- [flyctl](https://fly.io/docs/flyctl/install/) installed and
  authenticated.
- Cloudflare account with access to the `urbanistatlas.com` zone.
- This repo cloned locally; `Dockerfile` and `fly.toml` live at the
  repo root.

## Bring-up from a clean Fly + Cloudflare account

These steps assume the repo's current state (single Fly app, no DB).
The original Postgres-included bring-up procedure is preserved in the
git history of this file if it ever needs to be replayed.

### 1. Create the Fly app + volume + secrets

```sh
flyctl apps create urbanist-atlas --org <your-org>
flyctl volumes create atlas_data --size 1 --region iad -a urbanist-atlas
flyctl secrets set \
  URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  URBANIST_CLIENT_SECRET="$(openssl rand -hex 32)" \
  URBANIST_GITHUB_TOKEN="<paste fine-grained PAT here>" \
  -a urbanist-atlas
flyctl deploy --remote-only -a urbanist-atlas
```

The Dockerfile bakes `api/seed/` into the binary via `//go:embed`,
so reads are served from memory; writes (submissions) land in the
SQLite DB on the mounted volume at `/data/atlas.db`. Boot should log
`store initialized kind=file regions=<n> orgs=<n>` and
`submission store initialized path=/data/atlas.db` within ~1s, plus
a one-time `goose: successfully migrated database to version: 1`
on the very first boot against a fresh volume.

### 2. DNS

Create the A/AAAA records in Cloudflare's `urbanistatlas.com` zone
(Cloudflare proxy **off** so Fly's edge handles TLS termination):

| Host | Target |
|---|---|
| `api.urbanistatlas.com` | `urbanist-atlas.fly.dev` (A + AAAA from `flyctl ips list`) |

Then issue a Fly cert:

```sh
flyctl certs create api.urbanistatlas.com -a urbanist-atlas
flyctl certs list -a urbanist-atlas   # wait for "ready"
```

### 3. Cloudflare Workers + Pages

Provision `urbanist-atlas` in the Cloudflare dashboard (Workers &
Pages → Create application → Connect to git). Production branch =
`main`, build command = `cd web && npm ci && npm run build`,
output directory = `web/dist`. The `wrangler.jsonc` at the repo root
carries asset handling and SPA fallback. Response headers — including
the Content-Security-Policy — live in `web/public/_headers`, which
Vite copies to `dist/_headers` at build time (the same `_headers`
convention Cloudflare Pages uses; Workers Static Assets honors it).

Set the SPA's API base + client secret as Workers environment
variables:

| Variable | Value |
|---|---|
| `VITE_API_BASE` | `https://api.urbanistatlas.com` |
| `VITE_API_CLIENT_SECRET` | the same value set on Fly as `URBANIST_CLIENT_SECRET` |

Attach `urbanistatlas.com` to the Workers project under custom
domains; Cloudflare provisions the cert and DNS automatically since
the zone lives in the same account.

#### Response headers (Content-Security-Policy)

`web/public/_headers` declares the response-header policy applied to
every static asset Cloudflare serves. The Content-Security-Policy is:

```
default-src 'self';
script-src  'self' https://static.cloudflareinsights.com;
style-src   'self';
font-src    'self';
img-src     'self' data:;
connect-src 'self' https://api.urbanistatlas.com https://cloudflareinsights.com;
frame-ancestors 'none';
base-uri    'self';
form-action 'self';
```

Notes on the directives:

- `script-src 'self' https://static.cloudflareinsights.com` — the SPA
  has no inline `<script>`; our own code goes through Vite's bundled
  module graph, and the one external script is the Cloudflare Web
  Analytics beacon (cookieless RUM). The beacon is **injected at the
  edge** by Cloudflare (Web Analytics automatic injection), not shipped
  in our HTML — but the browser enforces CSP against the injected tag,
  so the origin must be allow-listed here regardless.
- `style-src 'self'` — Vite's production build emits all CSS as
  external `<link rel="stylesheet">` assets. There are no inline
  `<style>` blocks in `dist/index.html`. Dev mode (HMR) uses inline
  styles, but `_headers` only applies on Cloudflare, so that's not
  a real constraint. If a future plugin starts inlining critical
  CSS, add `'unsafe-inline'` back with a one-line justification.
- `font-src 'self'` — all four families ship via
  `@fontsource-variable/*` and are bundled with the build.
- `connect-src` — outbound fetches go to the Atlas API plus
  `cloudflareinsights.com`, where the Web Analytics beacon POSTs its
  cookieless RUM data. (The pre-launch `qa-api.urbanistatlas.com`
  origin was removed from this list in the 2026-05-27 qa-teardown,
  alongside the qa cert, Workers custom domain, and CORS origin — see
  § QA hostname retirement.)
- `frame-ancestors 'none'` — the Atlas is never embedded in an
  iframe.

The same file also sets:

- `Strict-Transport-Security: max-age=31536000; includeSubDomains` —
  one-year HSTS with subdomain coverage. Cloudflare already terminates
  HTTPS at the edge; the header is defense in depth.
- `Referrer-Policy: strict-origin-when-cross-origin`
- `X-Content-Type-Options: nosniff`
- `Permissions-Policy: geolocation=(), camera=(), microphone=(), payment=(), usb=()`
  — turn off browser features the Atlas has no reason to touch.

When you add a new outbound origin (e.g. a future analytics endpoint),
edit the `connect-src` list in `web/public/_headers` and update this
section.

### 4. Smoke test

```sh
curl -fsS https://api.urbanistatlas.com/healthz
curl -fsS -H "X-Atlas-Client: <secret>" \
  'https://api.urbanistatlas.com/api/v1/lookup?postal_code=11217&country=US' \
  | jq '.local | length, .regional | length'
```

The `deploy-api` GHA job runs the same `/healthz` smoke after every
deploy with a 5×5s retry, so a transient cutover blip won't fail CI.

## Updating seed data

1. Edit `api/seed/orgs.toml` (or any region/postal file).
2. Open a PR. CI runs `just api-check`, which exercises the FileStore
   loader against the new bundle.
3. Merge to `main`. The `deploy-api` workflow builds a new image with
   the updated seed embedded and ships it to Fly.

There is no live data store, so a deploy *is* the data refresh.

### Regenerating ETL-generated seed (regions + postal codes)

`regions_us_msas.toml`, `regions_ca_cmas.toml`, `postal_codes_us.csv`,
and `postal_codes_ca.csv` are **generated** from upstream Census /
StatsCan data + HUD — never hand-edit them. To refresh after an ETL code
change or a vintage bump, on a maintainer machine with the sources staged
(see [`etl/SOURCES.md`](../etl/SOURCES.md), incl. the account-gated HUD
file for US postal):

```sh
just etl-regenerate US        # full: regions + postal (needs HUD staged)
just etl-regenerate CA        # full: regions + postal (needs the 155 MB FSA staged)
just seed-check               # verify committed region files == a fresh regen
cd api && go test ./internal/etl/... -run GoldenDeterminism   # lock generator logic
git add api/seed/ && git commit -m "seed: regenerate <CC>"
git push                      # → PR → merge → deploy-api ships the new image
```

CI enforces the first verify step: the `data` job runs `just seed-check`
on every PR and `deploy-api` won't run if it fails (#67). Sharp edges the
gate is built around:

- **Sources must match the `SOURCES.md` vintages** (download validates
  sha256). `etl regenerate` rewrites the *whole* file, so a stale local
  vintage smuggles an unrelated diff into the commit.
- **HUD must be staged for a full US regenerate**, or the CT legacy-county
  reconcile silently no-ops *and* reverts those ZIPs to bare `ct` — a
  `--target=regions` run avoids touching the postal CSV at all.
- **The diff should be country-scoped.** Churn outside the country you
  regenerated ⇒ stop and investigate (the signal-rich-diff property).

## Backups (SQLite submission queue)

[`.github/workflows/backup-sqlite.yml`](../.github/workflows/backup-sqlite.yml)
runs nightly at 09:17 UTC. It opens an `flyctl ssh console` into the
running Fly machine, pipes `sqlite3 /data/atlas.db .dump | gzip` to
stdout, and uploads the resulting `atlas-<date>.sql.gz` to the
`urbanist-atlas-backups` R2 bucket (30-day lifecycle retention set
out-of-band on the bucket).

Required Actions secrets:

| Secret | Source |
|---|---|
| `CF_ACCOUNT_ID` | Cloudflare dashboard → account ID (expanded into `https://<id>.r2.cloudflarestorage.com` by the workflow) |
| `R2_ACCESS_KEY_ID` | R2 API token (Object R/W on the backups bucket) |
| `R2_SECRET_ACCESS_KEY` | same R2 API token |
| `R2_BACKUP_BUCKET` | `urbanist-atlas-backups` |

These names match the secrets already provisioned for the prior
Postgres-era backup workflow, so the new workflow reuses them
without duplication.

`FLY_API_TOKEN_DEPLOY` (already configured for `ci.yml`) is reused
for the `flyctl ssh` step.

### Restore

```sh
# 1. Pull the snapshot locally and confirm the gzip stream is intact
#    (the nightly workflow already gunzip -t's before upload, but the
#    restore path is rare enough to double-check).
rclone copy r2:urbanist-atlas-backups/atlas-2026-05-28-0917.sql.gz .
gunzip -t atlas-2026-05-28-0917.sql.gz

# 2. Reconstruct a fresh DB.
gunzip -c atlas-2026-05-28-0917.sql.gz | sqlite3 /tmp/atlas.db.new

# 3. Stop the app machine so its open SQLite handle releases the file.
#    Skipping this and just mv'ing the file under a running binary
#    leaves the kernel holding the old inode open until the next
#    restart — submissions written in between will land in the OLD
#    file and disappear when the mv eventually takes effect.
flyctl machines list -a urbanist-atlas    # note the machine id
flyctl machines stop <machine-id> -a urbanist-atlas

# 4. Push the new DB onto the Fly volume.
flyctl ssh sftp shell -a urbanist-atlas
  put /tmp/atlas.db.new /data/atlas.db.new
  bye
flyctl ssh console -a urbanist-atlas -C \
  "sh -c 'mv /data/atlas.db /data/atlas.db.bak && mv /data/atlas.db.new /data/atlas.db'"

# 5. Restart and smoke-test.
flyctl machines start <machine-id> -a urbanist-atlas
curl -fsS https://api.urbanistatlas.com/readyz
```

### Re-running a failed promotion PR

If the GitHub PR worker logs a `promotion_error` (token expired,
GitHub outage, etc.) for an already-approved submission, re-run it
without needing to flip the row back to pending:

```sh
flyctl ssh console -a urbanist-atlas -C \
  "urbanist-atlas-server submissions retry-pr --id=<uuid>"
```

The retry uses the current `URBANIST_GITHUB_TOKEN` Fly secret and
overwrites `promotion_pr_url` / `promotion_error` on the row.

## Monitoring & incident response

Solo-dev posture: **reactive + manual.** There is no paging stack — the
job is to be able to *look* when something is reported, and to get a
nudge when the two things that fail silently fail. See the observability
design spec
([`superpowers/specs/2026-06-08-observability-design.md`](./superpowers/specs/2026-06-08-observability-design.md))
for the why.

### Where to look

- **Logs (primary debugging surface).** `flyctl logs -a urbanist-atlas`
  (or `just fly-logs`). Logs are structured slog (JSON in prod). Every
  request carries a request id; every error and admin action logs it as
  `rid`. When a user reports a problem, ask for the request ID shown in
  the error UI and grep for it:

  ```sh
  flyctl logs -a urbanist-atlas | grep '<rid>'
  ```

  Successful reads log at DEBUG (`lookup ok`, `region view`, `org view`,
  `region search`) — set `URBANIST_LOG_LEVEL=debug` (Fly secret) to see
  them; prod runs at `info`.

- **Metrics dashboard.** Fly's managed Grafana (Monitoring → Grafana, or
  `https://fly-metrics.net`) reads the managed Prometheus that scrapes the
  private `/metrics`. Import
  [`ops/grafana/dashboards/atlas-overview.json`](../ops/grafana/dashboards/atlas-overview.json);
  see [`ops/grafana/README.md`](../ops/grafana/README.md). Use it for
  latency/error trends, lookup hit/miss/**empty**, and the submission
  funnel.

- **Coverage gaps (editorial).** Which inputs return nothing:
  `GET /api/v1/admin/coverage-gaps` (bearer-gated). Capture is **off by
  default** — set `URBANIST_COVERAGE_SAMPLE_RATE` (e.g. `0.1`) to start
  sampling empty-result lookups/searches.

  ```sh
  curl -fsS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
    https://api.urbanistatlas.com/api/v1/admin/coverage-gaps | jq
  ```

- **Usage rollups (product).** Daily aggregate counts of content
  popularity and lookup outcomes, kept ~400 days on the SQLite volume —
  so they outlive the ~30-day Prometheus window and ride the nightly R2
  backup. Read them directly:

  ```sh
  curl -fsS -H "Authorization: Bearer $URBANIST_ADMIN_TOKEN" \
    'https://api.urbanistatlas.com/api/v1/admin/usage?from=2026-08-01&to=2026-08-31&kind=region_view&limit=25' | jq
  ```

  `from` and `to` are required (an unbounded range would scan the whole
  table). Tuned by `URBANIST_USAGE_FLUSH_INTERVAL` (default `1m`) and
  `URBANIST_USAGE_KEEP_DAYS` (default `400` — a year plus a month of
  margin, so year-over-year is always available). Counts buffer in RAM
  between flushes, so an ungraceful machine kill loses at most one
  interval.

  Per-slug popularity is recorded **here**, not in the logs. The
  `region view` / `org view` DEBUG slog lines are a debugging aid only —
  do **not** set `URBANIST_LOG_LEVEL=debug` in production to answer
  popularity questions. (Before this table existed, those lines were the
  only popularity signal, and because prod runs at `info` they were never
  actually emitted.)

### What pages you (GitHub Issues, no SaaS)

- **API down** — [`uptime.yml`](../.github/workflows/uptime.yml) probes
  `/healthz` from outside Fly every ~30 min and opens an issue if it's
  down for ~50s. (Fly's own machine health checks recycle unhealthy VMs
  but can't see DNS/TLS/edge problems — this catches those.)
- **Backup failure** — [`backup-sqlite.yml`](../.github/workflows/backup-sqlite.yml)
  opens an issue if the nightly SQLite→R2 snapshot fails.

Both reuse one open issue (comment, not spam) until you close it.

### What reports to you (monthly)

- **Usage digest** — [`usage-digest.yml`](../.github/workflows/usage-digest.yml)
  opens an issue on the 2nd of each month summarising audience
  (Cloudflare pageviews), content popularity, coverage gaps, and health,
  each with a month-over-month delta. Unlike the alarms above, **each
  month gets its own issue**: the digest is a durable record and the
  issue list is the archive.

  Sources degrade independently — a failed Cloudflare token shows one
  "unavailable" line rather than costing the whole digest. The job fails
  only if every source fails. Run it early with **Actions → Monthly usage
  digest → Run workflow**.

  Needs repo secrets `URBANIST_ADMIN_TOKEN`, `CF_ANALYTICS_TOKEN`,
  `CF_WEB_ANALYTICS_SITE_TAG`, and `FLY_ORG_SLUG` alongside the existing
  `CF_ACCOUNT_ID` and `FLY_API_TOKEN_DEPLOY`.

### Triage

| Symptom | First moves |
|---------|-------------|
| Uptime issue opened | `flyctl status -a urbanist-atlas` (machines up?), `flyctl logs`, check DNS/TLS for the apex. |
| `/readyz` 503 / `atlas_store_ping_failures_total` climbing | The SQLite volume is unreachable. `flyctl status`, `flyctl volumes list -a urbanist-atlas`; reads still work (they're in-memory), writes (submissions) don't. |
| 5xx spike on the dashboard | Grab a recent `rid` from `flyctl logs`, follow it; check a recent deploy — `flyctl releases -a urbanist-atlas`, roll back if needed (see §Rollback). |
| Backup issue opened | Not an outage. Re-run the workflow (Actions → SQLite nightly backup → Run workflow); if it keeps failing, check the Fly token / R2 creds (§GitHub Actions secrets) and take a manual snapshot (§Backups). |

## QA hostname retirement (historical, 2026-05-27)

`qa.urbanistatlas.com` + `qa-api.urbanistatlas.com` were the
pre-launch dogfooding origins while the Phase 1 stack was shaking
out. On 2026-05-27 the apex hostnames attached to the same Fly app
and Cloudflare Workers project; no rebuilds, no data migration —
purely a DNS + Workers custom-domain + Fly cert swap.

Cutover steps, kept here as a reference for any future hostname
move on this stack:

1. Add A/AAAA records for the new apex host pointing at
   `urbanist-atlas.fly.dev` (Cloudflare proxy off).
2. `flyctl certs add api.urbanistatlas.com -a urbanist-atlas` and
   wait for `Status: Issued`.
3. Attach the apex SPA host to the Workers project under custom
   domains.
4. Flip the Workers build env `VITE_API_BASE` to the new API host,
   trigger a rebuild.
5. Prepend the new apex SPA host to `URBANIST_CORS_ORIGINS` in
   `fly.toml` and deploy; retain the old origin for a verification
   window.
6. Smoke against the new hosts (`just smoke`), watch logs for a
   day, then in a single follow-up commit: drop the old origin
   from `URBANIST_CORS_ORIGINS`, drop the old API host from the
   `connect-src` list in `web/public/_headers`, remove the old Fly
   cert, the old Workers custom domain, and the old DNS records.
