# Urbanist Atlas

> Find the people fighting for better streets where you live.

A geographically-organized directory of transit and safe-streets advocacy
organizations across the US and Canada. Enter a ZIP or postal code, get
back the local and regional groups working in your area.

A companion volume to [*Urbanist Lexicon*](https://mjrossi.com).

**Site:** Not yet deployed. Phase 1 dogfooding will attach to `qa.urbanistatlas.com` (SPA) and `qa-api.urbanistatlas.com` (API) when the [deploy runbook](./docs/deploy.md) is executed against a fresh Fly.io + Cloudflare Pages account; the production `urbanistatlas.com` hostname attaches to the same Pages project once Phase 2 (API keys, rate limiting) ships.

---

## Repository layout

This is a monorepo with two halves:

- **[`api/`](./api)** — Go service (chi + sqlc + goose +
  `postgres:17-alpine` on a sibling Fly app), deployed to Fly.io.
  Hosts the public JSON API at `/api/v1`.
- **[`web/`](./web)** — React + Vite SPA, deployed to Cloudflare Pages.
  Consumes the JSON API.

See each subdirectory's `README.md` for build instructions, and the
top-level [`CLAUDE.md`](./CLAUDE.md) for project conventions and the
reasoning behind tech choices.

## Status

Most of v1.0 is wired. The API serves `/lookup`, `/metros`,
`/metros/{slug}`, and `/recent` against a Postgres store with
embedded migrations; the v1 wire contract is committed at
[`api/openapi.yaml`](./api/openapi.yaml) and embedded into the
binary at `GET /api/v1/openapi.yaml`. Every `/api/v1/**` success
response carries ODbL attribution headers; collection responses
wrap their payload in a `{ meta, data }` envelope. The SPA renders
home (search) + results + browse + per-metro + about + 404
against the live API. Errors on both halves use
[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)
`application/problem+json`.

Remaining work to v1.0 — executing the Fly + Cloudflare Pages
deploy runbook (slice #20.6 deliverables landed as code, config,
and docs, but the runbook has not yet been run against live
infrastructure), and the Phase 1 lockdown sequence — is tracked in
[`docs/roadmap.md`](./docs/roadmap.md). Public submissions are
deferred to Phase 2 alongside the API-key + email-verified account
system. The full architectural plan lives at
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (local to the
maintainer); the load-bearing pieces are mirrored into
[`CLAUDE.md`](./CLAUDE.md).

### Quick start

One-time setup: install [mise](https://mise.jdx.dev) and add
`MISE_ENV=development` to your shell rc (see
[`mise.development.toml`](./mise.development.toml) for the exact line).

```sh
mise install                  # provision Go, Node, sqlc, goose, staticcheck, oapi-codegen
just pg-up                    # start the dev Postgres in a docker container on :55432
just migrate-up               # apply migrations against the dev DB
just api-run                  # API on :8080 (text logs)

# in another shell:
cd web && npm install && npm run dev    # SPA on :5173
```

### Deploy

The API is configured to ship to Fly.io (region `iad`, Virginia) via
a multi-stage `Dockerfile` at the repo root; the API's `fly.toml`
declares the build, runtime config, and `release_command` for
migrations. The database runs on a sibling Fly app
`urbanist-atlas-db` (config at `infra/postgres/fly.toml`) executing
`postgres:17-alpine` with a 1 GB volume — same image as the
integration test suite. The web SPA deploys to Cloudflare Pages from
`web/`, with `web/public/_redirects` providing the SPA fallback for
direct navigation. Nightly `pg_dump` backups land in Cloudflare R2
via the GitHub Actions workflow at
[`.github/workflows/backup.yml`](./.github/workflows/backup.yml).
**No deploy has been executed yet** — the initial provisioning
runbook (creating both Fly apps, attaching the volume, wiring DNS
and certs, setting secrets, enabling backups) lives at
[`docs/deploy.md`](./docs/deploy.md); see slice #20.6 in the
[roadmap](./docs/roadmap.md). Ongoing ops will use the `fly-*` /
`db-*` recipes (`just fly-deploy`, `just fly-logs`,
`just fly-secrets`, `just fly-ssh`, `just fly-loaddata`,
`just db-backup`, `just db-restore <file>`).

The hosting decision is documented at
[`docs/superpowers/specs/2026-05-18-hosting-cost-spike.md`](./docs/superpowers/specs/2026-05-18-hosting-cost-spike.md)
and
[`docs/superpowers/specs/2026-05-21-fly-redeploy-design.md`](./docs/superpowers/specs/2026-05-21-fly-redeploy-design.md)
(which supersedes the earlier
[`2026-05-18-heroku-deploy-design.md`](./docs/superpowers/specs/2026-05-18-heroku-deploy-design.md)).
The full chunk design lives at
[`docs/superpowers/specs/2026-05-18-qa-deploy-design.md`](./docs/superpowers/specs/2026-05-18-qa-deploy-design.md).

## Contributing organizations

Once the site is live: visit `/submit` to propose an organization. All
submissions are reviewed by a human before going live.

## License

Urbanist Atlas is a multi-license project; different parts of the
repository are governed by different licenses appropriate to their type.

| Component | License | File |
| --- | --- | --- |
| Source code (`api/`, `web/`, build tooling) | [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0) | [`LICENSE`](./LICENSE) |
| Organization directory dataset | [Open Database License (ODbL) 1.0](https://opendatacommons.org/licenses/odbl/1-0/) | [`LICENSE-DATA`](./LICENSE-DATA) |
| Documentation and prose content | [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) | [`LICENSE-CONTENT`](./LICENSE-CONTENT) |

Commercial use is permitted under all three licenses. The dataset and
content licenses require share-alike on derivative works; the code license
does not.

### Contributing organizations

By submitting an organization through the `/submit` flow on
[urbanistatlas.com](https://urbanistatlas.com), you confirm the information
is accurate to your knowledge and agree to license your contribution under
the Open Database License (ODbL) 1.0.

