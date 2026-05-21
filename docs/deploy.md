# Deploy runbook

Operational guide for taking `urbanist-atlas` from a clean Fly +
Cloudflare account to a working QA deployment. The design that
motivates this runbook is at
[`docs/superpowers/specs/2026-05-21-fly-redeploy-design.md`](./superpowers/specs/2026-05-21-fly-redeploy-design.md);
this file is the executable playbook.

> **Status: not yet executed.** Slice #20.6 (this slice) merges all
> code/config/docs to `main`, but no Fly apps, Pages project, or DNS
> records have been provisioned yet. The steps below run end-to-end
> against a clean account.

The launch chunk is now consolidated into a single slice (#20.6) that
absorbs the original #20/#21-DNS/#22/#25:

| Slice | What it ships | Where it's documented |
|---|---|---|
| #19 | (Retired — `Dockerfile` + `fly.toml` ship again on the Heroku → Fly pivot) | — |
| #19.5 | Hosting cost spike + decision | `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` |
| #20 | (Retired by #20.6 — Heroku deliverables superseded) | — |
| #20.6 | Fly API + sibling Postgres + R2 backups + DNS + smoke + CORS audit | This file + the Fly redeploy design doc |
| #21 | Cloudflare Pages + DNS + TLS — Pages code unchanged; DNS retargeted | **§ Cloudflare Pages + DNS** below |
| #23 | `X-Atlas-Client` shared-secret gate | In tree; this runbook references the secret in § Secrets |

## Hosting topology

| Component | Resource | Initial hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` (Virginia, US East), shared-cpu-1x / 256 MB | `qa-api.urbanistatlas.com` |
| DB | Fly app `urbanist-atlas-db`, region `iad`, `postgres:17-alpine` + 1 GB volume | Private: `urbanist-atlas-db.internal:5432` |
| Web | Cloudflare Pages project `urbanist-atlas`, prod branch `main` | `qa.urbanistatlas.com` |
| Web previews | `<branch>.urbanist-atlas.pages.dev` | Auto-provisioned per non-`main` branch |
| Backups | GHA cron → R2 bucket `urbanist-atlas-backups` (30-day retention) | — |

None of the resources carry a `-qa` suffix. They are the *same*
resources that will host production; only the *hostnames* are
environment-flavored during QA. When prod launches, prod hostnames
attach to the same apps/project and QA hostnames retire — no rebuilds,
no data migration.

## Prerequisites

- [flyctl](https://fly.io/docs/flyctl/install/) installed and
  authenticated (`flyctl auth signup` or `flyctl auth login`).
  Credit card on the Fly account (Fly machines are billed per second).
- Cloudflare account with access to the `urbanistatlas.com` zone and
  with R2 enabled (free tier is sufficient).
- This repo cloned locally; `Dockerfile` and `fly.toml` live at the
  repo root, `infra/postgres/fly.toml` is the sibling Postgres config.
- (Optional but recommended) `just` available so the `fly-*` recipes
  in the root [`justfile`](../justfile) work — they are thin
  wrappers around `flyctl` so the verbs are discoverable via
  `just --list`.
- (For step 7 — the backup workflow) a Fly personal access token
  generated via `flyctl auth token`, and a Cloudflare R2 access key
  pair from the dashboard.

## Fly: API + sibling Postgres

The DB app comes up first; the API app's `DATABASE_URL` secret is
constructed from the DB's Postgres password, so we need that password
generated before the API app deploys.

### 1. Provision the DB app

```sh
flyctl launch \
    --no-deploy \
    --copy-config \
    --name urbanist-atlas-db \
    --region iad \
    --config infra/postgres/fly.toml
```

`flyctl launch` is interactive — accept the `infra/postgres/fly.toml`
config when prompted, say **no** to "Would you like to set up Postgres
now?" (we're *being* the Postgres), **no** to Redis, **no** to deploy
yet.

Create the 1 GB volume the Postgres data dir mounts to:

```sh
flyctl volumes create pgdata \
    -a urbanist-atlas-db \
    -r iad \
    -s 1 \
    --yes
```

Generate the Postgres password and set it as a Fly secret. **Capture
the value locally now** — Fly secrets are write-only; you can't read
this back later, and the API app's `DATABASE_URL` needs it in step 3.

```sh
PG_PASSWORD="$(openssl rand -hex 32)"
echo "PG_PASSWORD=$PG_PASSWORD"     # → paste into a password manager
flyctl secrets set POSTGRES_PASSWORD="$PG_PASSWORD" -a urbanist-atlas-db
```

Deploy the DB app:

```sh
flyctl deploy --config infra/postgres/fly.toml
# or: just fly-deploy-db
```

Verify Postgres came up:

```sh
flyctl status -a urbanist-atlas-db
```

**Gotcha:** the sibling Postgres app has no `[[services]]` /
`[http_service]` block (it's internal-only), so Fly's "rolling deploy
strategy" has nothing to health-check and exits after clearing the
lease — leaving the machine in whatever state it was in. Because
`flyctl launch --no-deploy` creates the machine in `stopped` state,
the first deploy may leave it stopped. If `flyctl status` shows
`STATE=stopped`, start it explicitly:

```sh
flyctl machines start -a urbanist-atlas-db
flyctl status -a urbanist-atlas-db     # confirm STATE=started
```

After this first-boot bring-up, the `[restart] policy = "always"`
block in `infra/postgres/fly.toml` keeps Fly auto-restarting any
crashed machine indefinitely (Fly's default of `on-failure
max_retries=10` would otherwise strand the DB stopped after a bug
storm). Manually `flyctl machine stop` still works for maintenance —
the `always` policy doesn't override an explicit stop.

Sanity-check the database:

```sh
flyctl ssh console -a urbanist-atlas-db -C \
    "psql -U urbanist -d urbanist_atlas -c 'select version();'"
```

### 2. Provision the API app

```sh
flyctl launch \
    --no-deploy \
    --copy-config \
    --name urbanist-atlas \
    --region iad
```

Same interactive prompts as step 1 — accept the root `fly.toml`,
**no** to Postgres (we already have one), **no** to Redis, **no** to
deploy.

### 3. Set API config + secrets

Non-secret config is already in `fly.toml`'s `[env]` block
(`URBANIST_LOG_FORMAT`, `URBANIST_STORE`, `URBANIST_SEED_DIR`,
`URBANIST_PORT`, `URBANIST_CORS_ORIGINS`). Three secrets go via
`flyctl secrets set` — and one of them (`URBANIST_CLIENT_SECRET`)
needs to be captured locally for the Cloudflare Pages dashboard in §
Cloudflare Pages + DNS, because Fly secrets are write-only after set
(`flyctl secrets list` only shows digests). So pre-generate the
captured values, echo the one you'll paste later, then set everything
in one call:

```sh
ADMIN_TOKEN="$(openssl rand -hex 32)"
CLIENT_SECRET="$(openssl rand -hex 32)"
echo "CLIENT_SECRET=$CLIENT_SECRET"   # → paste into password manager / Pages dashboard NOW

flyctl secrets set \
    DATABASE_URL="postgres://urbanist:${PG_PASSWORD}@urbanist-atlas-db.internal:5432/urbanist_atlas?sslmode=disable" \
    URBANIST_ADMIN_TOKEN="$ADMIN_TOKEN" \
    URBANIST_CLIENT_SECRET="$CLIENT_SECRET" \
    -a urbanist-atlas
```

`${PG_PASSWORD}` is the value captured in § Fly step 1 when you set
`POSTGRES_PASSWORD` on the DB app. If the shell session that captured
it has closed, re-`export` it from your password manager before
running this block.

`URBANIST_ADMIN_TOKEN` is pre-staged for Phase 2 (no-op until admin
endpoints land). `URBANIST_CLIENT_SECRET` is read by the slice-#23
`X-Atlas-Client` middleware and must match the
`VITE_API_CLIENT_SECRET` value you set in the Cloudflare Pages
dashboard.

> **Fish users:** translate `VAR=value` → `set VAR value` and
> `"$(cmd)"` → `(cmd)` (fish doesn't do command substitution inside
> double quotes); see the API smoke section's example for the
> equivalent fish form.

### 4. First deploy

```sh
flyctl deploy
# or: just fly-deploy
```

The deploy:

1. Builds the multi-stage Dockerfile from the repo root, producing
   the `urbanist-atlas-server` binary at `/usr/local/bin/` and the
   seed dir at `/app/seed`.
2. Pushes the image to Fly's registry.
3. Runs `release_command = "migrate up"` in a one-off machine against
   the `DATABASE_URL` secret (the Dockerfile's
   `ENTRYPOINT ["urbanist-atlas-server"]` is prepended automatically).
   Migration failure blocks the deploy.
4. Starts a new app machine; old machine drains after the health-check
   on the new one passes.

### 5. Seed the database

```sh
flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server loaddata"
# or: just fly-loaddata
```

`loaddata` runs the regions → postal-codes → orgs chain for every
bundled country in dependency order. Each country has its own
multi-tier region file list (state/province → multistate → MSAs/CMAs
→ curated leaves) declared in `api/internal/loaddata/loaddata.go`'s
`countries` table. The loaders are upsert-based, so re-running is
safe — counts won't change unless the seed files do. Adding a new
country is documented in `docs/region-graph.md` § "Adding a new
country"; the integration test (`TestPipeline_LoaddataLoadAll`)
picks the new country up automatically via `loaddata.Countries()`.

**Expected load size for the current bundle** (slice #7.5 onward):

| Country | Region rows | Postal-code rows |
|---|---|---|
| US | 52 states + 3 multistate + 393 MSAs + 19 leaves = **467** | **33,774** ZCTAs |
| CA | 13 provinces + 41 CMAs + 5 leaves = **59** | **1,643** FSAs |
| PT | 22 regions | 7 postal codes (validation fixture) |
| Orgs | — | 134 organizations |

The 33k US postal-code load is handled by the batched UNNEST upsert
path in `internal/loadpostal` and completes in ~3s on the Fly DB (vs.
~27min if it were per-row upserts).

If `loaddata` fails partway through, the preceding countries' rows
stay committed — the loaders are upsert-based and idempotent, so fix
the offending file and re-run. No reset needed.

`URBANIST_SEED_DIR=/app/seed` was set in `fly.toml`'s `[env]` block;
the Dockerfile COPYs `api/seed/` to `/app/seed`. If `flyctl ssh
console` ever can't find the seed files, `flyctl ssh console -a
urbanist-atlas -C "ls -la /app"` confirms the slug layout.

### 6. Smoke-test the API

The auto-generated Fly hostname is live as soon as the deploy
completes — useful before DNS lands:

```sh
FLY_URL="https://urbanist-atlas.fly.dev"

# /healthz is bypass-listed and works without a header.
curl -i "$FLY_URL/healthz"
# → 200

# /api/v1/* requires X-Atlas-Client.
curl -sS "$FLY_URL/api/v1/lookup?postal_code=10001&country=US"
# → 401 problem+json

curl -sS -H "X-Atlas-Client: $CLIENT_SECRET" \
    "$FLY_URL/api/v1/lookup?postal_code=10001&country=US"
# → 200 with X-Data-License: ODbL-1.0 and X-Data-Attribution headers
```

### 7. Set up the backup workflow

The nightly backup runs in GitHub Actions at
`.github/workflows/backup.yml`. Add the required secrets to the repo
(**Settings → Secrets and variables → Actions**):

| Secret | Value |
|---|---|
| `FLY_API_TOKEN` | `flyctl auth token` — Fly personal access token |
| `CF_ACCOUNT_ID` | Cloudflare account id (from any zone overview page in the dashboard) |
| `R2_ACCESS_KEY_ID` | R2 access key (from R2 dashboard → Manage API tokens) |
| `R2_SECRET_ACCESS_KEY` | Paired secret |

Create the R2 bucket and the 30-day retention rule:

1. Cloudflare dashboard → **R2 → Create bucket** → name
   `urbanist-atlas-backups`, location automatic.
2. Bucket → **Settings → Object lifecycle rules → Add rule**:
   name `expire-30d`, scope `Apply to all objects`, action
   `Delete objects` after `30 days`.

Then trigger the workflow manually to verify end-to-end (**Actions →
backup-postgres → Run workflow**). It should succeed in <60 s and
the R2 bucket should contain a single dated `.sql.gz`. Download it,
`gunzip -t` to verify integrity, optionally `pg_restore --list` to
inspect.

The cron runs nightly at 02:00 America/New_York from then on.

## Cloudflare Pages + DNS

The SPA deploys to Cloudflare Pages from `web/`. Pages reads
`web/public/_redirects` to rewrite every non-asset path to
`/index.html`, which is the SPA fallback that makes direct navigation
to `/about`, `/browse`, `/m/:slug`, `/r/:postalCode` work.

The order in this section matters: Fly issues the Let's Encrypt cert
for `qa-api.urbanistatlas.com` automatically, but only once the
CNAME is resolvable — so the Fly side adds the cert request first
(§1), then Cloudflare DNS (§3), then the Pages side (§4–§5).

### 1. Attach the API hostname to Fly

```sh
flyctl certs add qa-api.urbanistatlas.com -a urbanist-atlas
flyctl certs show qa-api.urbanistatlas.com -a urbanist-atlas
```

`flyctl certs show` prints the DNS target — the same `urbanist-atlas.fly.dev`
hostname Fly issued for the app. It also prints the ACME challenge
status (Let's Encrypt issuance is automated once DNS resolves).

### 2. Create the Pages project (dashboard)

Cloudflare dashboard → **Pages → Create → Connect to Git**:

- **Repo:** `mjrossi/urbanist-atlas`
- **Production branch:** `main`
- **Build command:** `cd web && npm ci && npm run build`
- **Build output directory:** `web/dist`
- **Root directory:** repo root (leave blank)
- **Project name:** `urbanist-atlas` *(this becomes the `*.pages.dev`
  subdomain — substitute consistently for `<pages-project>` in §3 if
  you pick a different name)*

Environment variables — **apply to both Production AND Preview**:

| Name | Value |
|---|---|
| `VITE_API_BASE` | `https://qa-api.urbanistatlas.com` |
| `VITE_API_CLIENT_SECRET` | *(same value as the Fly `URBANIST_CLIENT_SECRET` set in § Fly step 3 above)* |
| `NODE_VERSION` | `22` *(Pages honors a major-version pin only; minor/patch may drift from `mise.toml`'s `22.22.3` pin, which is acceptable for a static SPA build)* |

`VITE_API_CLIENT_SECRET` is bundled into the static build, so the SPA
can include it in the `X-Atlas-Client` header on every API request.
Phase 1 lockdown is a deterrent against casual scrapers, not a real
secret — anyone who downloads the bundle can read it. The value still
needs to match `URBANIST_CLIENT_SECRET` exactly or the SPA 401s.

The first push to `main` after this configures Pages triggers an
automatic deploy. Pages will say "this branch has not been deployed"
until then.

### 3. DNS records (Cloudflare DNS, `urbanistatlas.com` zone)

| Name | Type | Value | Proxy |
|---|---|---|---|
| `qa` | CNAME | `<pages-project>.pages.dev` | **ON** *(Pages requires its CDN proxy)* |
| `qa-api` | CNAME | `urbanist-atlas.fly.dev` | **OFF** *(Fly's edge terminates TLS directly)* |

`qa` is the SPA hostname. `qa-api` is the API hostname; the proxy
must be OFF because Fly is the TLS terminator. A proxied `qa-api`
would cause cert validation failures and break Fly's automated
Let's Encrypt issuance.

### 4. Custom domain for the SPA

In the Pages project: **Custom domains → Set up a custom domain →
`qa.urbanistatlas.com`**. Pages auto-detects the CNAME added in §3 and
issues its own cert through Cloudflare. No manual cert step.

### 5. Wait for the Fly cert

```sh
# Re-run until Status = Verified, Configured = true.
flyctl certs show qa-api.urbanistatlas.com -a urbanist-atlas
```

Propagation is usually under 5 minutes once the `qa-api` CNAME in §3
goes live; if it stalls beyond 15 minutes, verify (a) the `qa-api`
CNAME resolves to `urbanist-atlas.fly.dev` exactly
(`dig qa-api.urbanistatlas.com CNAME +short`), (b) the Cloudflare
proxy on that record is OFF, and (c) no DNSSEC or CAA record on the
zone is blocking Let's Encrypt issuance.

### 6. Smoke test the deploy

```sh
# Web SPA — direct navigation should work for every client route.
curl -I https://qa.urbanistatlas.com
curl -I https://qa.urbanistatlas.com/browse
curl -I https://qa.urbanistatlas.com/about
# All → 200, served by Pages (look for the cf-ray header).

# Confirm _redirects actually rewrites to index.html (not a Pages
# default-404 page, which would also 200): the response body for a
# non-root path must include the SPA root mount.
curl -s https://qa.urbanistatlas.com/browse | grep -q '<div id="root"' \
    && echo "SPA shell served"

# Confirm static assets are served as files, NOT rewritten to
# index.html.
ASSET=$(curl -s https://qa.urbanistatlas.com \
    | grep -oE '/assets/[A-Za-z0-9._-]+\.js' | head -1)
curl -I "https://qa.urbanistatlas.com${ASSET}"
# → 200 with content-type: application/javascript (NOT text/html).

# Confirm the API CORS allowlist contains the SPA origin.
flyctl secrets list -a urbanist-atlas | grep URBANIST_CORS_ORIGINS \
    || flyctl config show -a urbanist-atlas | grep URBANIST_CORS_ORIGINS
# → must contain https://qa.urbanistatlas.com and *.pages.dev.

# API direct — no cf-ray header, Fly is the terminator.
curl -I https://qa-api.urbanistatlas.com/healthz
# → 200, no cf-ray.

# End-to-end smoke via the just recipe (covers slice #25):
URBANIST_CLIENT_SECRET="$CLIENT_SECRET" just smoke

# Browser end-to-end: load qa.urbanistatlas.com, run a known seed ZIP
# (e.g. 10001 US → /r/10001). DevTools should show:
#   Request:  X-Atlas-Client: <matches VITE_API_CLIENT_SECRET>
#   Response: X-Data-License: ODbL-1.0
#             X-Data-Attribution: …urbanistatlas.com/about/data…
```

A throwaway branch push exercises the Pages preview path: `git push
origin throwaway-branch` produces
`https://throwaway-branch.<pages-project>.pages.dev`, which Pages
auto-deploys with the **Preview** env vars (so it still calls the QA
API). The `*.pages.dev` entry in `URBANIST_CORS_ORIGINS` keeps these
preview URLs working.

## Ongoing operations

Every recipe below has a `just fly-*` (or `just db-*`) wrapper at the
repo root so the verbs are discoverable via `just --list`.

| Task | Command | Wrapper |
|---|---|---|
| Build + deploy current branch | `flyctl deploy -a urbanist-atlas` | `just fly-deploy` |
| Deploy sibling Postgres (rare) | `flyctl deploy -a urbanist-atlas-db -c infra/postgres/fly.toml` | `just fly-deploy-db` |
| Tail live API logs | `flyctl logs -a urbanist-atlas` | `just fly-logs` |
| Tail live DB logs | `flyctl logs -a urbanist-atlas-db` | `just fly-logs-db` |
| List config (names + digests) | `flyctl secrets list -a urbanist-atlas` | `just fly-secrets` |
| Interactive shell on the API machine | `flyctl ssh console -a urbanist-atlas` | `just fly-ssh` |
| Re-seed the database | `flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server loaddata"` | `just fly-loaddata` |
| Ad-hoc local backup | `just db-backup` (writes `./urbanist-atlas-YYYY-MM-DD.sql.gz`) | `just db-backup` |
| Restore a dump (DESTRUCTIVE) | `gunzip -c <file>.sql.gz \| flyctl ssh console -a urbanist-atlas-db -C "psql ..."` | `just db-restore <file>` |
| psql against the DB | `flyctl ssh console -a urbanist-atlas-db -C "psql -U urbanist urbanist_atlas"` | — |

A redeploy after a code change is `flyctl deploy` (or
`just fly-deploy`) from any branch; the `release_command` in
`fly.toml` re-runs migrations.

## Secrets

### Rotation procedure

Both app-level secrets have a partner on the web side. Rotating one
means updating the other in lockstep, or the SPA will start 401-ing.

**`URBANIST_CLIENT_SECRET` ↔ `VITE_API_CLIENT_SECRET` (Pages):**

```sh
NEW=$(openssl rand -hex 32)
flyctl secrets set URBANIST_CLIENT_SECRET="$NEW" -a urbanist-atlas
# Then in the Cloudflare Pages dashboard:
#   Pages → urbanist-atlas → Settings → Environment variables
#   Production + Preview: set VITE_API_CLIENT_SECRET = <NEW>
#   Trigger a redeploy (Deployments → Retry on latest, or push a commit)
echo "$NEW"   # paste into the Pages dashboard
```

During the rotation window (Fly secret updated, Pages still
serving the old build): in-flight browser sessions will 401 until
Pages finishes rebuilding (~1 min). Schedule rotations during low
traffic.

**`URBANIST_ADMIN_TOKEN`** is bearer-token-style and is read per
request, so rotation is instant from the API's perspective:

```sh
flyctl secrets set URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" -a urbanist-atlas
```

Hand the new value to whichever tool/script consumes the admin
endpoints.

**`DATABASE_URL`** is set once at provisioning (constructed from the
DB app's `POSTGRES_PASSWORD`). To rotate the Postgres password:

```sh
NEW_PG=$(openssl rand -hex 32)
flyctl ssh console -a urbanist-atlas-db -C \
    "psql -U urbanist -d urbanist_atlas -c \"ALTER USER urbanist WITH PASSWORD '$NEW_PG';\""
flyctl secrets set POSTGRES_PASSWORD="$NEW_PG" -a urbanist-atlas-db
flyctl secrets set \
    DATABASE_URL="postgres://urbanist:${NEW_PG}@urbanist-atlas-db.internal:5432/urbanist_atlas?sslmode=disable" \
    -a urbanist-atlas
```

Fly restarts the affected machines on secret changes. The API picks
up the new `DATABASE_URL` on the next request after the restart.

### What's *not* a secret

The values in `fly.toml`'s `[env]` block
(`URBANIST_LOG_FORMAT`, `URBANIST_STORE`, `URBANIST_SEED_DIR`,
`URBANIST_PORT`, `URBANIST_CORS_ORIGINS`) are non-sensitive; they
live in the file rather than the repo only because Fly's config
mechanism keeps them with the deploy. Treat changes to them as
code-review concerns even though they're not technically secret.

## QA → prod transition (future)

When the maintainer is ready to launch prod:

1. **Cloudflare Pages**: add `urbanistatlas.com` as a *second*
   custom domain to the same Pages project. Add the apex + `www`
   CNAMEs.
2. **Fly**: `flyctl certs add api.urbanistatlas.com -a urbanist-atlas`.
   Add a CNAME `api` → `urbanist-atlas.fly.dev` (Cloudflare proxy
   **off**).
3. **CORS**: edit `fly.toml`'s `[env].URBANIST_CORS_ORIGINS` to
   include `https://urbanistatlas.com`, then `flyctl deploy` (or set
   via secret if treating CORS as deploy-coupled rather than
   code-coupled).
4. **Pages env**: switch `VITE_API_BASE` from
   `https://qa-api.urbanistatlas.com` to
   `https://api.urbanistatlas.com`. Trigger a new Pages deploy.
5. **Verify** the prod hostnames serve correctly, then retire the
   QA hostnames: remove `qa` + `qa-api` DNS records, drop the QA
   custom domain from Fly
   (`flyctl certs remove qa-api.urbanistatlas.com`), remove the
   QA custom domain from the Pages project, drop the
   `https://qa.urbanistatlas.com` entry from
   `URBANIST_CORS_ORIGINS`.
6. **Going forward**: rely on Pages' automatic `*.pages.dev`
   previews for ephemeral web environments.

No code change is required for this transition — it's all config
and DNS. The Fly apps, the Pages project, the Postgres volume, and
every commit hash are reused as-is.

## Troubleshooting

**`flyctl deploy` fails during the release_command step.**
The `release_command = "migrate up"` runs in a one-off machine before
the new app machine takes traffic; Fly prepends the Dockerfile
`ENTRYPOINT ["urbanist-atlas-server"]` automatically. Check
`flyctl logs -a urbanist-atlas` for the migration error (usually a
Goose SQL syntax issue or a missing column from a hand-applied prior
migration). Migrations are embedded in the binary
(`api/migrations/embed.go`), so the deployed code and the migration
set are always in lockstep — no version-skew failure mode. **Gotcha:**
if you ever rewrite the release_command, do **not** repeat the binary
name (`urbanist-atlas-server migrate up`) — Fly appends to ENTRYPOINT,
not replaces it, so a doubled name shows up as `urbanist-atlas-server
urbanist-atlas-server ...` and the binary sees the second
`urbanist-atlas-server` as a subcommand and exits non-zero.

**`/healthz` returns 200 but `/api/v1/lookup` returns 401.**
Expected when calling without the `X-Atlas-Client` header. Either
pass the header (see § Smoke-test the API) or check that the SPA's
`VITE_API_CLIENT_SECRET` matches the Fly `URBANIST_CLIENT_SECRET`
value.

**`/api/v1/lookup` returns CORS error in a browser.**
The calling origin isn't in `URBANIST_CORS_ORIGINS`. For local dev
against the deployed API, add `http://localhost:5173` to the
config and redeploy, or run against `just api-run` locally.
`*.pages.dev` covers every Pages preview hostname.

**`flyctl ssh console -a urbanist-atlas -C "urbanist-atlas-server loaddata"` fails.**
Either the seed files aren't in the image (verify by
`flyctl ssh console -a urbanist-atlas -C "ls -la /app/seed"`) or
`DATABASE_URL` isn't set (verify by `flyctl secrets list -a urbanist-atlas`).
Re-deploy if seed files are missing — they ship in the image because
the Dockerfile COPYs `api/seed/` to `/app/seed`. `URBANIST_SEED_DIR=/app/seed`
is set in `fly.toml`; if the path ever shifts, adjust both.

**API can't connect to the DB.**
Verify (a) `urbanist-atlas-db` is running (`flyctl status -a
urbanist-atlas-db`), (b) the API's `DATABASE_URL` references
`urbanist-atlas-db.internal:5432` (Fly internal 6PN), (c) the password
component matches the DB app's `POSTGRES_PASSWORD` secret. A typo in
the constructed `DATABASE_URL` is the most common failure here.

**DB machine boot-loops with `initdb: error: directory ... exists but is not empty`.**
Fly's ext4 volumes auto-include a `lost+found` directory at the mount
root, which `initdb` refuses to write into. The fix is in
`infra/postgres/fly.toml`'s `[env]`: `PGDATA =
"/var/lib/postgresql/data/pgdata"` (subdirectory of the mount, not
the mount root). If you ever see this error after a config change,
confirm `PGDATA` is still pointed at the subdir.

**DB machine boot-loops with `mkdir: can't create directory '/var/lib/postgresql/data/pgdata': Permission denied`.**
PGDATA is correctly pointed at a subdirectory, but Fly mounts volumes
as `root:root mode 0755` and the upstream `postgres:17-alpine`
entrypoint demotes to the `postgres` user before doing the mkdir.
The fix is the thin wrapper at `infra/postgres/Dockerfile` +
`infra/postgres/entrypoint-fly.sh`, which runs as root, pre-creates
the PGDATA subdir + chowns the mount root to `postgres`, then exec's
`docker-entrypoint.sh`. If you ever see this error, confirm
`infra/postgres/fly.toml` has `[build] dockerfile = "Dockerfile"`
(not `image = "postgres:17-alpine"` — that bypasses the wrapper).

**Need to inspect production data.**
`flyctl ssh console -a urbanist-atlas-db -C "psql -U urbanist
urbanist_atlas"` opens a `psql` session against the DB.
`flyctl ssh console -a urbanist-atlas` opens a shell on the API
machine. Both require flyctl auth.

**Backup workflow fails.**
Check the Actions log. Most common causes: missing or wrong
`FLY_API_TOKEN` (regenerate via `flyctl auth token`), missing R2
credentials, or a typo in `CF_ACCOUNT_ID`. The workflow has a
sanity check that refuses to upload an empty dump, so a silent
`pg_dump` failure won't quietly clobber a real backup.

**Reverting to Heroku (if Fly turns out not to fit).**
The Heroku deploy was never executed; no live state to migrate.
Reverting is a `git revert` of the slice-#20.6 commit — Heroku-shaped
files (`Procfile`, `heroku-*` justfile recipes, the original
Heroku-targeted `docs/deploy.md`) are restored from git history.
See § Reversibility in
[`docs/superpowers/specs/2026-05-21-fly-redeploy-design.md`](./superpowers/specs/2026-05-21-fly-redeploy-design.md).
