# Urbanist Atlas

> Find the people fighting for better streets where you live.

A geographically-organized directory of transit and safe-streets advocacy
organizations across the US and Canada. Enter a ZIP or postal code, get
back the local and regional groups working in your area.

A companion volume to [*Urbanist Lexicon*](https://mjrossi.com).

**Site:** [urbanistatlas.com](https://urbanistatlas.com) *(not yet live)*

---

## Repository layout

This is a monorepo with two halves:

- **[`api/`](./api)** — Go service (chi + sqlc + goose + Fly Managed Postgres),
  deployed to Fly.io. Hosts the public JSON API at `/api/v1`.
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

Remaining slices to v1.0 — postal-code data expansion, seed-data
growth, Dockerfile + Fly deploy, Cloudflare Pages, and the Phase 1
lockdown sequence — are tracked in
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

The API ships as a multi-stage Docker image to Fly.io
(`fly.toml` + `Dockerfile` at the repo root); the web SPA deploys
to Cloudflare Pages from `web/`. Initial provisioning steps
(creating the Fly app, attaching Fly Managed Postgres, wiring DNS,
setting secrets) are documented in
[`docs/deploy.md`](./docs/deploy.md) — see slice #20 / #21 in the
[roadmap](./docs/roadmap.md) for status. Ongoing ops use the
`fly-*` recipes (`just fly-deploy`, `just fly-status`,
`just fly-logs`, `just fly-secrets`, `just fly-ssh`).

The full chunk design (slices #19/#20/#21/#23) lives at
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

