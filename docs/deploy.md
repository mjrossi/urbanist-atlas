# Deploy runbook

Operational guide for `urbanist-atlas`. The original Postgres-backed
design lives at
[`docs/superpowers/specs/2026-05-21-fly-deploy-design.md`](./superpowers/specs/2026-05-21-fly-deploy-design.md);
the runtime has since moved to a stateless, file-backed shape — this
file is the current playbook.

> **Status: live since 2026-05-21.** Phase 1 QA stack (Fly API +
> Cloudflare Workers + DNS + shared-secret gate) is up on
> `qa-api.urbanistatlas.com` + `qa.urbanistatlas.com`. The sibling
> Postgres app and its nightly backup workflow were retired with the
> file-store cutover; reads come straight from the `api/seed/`
> bundle baked into the API image.

## Hosting topology

| Component | Resource | Initial hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` (Virginia, US East), shared-cpu-1x / 256 MB. Read path is stateless: `api/seed/` is baked into the image and loaded into an in-memory FileStore at boot. Writes (submissions only) land in a SQLite DB at `/data/atlas.db` on the `atlas_data` Fly volume (1 GiB, ~$0.15/mo). | `qa-api.urbanistatlas.com` |
| Web | Cloudflare Workers + Pages project `urbanist-atlas` (Static Assets), prod branch `main`, configured by `wrangler.jsonc` at repo root | `qa.urbanistatlas.com` |
| Web previews | `<version>-urbanist-atlas.<account>.workers.dev` | Auto-provisioned per non-`main` deploy via `wrangler versions upload` |

None of the resources carry a `-qa` suffix. When prod launches, prod
hostnames attach to the same apps/project and QA hostnames retire —
no rebuilds, no data migration.

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

Create the CNAME records in Cloudflare's `urbanistatlas.com` zone:

| Host | Target |
|---|---|
| `qa-api.urbanistatlas.com` | `urbanist-atlas.fly.dev` |

Then issue a Fly cert:

```sh
flyctl certs create qa-api.urbanistatlas.com -a urbanist-atlas
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
| `VITE_API_BASE` | `https://qa-api.urbanistatlas.com` |
| `VITE_API_CLIENT_SECRET` | the same value set on Fly as `URBANIST_CLIENT_SECRET` |

CNAME `qa.urbanistatlas.com` → the Workers project hostname; add it
in the Workers project's custom-domain settings.

#### Response headers (Content-Security-Policy)

`web/public/_headers` declares the response-header policy applied to
every static asset Cloudflare serves. The Content-Security-Policy is:

```
default-src 'self';
script-src  'self';
style-src   'self';
font-src    'self';
img-src     'self' data:;
connect-src 'self' https://api.urbanistatlas.com https://qa-api.urbanistatlas.com;
frame-ancestors 'none';
base-uri    'self';
form-action 'self';
```

Notes on the directives:

- `script-src 'self'` — the SPA has no inline `<script>`; everything
  goes through Vite's bundled module graph.
- `style-src 'self'` — Vite's production build emits all CSS as
  external `<link rel="stylesheet">` assets. There are no inline
  `<style>` blocks in `dist/index.html`. Dev mode (HMR) uses inline
  styles, but `_headers` only applies on Cloudflare, so that's not
  a real constraint. If a future plugin starts inlining critical
  CSS, add `'unsafe-inline'` back with a one-line justification.
- `font-src 'self'` — all four families ship via
  `@fontsource-variable/*` and are bundled with the build.
- `connect-src` — the only outbound fetches are to the Atlas API.
  The QA host stays in the list until Phase 2 cutover collapses
  qa/prod onto a single origin; drop the QA entry then.
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
curl -fsS https://qa-api.urbanistatlas.com/healthz
curl -fsS -H "X-Atlas-Client: <secret>" \
  'https://qa-api.urbanistatlas.com/api/v1/lookup?postal_code=11217&country=US' \
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
curl -fsS https://qa-api.urbanistatlas.com/readyz
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
