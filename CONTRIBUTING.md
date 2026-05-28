# Contributing to Urbanist Atlas

Thanks for the interest. This project welcomes contributions from
both **organizers** (adding or correcting directory entries) and
**engineers** (code, docs, infrastructure).

## What this project is

A directory of transit and safe-streets advocacy organizations,
searchable by US ZIP or Canadian postal code. The dataset is a
hand-curated editorial product; the codebase is the small machinery
that serves it. Companion to *Urbanist Lexicon* at
[mjrossi.com](https://mjrossi.com).

Scope is deliberately narrow:

- **In scope:** transit advocacy (riders' alliances, bus and rail
  coalitions), safe-streets advocacy (Vision Zero groups, traffic-
  calming coalitions, pedestrian and cycling alliances).
- **Out of scope (deliberately):** housing / YIMBY organizations
  (even when adjacent), consultancies, think tanks, and academic
  centers — unless they double as a membership advocacy group.
- **Geography:** US + Canada in v1. Other countries are tracked in
  the roadmap; the data model already supports them.

If you're not sure whether something fits, open an issue and ask.

## Ways to contribute

### Suggest or correct an organization

File a tip through the submission form on the site, or — if your
change needs a written trail — open an [organization correction or
addition][issue-org] issue. This is the most useful contribution for
most readers; the directory only works if its entries are right.

Good entries include:

- Organization name and primary website
- A one-line description of what they actually do (not their tagline)
- The region they serve (city, county, metro, state)
- Whether they're a chapter of a national federation

If you're listing your own organization, say so in the submission —
it's not disqualifying, just useful context for the editor.

The public submission form on the site is the primary channel; the
issue tracker is the fallback for corrections that need a written
trail. Either way, please read
[`docs/editorial-style.md`](./docs/editorial-style.md) for the voice
the directory aims for — verb-first descriptions, concrete campaigns
over adjectives, words to avoid.

### Report a bug

Open a [bug report][issue-bug]. Include the URL, the postal code or
metro slug you used, what you expected, and what happened. If it's
an API bug, the `request_id` from any error response is gold —
server logs key on it.

### Suggest a feature

Open a [feature request][issue-feature]. Smaller, scoped ideas land
faster than redesigns. Roadmap items live in
[`docs/roadmap.md`](./docs/roadmap.md); see if your idea is already
queued.

### Contribute code

Pull requests welcome. A few things to know first:

1. **Read [`CLAUDE.md`](./CLAUDE.md).** It's the project's contract
   for tech conventions — what libraries are approved, what the
   code shape looks like, what's deliberately out of scope.
2. **Read the architecture docs** for the area you're touching:
   - [`docs/api-architecture.md`](./docs/api-architecture.md) —
     Go-side library split, store abstraction, error/response
     envelopes
   - [`docs/etl-architecture.md`](./docs/etl-architecture.md) —
     postal-code + region ETL pipeline
   - [`docs/region-graph.md`](./docs/region-graph.md) — region
     DAG modeling rules
   - [`docs/testing-strategy.md`](./docs/testing-strategy.md) —
     when to write unit / handler / integration tests
3. **Run the dev loop** to make sure your change actually works.
   The repo's [`README.md`](./README.md) has the quick-start steps.
4. **Open an issue first** for anything beyond a small fix — saves
   you from writing code that doesn't fit the project's direction.

## Development setup

The repo is monorepo (`api/` Go + `web/` React) and uses
[mise](https://mise.jdx.dev) to pin language and tool versions:

```sh
# one time
mise install                  # provisions Go, Node, oapi-codegen, etc.
# add MISE_ENV=development to your shell rc per mise.development.toml

# the dev loop
just api-run                  # API on :8080 (loads api/seed/ into memory)
# in another shell:
cd web && npm install && npm run dev    # SPA on :5173
```

No database to start: the API reads the `api/seed/` TOML/CSV bundle
into an in-memory FileStore at boot.

Common verbs (run `just` with no args for the full list):

- `just api-check` — vet + race-enabled tests + oapi gen-no-diff
- `just web-check` — lint + vitest + build + TS gen-no-diff
- `just ci` — what CI runs (`api-check` + `web-check`)

## Full-stack PR review

Cloudflare Workers preview URLs target the *current* production API
at `api.urbanistatlas.com`, **not** the API as it stands on the
PR's branch. For PRs that add or change an API endpoint, the
preview frontend will 404 (or otherwise misbehave) against the
not-yet-deployed backend. Two workflows handle this:

1. **Local stack** — check out the PR branch and run the whole
   stack locally:

   ```sh
   gh pr checkout <PR#>
   just preview         # one terminal: API on :8080 (file store)
   just web-dev         # another terminal: SPA on http://localhost:5173
   ```

   `just preview` boots the file-backed API in the foreground; there's
   no DB to manage.

2. **Merge backend first** — if the work can be split, land + deploy
   the API change first; then open the frontend PR. The Cloudflare
   preview will then hit a production API that has the new endpoint
   and Just Work.

This asymmetry exists because Workers previews are stateless and
near-free, while a per-branch API preview would need its own
database. A shared "preview" Fly app (one extra ~$5/mo machine
that takes the latest push from any non-main branch) is a queued
ergonomics upgrade in [`docs/roadmap.md`](./docs/roadmap.md) for
when full-stack PR volume justifies it.

## Pull request guidelines

- **One topic per PR.** Refactors, behavior changes, and doc
  updates are easier to review separately. Mixed PRs get bounced.
- **Tests.** New behavior needs tests. The
  [testing-strategy doc](./docs/testing-strategy.md) explains which
  tier to use.
- **Wire contract first.** If you're adding or changing an endpoint,
  edit `api/openapi.yaml` first, regenerate via `just api-gen`
  and `just web-oapi-gen`, then implement against the generated
  types. Both halves are guarded against drift.
- **Commits.** Small, focused, with a present-tense subject ("add
  X", not "added X"). Body explains *why* when the *what* isn't
  obvious from the diff.
- **No `--no-verify`, no force-pushes to `main`.** Pre-commit hooks
  catch real things; force-pushes destroy history other people
  depend on.
- **CI must pass.** Both `api-check` and `web-check` jobs (the union
  is `just ci`).

## Licensing of contributions

This is a multi-license repository (see [`README.md`](./README.md#license)
for the full table). By submitting a contribution, you agree that:

- **Code contributions** (changes under `api/`, `web/`, build tooling)
  are licensed under [Apache License 2.0](./LICENSE).
- **Data contributions** (organization entries, postal-code data,
  region taxonomy) are licensed under
  [Open Database License (ODbL) 1.0](./LICENSE-DATA).
- **Prose contributions** (documentation, copy, READMEs) are
  licensed under [CC BY-SA 4.0](./LICENSE-CONTENT).

You also confirm you have the right to submit the contribution
under those terms (i.e., the work is yours, or you have permission
from the rights holder).

No separate CLA. The Apache 2.0 license's patent grant covers the
code path; the ODbL and CC BY-SA share-alike terms cover the data
and prose paths.

## Code of conduct

This project follows the
[Contributor Covenant](./CODE_OF_CONDUCT.md). Be kind, assume good
faith, and remember everyone here is doing this voluntarily.

## Security

Found a security issue? Please **don't** open a public issue.
See [`SECURITY.md`](./SECURITY.md) for the private reporting path.

## Where to ask questions

- **Github issues:** for anything bug-like, anything specific to a
  feature or organization entry, or anything that needs a written
  trail.
- **Discussions:** if enabled on the repo, for open-ended questions
  about direction or scope.

If your question is "should I work on X?", open an issue. The
five-minute answer is worth more than a week of unaligned work.

[issue-bug]: https://github.com/mjrossi/urbanist-atlas/issues/new?template=bug_report.md
[issue-feature]: https://github.com/mjrossi/urbanist-atlas/issues/new?template=feature_request.md
[issue-org]: https://github.com/mjrossi/urbanist-atlas/issues/new?template=org_correction_or_addition.md
