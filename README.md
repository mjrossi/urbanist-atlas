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

Scaffolding in progress. The approved design lives at
`~/.claude/plans/we-are-planning-a-smooth-candy.md` (local to the
maintainer; the salient pieces are mirrored into [`CLAUDE.md`](./CLAUDE.md)).
The remaining implementation work is broken into ~23 slices in
[`docs/roadmap.md`](./docs/roadmap.md).

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

