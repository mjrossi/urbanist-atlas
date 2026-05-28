# Urbanist Atlas

> Find the people fighting for better streets where you live.

A geographically-organized directory of transit and safe-streets advocacy
organizations across the US and Canada. Enter a ZIP or postal code, get
back the local and regional groups working in your area.

A companion volume to [*Urbanist Lexicon*](https://mjrossi.com).

**Site:** Live at [`urbanistatlas.com`](https://urbanistatlas.com) (SPA on Cloudflare Workers + Pages) and `api.urbanistatlas.com` (API on Fly.io, region `iad`). The API is still locked down behind an `X-Atlas-Client` shared-secret header bundled into the frontend build — a Phase 1 holdover; public, unkeyed access opens up with Phase 2 (API keys + rate limiting, roadmap slices #26–#28).

---

## Repository layout

This is a monorepo with two halves:

- **[`api/`](./api)** — Go service (chi router), deployed to Fly.io.
  Stateless: the `api/seed/` TOML/CSV bundle is baked into the image
  and loaded into an in-memory FileStore at boot. Hosts the public
  JSON API at `/api/v1`.
- **[`web/`](./web)** — React + Vite SPA, deployed to Cloudflare
  Workers + Pages (Static Assets). Consumes the JSON API.

See each subdirectory's `README.md` for build instructions, and the
top-level [`CLAUDE.md`](./CLAUDE.md) for project conventions and the
reasoning behind tech choices.

## Status

Most of v1.0 is wired. The API serves `/lookup`, `/regions`,
`/regions/{slug}`, `/recent`, and `/orgs/{slug}` from an in-memory
FileStore populated at boot from `api/seed/`; the v1 wire contract
is committed at [`api/openapi.yaml`](./api/openapi.yaml) and embedded
into the binary at `GET /api/v1/openapi.yaml`. `/regions` returns the
editorial default browse set (metros + cities); `/regions/{slug}`
resolves any non-national region in the DAG — metros, cities,
counties, boroughs, states, multi-state coalitions. Every
`/api/v1/**` success response carries ODbL attribution headers;
collection responses wrap their payload in a `{ meta, data }`
envelope. The SPA renders home (search) + results + browse +
per-region + per-org + about + 404 against the live API. Errors on both halves use
[RFC 9457](https://www.rfc-editor.org/rfc/rfc9457.html)
`application/problem+json`.

Remaining work to v1.0 — editorial drip of orgs, then the Phase 2
public launch (slices #26–#28: API keys, rate limiting, prod-hostname
cutover) — is tracked in [`docs/roadmap.md`](./docs/roadmap.md).
Public submissions are live: `POST /api/v1/submissions` lands new
orgs in a small SQLite queue on the Fly volume, and approving a
submission opens a GitHub PR appending it to `api/seed/orgs.toml`.
The Phase 2 work is the API-key + email-verified account system that
opens the read API beyond the shared-secret gate. The full
architectural plan lives at
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (local to the
maintainer); the load-bearing pieces are mirrored into
[`CLAUDE.md`](./CLAUDE.md).

### Quick start

One-time setup: install [mise](https://mise.jdx.dev) and add
`MISE_ENV=development` to your shell rc (see
[`mise.development.toml`](./mise.development.toml) for the exact line).

```sh
mise install                  # provision Go, Node, staticcheck, oapi-codegen
just api-run                  # API on :8080 (text logs); also serves /healthz + /readyz

# in another shell:
cd web && npm install && npm run dev    # SPA on :5173
```

No database is required — the API loads the bundled seed (TOML/CSV
under `api/seed/`) into memory at boot.

### Deploy

The API ships to Fly.io (region `iad`, Virginia) as a single
stateless app via a multi-stage `Dockerfile` at the repo root; the
image bakes `api/seed/` so a deploy is a code+data ship in one step.
The web SPA deploys to Cloudflare Workers + Pages (Static Assets)
configured via `wrangler.jsonc` at the repo root, which sets
`assets.directory = "./web/dist"` and
`not_found_handling = "single-page-application"` (the SPA fallback
that lets direct navigation to `/about`, `/browse`, `/r/:postalCode`
work).

Every push to `main` auto-deploys the API to Fly via the `deploy-api`
job in [`.github/workflows/ci.yml`](./.github/workflows/ci.yml). The
web SPA auto-deploys via Cloudflare's git integration. `just fly-deploy`
remains as a manual fallback when Actions is degraded or a non-`main`
branch needs a hot-fix. Seed-data edits go through the same code
deploy: edit `api/seed/**`, open a PR, merge — the next API image
carries the new bundle.

The runbook for ongoing ops + first-time bring-up lives at
[`docs/deploy.md`](./docs/deploy.md). Editorial workflows for adding
or correcting orgs and regions are at
[`docs/editorial.md`](./docs/editorial.md).

The original Postgres-backed deploy design (since superseded by the
file-store cutover) lives at
[`docs/superpowers/specs/2026-05-21-fly-deploy-design.md`](./docs/superpowers/specs/2026-05-21-fly-deploy-design.md).

## Contributing

Pull requests, bug reports, and organization suggestions are all
welcome. Start with [`CONTRIBUTING.md`](./CONTRIBUTING.md) — it
covers the scope guardrails (what's in, what's deliberately out),
the dev-loop setup, and the PR / commit conventions.

For organizations to add or correct: file a tip through the
submission form on the site, or open an
[org-correction issue](./.github/ISSUE_TEMPLATE/org_correction_or_addition.md)
if the change needs a written trail. The editorial voice the
directory aims for is documented in
[`docs/editorial-style.md`](./docs/editorial-style.md).

This project follows the
[Contributor Covenant](./CODE_OF_CONDUCT.md). Security issues go
through GitHub's private vulnerability reporting per
[`SECURITY.md`](./SECURITY.md) — please don't open a public issue
for those.

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

