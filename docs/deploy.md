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
| API | Fly app `urbanist-atlas`, region `iad` (Virginia, US East), shared-cpu-1x / 256 MB. Stateless: `api/seed/` is baked into the image and loaded into an in-memory FileStore at boot. | `qa-api.urbanistatlas.com` |
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
| `URBANIST_ADMIN_TOKEN` | bearer token for admin endpoints | `flyctl secrets set URBANIST_ADMIN_TOKEN=<value> -a urbanist-atlas` |
| `URBANIST_CLIENT_SECRET` | Phase 1 shared-secret `X-Atlas-Client` gate; mirrored to the SPA build as `VITE_API_CLIENT_SECRET` | `flyctl secrets set URBANIST_CLIENT_SECRET=<value> -a urbanist-atlas` |

Generate fresh values with `openssl rand -hex 32`. The client secret
must match the value built into the SPA bundle for the gate to pass.

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

### 1. Create the Fly app

```sh
flyctl apps create urbanist-atlas --org <your-org>
flyctl secrets set \
  URBANIST_ADMIN_TOKEN="$(openssl rand -hex 32)" \
  URBANIST_CLIENT_SECRET="$(openssl rand -hex 32)" \
  -a urbanist-atlas
flyctl deploy --remote-only -a urbanist-atlas
```

The `[env]` section of `fly.toml` pins `URBANIST_SEED_DIR=/app/seed`,
matching the Dockerfile's `COPY api/seed/ ./seed/`. Boot should log
`store initialized kind=file regions=<n> orgs=<n>` within ~1s.

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
carries everything else (asset handling, SPA fallback, headers).

Set the SPA's API base + client secret as Workers environment
variables:

| Variable | Value |
|---|---|
| `VITE_API_BASE_URL` | `https://qa-api.urbanistatlas.com` |
| `VITE_API_CLIENT_SECRET` | the same value set on Fly as `URBANIST_CLIENT_SECRET` |

CNAME `qa.urbanistatlas.com` → the Workers project hostname; add it
in the Workers project's custom-domain settings.

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

## Future state: writable surface

The public submission queue is not yet wired at the storage layer.
The next phase introduces a small SQLite store on a Fly volume for
submissions + future API keys; approval will open a GitHub PR
appending the org to `orgs.toml`. See the design plan in
`.claude/plans/i-want-to-consider-proud-beacon.md`.
