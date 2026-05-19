# Deploy runbook

Operational guide for taking `urbanist-atlas` from a clean Heroku +
Cloudflare account to a working QA deployment. The design that
motivates this runbook is at
[`docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`](./superpowers/specs/2026-05-18-heroku-deploy-design.md);
this file is the executable playbook.

The launch chunk is four slices:

| Slice | What it ships | Where it's documented |
|---|---|---|
| #19 | (Retired by slice #19.5 pivot — `Dockerfile` + `fly.toml` deleted) | — |
| #19.5 | Hosting cost spike + Heroku pivot decision | `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` + `2026-05-18-heroku-deploy-design.md` |
| #20 | Heroku app + Postgres Essential-0 + first deploy + seed | **§ Heroku: API + database** below |
| #21 | Cloudflare Pages + DNS + TLS for `qa.*` hostnames | § Cloudflare Pages + DNS below (added by slice #21) |
| #23 | `X-Atlas-Client` shared-secret gate | In tree; this runbook references the secret in § Secrets |

## Hosting topology

| Component | Resource | Initial hostname |
|---|---|---|
| API | Heroku app `urbanist-atlas`, region `us` (Virginia, Common Runtime), Basic dyno | `qa-api.urbanistatlas.com` |
| DB | Heroku Postgres Essential-0 add-on, attached to the API app | Private to the app via `DATABASE_URL` |
| Web | Cloudflare Pages project `urbanist-atlas`, prod branch `main` | `qa.urbanistatlas.com` |
| Web previews | `<branch>.urbanist-atlas.pages.dev` | Auto-provisioned per non-`main` branch |

None of the resources carry an `-qa` suffix. They are the *same*
resources that will host production; only the *hostnames* are
environment-flavored during QA. When prod launches, prod hostnames
attach to the same app/project and QA hostnames retire — no rebuilds,
no data migration.

## Prerequisites

- [Heroku CLI](https://devcenter.heroku.com/articles/heroku-cli)
  installed and authenticated (`heroku login`). The maintainer's
  Heroku account must be a credit-card account; Enterprise contracts
  are End-of-Sale as of Feb 2026 (see the cost-spike doc for
  context — credit-card accounts are unaffected).
- Cloudflare account with access to the `urbanistatlas.com` zone.
- This repo cloned locally; `Procfile` lives at the repo root.
- (Optional but recommended) `just` available so the `heroku-*`
  recipes in the root [`justfile`](../justfile) work — they are
  thin wrappers around `heroku` so the verbs are discoverable via
  `just --list`.

## Heroku: API + database

### 1. Create the app

```sh
heroku create urbanist-atlas --region us --buildpack heroku/go
```

`--region us` selects the Virginia datacenter (Common Runtime); see
the design doc for the rationale (matches the US/CA user base; the
maintainer accepts personal latency from EU in Phase 1).
`--buildpack heroku/go` pins the buildpack so the first deploy
doesn't autodetect against `web/` or other paths in this monorepo.

The repo's Go module lives at `api/`. Set the Heroku Go buildpack's
monorepo build target:

```sh
heroku config:set GO_INSTALL_PACKAGE_SPEC=./api/cmd/server -a urbanist-atlas
```

(*Verify against the live Heroku docs at provisioning time:* the
buildpack's env-var name for monorepo build targets has changed
across versions. The canonical reference is
<https://devcenter.heroku.com/articles/go-support>. If
`GO_INSTALL_PACKAGE_SPEC` is no longer the current name, substitute
the current equivalent before `git push heroku main`.)

### 2. Provision Postgres Essential-0

```sh
heroku addons:create heroku-postgresql:essential-0 -a urbanist-atlas
```

The add-on attaches automatically, sets `DATABASE_URL` on the app's
config, and starts a daily logical-backup schedule. Aurora-backed
continuous WAL off-premise begins immediately. Confirm with:

```sh
heroku config -a urbanist-atlas | grep DATABASE_URL
heroku pg:info -a urbanist-atlas
```

### 3. Set application config + secrets

Non-secret config:

```sh
heroku config:set \
    URBANIST_LOG_FORMAT=json \
    URBANIST_STORE=postgres \
    URBANIST_CORS_ORIGINS="https://qa.urbanistatlas.com,*.pages.dev" \
    URBANIST_SEED_DIR=./seed \
    -a urbanist-atlas
```

Secrets (`URBANIST_ADMIN_TOKEN` is pre-staged for Phase 2;
`URBANIST_CLIENT_SECRET` is read by the slice-#23 `X-Atlas-Client`
middleware and must match the value in the Cloudflare Pages
`VITE_API_CLIENT_SECRET` env):

```sh
heroku config:set \
    URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" \
    URBANIST_CLIENT_SECRET="$(openssl rand -hex 32)" \
    -a urbanist-atlas
```

Keep the printed `URBANIST_CLIENT_SECRET` value — paste it into the
Cloudflare Pages dashboard in slice #21.

### 4. First deploy

```sh
heroku git:remote -a urbanist-atlas
git push heroku main          # or: just heroku-deploy
```

The push triggers a buildpack compile of the Go binary at
`./api/cmd/server` (per `GO_INSTALL_PACKAGE_SPEC`), produces a slug
containing the `urbanist-atlas-server` binary at the slug root + the
`api/seed/` directory, then runs the `release` Procfile entry
(`urbanist-atlas-server migrate up`) in an ephemeral release dyno
against `DATABASE_URL`. A failing migration blocks the traffic flip
— a feature, not a bug.

### 5. Seed the database

```sh
heroku run urbanist-atlas-server loaddata -a urbanist-atlas
# or: just heroku-loaddata
```

`loaddata` runs the regions → postal-codes → orgs chain for every
bundled country in dependency order. The loaders are upsert-based,
so re-running is safe — counts won't change unless the seed files
do. Add a country by dropping `seed/regions_<cc>.toml` and
`seed/postal_codes_<cc>.csv` into `api/seed/` and appending an
entry to `api/internal/loaddata/loaddata.go`; the integration test
(`TestPipeline_LoaddataLoadAll`) picks the new country up
automatically via `loaddata.Countries()`.

If `loaddata` fails partway through (e.g. `loaddata: postal CA: …`),
the preceding countries' rows stay committed — the loaders are
upsert-based and idempotent, so fix the offending file and re-run.
No reset needed.

`URBANIST_SEED_DIR=./seed` was set in step 3 because the buildpack's
slug working directory is the repo root, not `api/`. If `heroku run`
ever can't find the seed files, run `heroku run ls -a urbanist-atlas`
to verify the slug layout, then adjust `URBANIST_SEED_DIR`.

### 6. Smoke-test the API

The auto-generated Heroku hostname is live as soon as the release
dyno completes — useful before DNS lands in slice #21:

```sh
HEROKU_URL=$(heroku apps:info -a urbanist-atlas | awk -F'= ' '/Web URL/ {print $2}')

# /healthz is bypass-listed and works without a header.
curl -i "${HEROKU_URL%/}/healthz"

# /api/v1/* requires X-Atlas-Client.
curl -sS "${HEROKU_URL%/}/api/v1/lookup?postal_code=10001&country=US"
# → 401 problem+json

CLIENT_SECRET=$(heroku config:get URBANIST_CLIENT_SECRET -a urbanist-atlas)
curl -sS -H "X-Atlas-Client: $CLIENT_SECRET" \
    "${HEROKU_URL%/}/api/v1/lookup?postal_code=10001&country=US"
# → 200 with X-Data-License: ODbL-1.0 and X-Data-Attribution headers
```

### 7. Schedule the backup window

```sh
heroku pg:backups:schedule DATABASE_URL --at "02:00 America/New_York" -a urbanist-atlas
heroku pg:backups -a urbanist-atlas    # list schedule + retained backups
```

(The `us` region's overnight low-traffic window aligns with Eastern
Time. Adjust if your user base shifts.)

## Ongoing operations

Every recipe below has a `just heroku-*` wrapper at the repo root
so the verbs are discoverable via `just --list`.

| Task | Command | Wrapper |
|---|---|---|
| Build + deploy current branch | `git push heroku main` | `just heroku-deploy` |
| Tail live logs | `heroku logs --tail -a urbanist-atlas` | `just heroku-logs` |
| List config (names + masked values) | `heroku config -a urbanist-atlas` | `just heroku-config` |
| Interactive one-off dyno | `heroku run bash -a urbanist-atlas` | `just heroku-ssh` |
| Re-seed the database | `heroku run urbanist-atlas-server loaddata -a urbanist-atlas` | `just heroku-loaddata` |
| Ad-hoc backup snapshot | `heroku pg:backups:capture -a urbanist-atlas` | `just db-backup` |
| psql against the DB | `heroku pg:psql -a urbanist-atlas` | — |

A redeploy after a code change is `git push heroku main` from the
local branch tracking `origin/main`; the `release` Procfile entry
re-runs migrations, no manual step required.

## Secrets

### Rotation procedure

Both app-level secrets have a partner on the web side. Rotating one
means updating the other in lockstep, or the SPA will start 401-ing.

**`URBANIST_CLIENT_SECRET` ↔ `VITE_API_CLIENT_SECRET` (Pages):**

```sh
NEW=$(openssl rand -hex 32)
heroku config:set URBANIST_CLIENT_SECRET="$NEW" -a urbanist-atlas
# Then in the Cloudflare Pages dashboard:
#   Pages → urbanist-atlas → Settings → Environment variables
#   Production + Preview: set VITE_API_CLIENT_SECRET = <NEW>
#   Trigger a redeploy (Deployments → Retry on latest, or push a commit)
echo "$NEW"   # paste into the Pages dashboard
```

During the rotation window (Heroku config updated, Pages still
serving the old build): in-flight browser sessions will 401 until
Pages finishes rebuilding (~1 min). Schedule rotations during low
traffic.

**`URBANIST_ADMIN_TOKEN`** is bearer-token-style and is read per
request, so rotation is instant from the API's perspective:

```sh
heroku config:set URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" -a urbanist-atlas
```

Hand the new value to whichever tool/script consumes the admin
endpoints.

**`DATABASE_URL`** is managed entirely by the Heroku Postgres
add-on. The add-on rotates credentials on its own schedule; the
binary picks up the new value on the next dyno restart (Heroku
restarts dynos automatically when config changes). No manual step
required — this is the primary reason we use `DATABASE_URL` directly
rather than mirroring into `URBANIST_DB_URL`.

### What's *not* a secret

All the values set via `heroku config:set` in §3 except
`URBANIST_ADMIN_TOKEN` and `URBANIST_CLIENT_SECRET` are
non-sensitive; they live in app config rather than the repo only
because Heroku doesn't have a `[env]`-equivalent in `Procfile`.
Treat changes to them as code-review concerns even though they're
not technically secret.

## QA → prod transition (future)

When the maintainer is ready to launch prod:

1. **Cloudflare Pages**: add `urbanistatlas.com` as a *second*
   custom domain to the same Pages project. Add the apex + `www`
   CNAMEs.
2. **Heroku**: `heroku domains:add api.urbanistatlas.com -a urbanist-atlas`.
   Add a CNAME `api` → the Heroku-managed target printed by
   `heroku domains -a urbanist-atlas` (Cloudflare proxy **off**).
3. **CORS**: `heroku config:set URBANIST_CORS_ORIGINS="https://qa.urbanistatlas.com,https://urbanistatlas.com,*.pages.dev" -a urbanist-atlas`.
   Heroku restarts the dyno automatically.
4. **Pages env**: switch `VITE_API_BASE` from
   `https://qa-api.urbanistatlas.com` to
   `https://api.urbanistatlas.com`. Trigger a new Pages deploy.
5. **Verify** the prod hostnames serve correctly, then retire the
   QA hostnames: remove `qa` + `qa-api` DNS records, drop the QA
   custom domain from Heroku
   (`heroku domains:remove qa-api.urbanistatlas.com`), remove the
   QA custom domain from the Pages project, drop the
   `https://qa.urbanistatlas.com` entry from
   `URBANIST_CORS_ORIGINS`.
6. **Going forward**: rely on Pages' automatic `*.pages.dev`
   previews for ephemeral web environments.

No code change is required for this transition — it's all config
and DNS. The Heroku app, the Pages project, the Postgres add-on,
and every commit hash are reused as-is.

## Troubleshooting

**`git push heroku main` fails during the release step.**
The `release` Procfile entry runs `urbanist-atlas-server migrate up`.
Check `heroku logs --tail -a urbanist-atlas` for the migration error
(usually a Goose SQL syntax issue or a missing column from a
hand-applied prior migration). Migrations are embedded in the binary
(`api/migrations/embed.go`), so the deployed code and the migration
set are always in lockstep — there's no version-skew failure mode.

**`/healthz` returns 200 but `/api/v1/lookup` returns 401.**
Expected when calling without the `X-Atlas-Client` header. Either
pass the header (see § Smoke-test the API) or check that the SPA's
`VITE_API_CLIENT_SECRET` matches the Heroku
`URBANIST_CLIENT_SECRET` value.

**`/api/v1/lookup` returns CORS error in a browser.**
The calling origin isn't in `URBANIST_CORS_ORIGINS`. For local dev
against the deployed API, add `http://localhost:5173` to the
config, or run against `just api-run` locally. `*.pages.dev`
covers every Pages preview hostname.

**`heroku run urbanist-atlas-server loaddata` fails.**
Either the seed files aren't in the slug (verify by
`heroku run ls ./seed -a urbanist-atlas`) or `DATABASE_URL` isn't
set (verify by `heroku config -a urbanist-atlas | grep DATABASE_URL`).
Re-deploy if seed files are missing — they ship in the slug because
the buildpack copies the whole repo into the build context.
`URBANIST_SEED_DIR=./seed` is set in §3 above; if the buildpack's
slug working directory ever shifts, adjust it.

**Need to inspect production data.**
`heroku pg:psql -a urbanist-atlas` opens a `psql` session against
the Postgres add-on. `heroku run bash -a urbanist-atlas` opens a
shell on a one-off dyno. Both require Heroku CLI auth.

**Reverting to Fly (if dogfood reveals Heroku isn't the right fit).**
See § Reversibility in
[`docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`](./superpowers/specs/2026-05-18-heroku-deploy-design.md).
Reversal is bounded (1–2 evenings) as long as you decide before
attaching prod hostnames.
