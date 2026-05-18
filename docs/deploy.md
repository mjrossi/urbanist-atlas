# Deploy runbook

Operational guide for taking `urbanist-atlas` from a clean Fly + Cloudflare
account to a working QA deployment. The design that motivates this
runbook is at
[`docs/superpowers/specs/2026-05-18-qa-deploy-design.md`](./superpowers/specs/2026-05-18-qa-deploy-design.md);
this file is the executable playbook.

The launch chunk is four slices:

| Slice | What it ships | Where it's documented |
|---|---|---|
| #19 | `Dockerfile` + `fly.toml` | This repo (in tree) |
| #20 | Fly app + Managed Postgres + first deploy + seed | **§ Fly: API + database** below |
| #21 | Cloudflare Pages + DNS + TLS for `qa.*` hostnames | § Cloudflare Pages + DNS below (added by slice #21) |
| #23 | `X-Atlas-Client` shared-secret gate | In tree; this runbook references the secret in § Secrets |

## Hosting topology

| Component | Resource | Initial hostname |
|---|---|---|
| API | Fly app `urbanist-atlas`, region `iad` | `qa-api.urbanistatlas.com` |
| DB | Fly Managed Postgres `urbanist-atlas-db` | Private to Fly app |
| Web | Cloudflare Pages project `urbanist-atlas`, prod branch `main` | `qa.urbanistatlas.com` |
| Web previews | `<branch>.urbanist-atlas.pages.dev` | Auto-provisioned per non-`main` branch |

None of the resources carry an `-qa` suffix. They are the *same* resources
that will host production; only the *hostnames* are environment-flavored
during QA. When prod launches, prod hostnames attach to the same app/
project and QA hostnames retire — no rebuilds, no data migration.

## Prerequisites

- `flyctl` installed (`mise install` provisions it via the
  [`aqua:superfly/flyctl`](../mise.toml) entry) and authenticated
  (`flyctl auth login`).
- Cloudflare account with access to the `urbanistatlas.com` zone.
- This repo cloned locally; `fly.toml` lives at the repo root.
- (Optional but recommended) `just` available so the `fly-*` recipes in
  the root [`justfile`](../justfile) work — they are thin wrappers around
  `flyctl` so the verbs are discoverable via `just --list`.

## Fly: API + database

### 1. Adopt the committed `fly.toml`

```sh
flyctl launch --no-deploy --name urbanist-atlas --region iad --copy-config
```

`--copy-config` adopts the committed `fly.toml` as-is, so the
`URBANIST_CORS_ORIGINS` allowlist, the `/healthz` check, and the
`release_command` are wired without editing anything.

### 2. Provision Managed Postgres and attach it

```sh
flyctl mpg create --name urbanist-atlas-db --region iad
flyctl mpg attach urbanist-atlas-db --app urbanist-atlas
```

`mpg attach` prints the connection string to stdout *once* and sets
`DATABASE_URL` on the Fly app. Copy the printed `DATABASE_URL=postgres://...`
line — the next step pastes it back under the project's env name
(`URBANIST_DB_URL`), since the binary reads that, not `DATABASE_URL`.

Why paste it manually instead of reading it via `flyctl ssh console -C
'printenv DATABASE_URL'`? At this point no machine is running yet
(launch was `--no-deploy`), so there's nothing to SSH into. The
connection string is only revealed on `attach`; capturing it from
stdout is the only one-pass option.

### 3. Set secrets

All three Phase 1 secrets, in one batch (so Fly does a single rolling
restart instead of three):

```sh
flyctl secrets set \
    URBANIST_DB_URL="postgres://...paste from the mpg attach output..." \
    URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" \
    URBANIST_CLIENT_SECRET="$(openssl rand -hex 32)"
```

- `URBANIST_DB_URL` — read by `serve`, `migrate`, `seed`, `loaddata`.
- `URBANIST_ADMIN_TOKEN` — bearer token for the (future) admin
  endpoints. Pre-staged here so Phase 2 doesn't need a secrets rotation.
- `URBANIST_CLIENT_SECRET` — the Phase 1 lockdown secret checked
  against the `X-Atlas-Client` request header on every `/api/v1/*` data
  endpoint. The same value must be set on Cloudflare Pages as
  `VITE_API_CLIENT_SECRET` (see § Cloudflare Pages + DNS below).

Keep the printed `URBANIST_CLIENT_SECRET` value — you'll paste it into
the Pages dashboard in slice #21.

`flyctl secrets list` confirms all three are set (names + digests only;
values are never readable after set).

### 4. First deploy

```sh
flyctl deploy            # or: just fly-deploy
```

Fly builds the image from the repo-root `Dockerfile`, pushes it, runs
the `release_command` (`urbanist-atlas-server migrate up`) inside an
ephemeral release machine against the attached database, and only then
flips traffic to the new machines. Migrations failing block the deploy
— a feature, not a bug.

### 5. Seed the database

```sh
flyctl ssh console -C "urbanist-atlas-server loaddata"
```

`loaddata` runs the regions → postal-codes → orgs chain for every
bundled country in dependency order. The underlying loaders are
upsert-based, so re-running is safe — counts won't change unless the
seed files do. Add a country by dropping `seed/regions_<cc>.toml` and
`seed/postal_codes_<cc>.csv` into `api/seed/` and appending an entry to
`api/internal/loaddata/loaddata.go`; the integration test
(`TestPipeline_LoaddataLoadAll`) will pick the new country up
automatically.

If `loaddata` fails partway through (e.g. `loaddata: postal CA: ...`),
the preceding countries' rows stay committed — the loaders are
upsert-based and idempotent, so fix the offending file and re-run.
No reset needed.

### 6. Smoke test the API

The `.fly.dev` hostname is live as soon as `flyctl deploy` reports
healthy — useful for verifying before DNS lands in slice #21. CORS
will reject browsers (the allowlist is `qa.urbanistatlas.com` +
`*.pages.dev`), but `curl` ignores CORS.

```sh
# /healthz is bypass-listed and works without a header.
curl -i https://urbanist-atlas.fly.dev/healthz

# /api/v1/* requires X-Atlas-Client.
curl -sS https://urbanist-atlas.fly.dev/api/v1/lookup\?postal_code=10001\&country=US
# → 401 problem+json

CLIENT_SECRET=$(flyctl ssh console -C 'printenv URBANIST_CLIENT_SECRET')
curl -sS -H "X-Atlas-Client: $CLIENT_SECRET" \
    https://urbanist-atlas.fly.dev/api/v1/lookup\?postal_code=10001\&country=US
# → 200 with X-Data-License: ODbL-1.0 and X-Data-Attribution headers
```

## Ongoing operations

Every recipe below has a `just fly-*` wrapper at the repo root so the
verbs are discoverable.

| Task | Command | Wrapper |
|---|---|---|
| Build + deploy current branch | `flyctl deploy` | `just fly-deploy` |
| Machine + service status | `flyctl status` | `just fly-status` |
| Tail live logs | `flyctl logs` | `just fly-logs` |
| List secrets (names only) | `flyctl secrets list` | `just fly-secrets` |
| Interactive shell on a machine | `flyctl ssh console` | `just fly-ssh` |
| One-off command in a machine | `flyctl ssh console -C "<cmd>"` | — |

A redeploy after a code change is just `git push` + `flyctl deploy`
from `main`; the `release_command` re-runs migrations, no manual step
required.

## Secrets

### Rotation procedure

Both Fly secrets have a partner on the web side. Rotating one means
updating the other in lockstep, or the SPA will start 401-ing.

**`URBANIST_CLIENT_SECRET` ↔ `VITE_API_CLIENT_SECRET` (Pages):**

```sh
NEW=$(openssl rand -hex 32)
flyctl secrets set URBANIST_CLIENT_SECRET="$NEW"
# Then in the Cloudflare Pages dashboard:
#   Pages → urbanist-atlas → Settings → Environment variables
#   Production + Preview: set VITE_API_CLIENT_SECRET = <NEW>
#   Trigger a redeploy (Deployments → Retry on latest, or push a commit)
echo "$NEW"   # paste into the Pages dashboard
```

During the rotation window (Fly secret updated, Pages still serving
the old build): in-flight browser sessions will 401 until Pages
finishes rebuilding (~1 min). Schedule rotations during low traffic.

**`URBANIST_ADMIN_TOKEN`** is bearer-token-style and is read per request,
so rotation is instant from the API's perspective:

```sh
flyctl secrets set URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)"
```

Hand the new value to whichever tool/script consumes the admin endpoints.

**`URBANIST_DB_URL`** is the project's renamed copy of MPG's
`DATABASE_URL`. When MPG rotates credentials it updates `DATABASE_URL`
on the Fly app; the renamed copy is not auto-updated, so the same
manual re-mirror used in the initial bootstrap (§3 above) is required:

```sh
flyctl mpg attach urbanist-atlas-db --app urbanist-atlas   # re-prints DATABASE_URL
flyctl secrets set URBANIST_DB_URL="postgres://...paste from the mpg attach output..."
```

Until that re-mirror runs, `serve`, `migrate`, and `loaddata` keep
connecting with the old credentials and start failing as MPG retires
them.

### What's *not* a secret

The values committed to `fly.toml` (`URBANIST_PORT`, `URBANIST_LOG_FORMAT`,
`URBANIST_STORE`, `URBANIST_CORS_ORIGINS`) are non-sensitive and live in
the repo. Changing them is a code-review concern, not a secret rotation.

## QA → prod transition (future)

When the maintainer is ready to launch prod:

1. **Cloudflare Pages**: add `urbanistatlas.com` as a *second* custom
   domain to the same Pages project. Add the apex + `www` CNAMEs.
2. **Fly**: `flyctl certs create api.urbanistatlas.com`, then add the
   `api` CNAME pointing at `urbanist-atlas.fly.dev` (proxy **off**).
3. **CORS**: edit `fly.toml` to add `https://urbanistatlas.com` to
   `URBANIST_CORS_ORIGINS`, commit, `flyctl deploy`.
4. **Pages env**: switch `VITE_API_BASE` from
   `https://qa-api.urbanistatlas.com` to `https://api.urbanistatlas.com`.
   Trigger a new Pages deploy.
5. **Verify** the prod hostnames serve correctly, then retire the QA
   hostnames: remove `qa` + `qa-api` DNS records, drop the QA cert
   (`flyctl certs remove qa-api.urbanistatlas.com`), remove the QA
   custom domain from the Pages project, edit `fly.toml` to drop the
   `qa.urbanistatlas.com` entry from `URBANIST_CORS_ORIGINS`.
6. **Going forward**: rely on Pages' automatic `*.pages.dev` previews
   and Fly review apps for ephemeral test URLs.

No code change is required for this transition — it's all config and
DNS. The Fly app, the Pages project, the MPG cluster, and every commit
hash are reused as-is.

## Troubleshooting

**`flyctl deploy` fails at the release step.**
The `release_command` runs `urbanist-atlas-server migrate up`. Check
`flyctl logs` for the migration error (usually a Goose SQL syntax
issue or a missing column from a hand-applied prior migration).
Migrations are embedded in the binary (`api/migrations/embed.go`), so
the deployed code and the migration set are always in lockstep —
there's no version-skew failure mode.

**`/healthz` returns 200 but `/api/v1/lookup` returns 401.**
Expected when calling without the `X-Atlas-Client` header. Either
pass the header (see § Smoke test the API) or check that the SPA's
`VITE_API_CLIENT_SECRET` matches the Fly `URBANIST_CLIENT_SECRET`.

**`/api/v1/lookup` returns CORS error in a browser.**
The calling origin isn't in `URBANIST_CORS_ORIGINS`. For local dev
against the deployed API, set `URBANIST_CORS_ORIGINS` to include
`http://localhost:5173`, or run against `just api-run` locally.
`*.pages.dev` covers every Pages preview hostname.

**`flyctl ssh console -C "urbanist-atlas-server loaddata"` fails.**
Either the seed files aren't in the image (verify by `flyctl ssh
console -C "ls /app/seed"`) or `URBANIST_DB_URL` isn't set (verify by
`flyctl secrets list`). Re-deploy if seed files are missing — they
ship in the runtime stage via `COPY api/seed/ ./seed/` in the
`Dockerfile`. The binary finds them via `URBANIST_SEED_DIR=/app/seed`
baked into `fly.toml`'s `[env]` block — no `--seed-dir` flag needed
on Fly.

**Need to inspect production data.**
`flyctl ssh console` opens a shell on the running machine.
`flyctl mpg connect -a urbanist-atlas-db` opens a `psql` session
against the MPG cluster. Both require Fly auth.
