# Submissions on SQLite + GitHub-PR auto-promote (slice β)

**Status:** Planned. Not yet started. Follows the slice α file-store
cutover (PR #52 / branch `slice-alpha-file-store-cutover`), which
makes `api/seed/` the runtime source of truth and retires Postgres
for reads. This slice adds the only writable surface the project
needs in the foreseeable future.

**Related:**
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §Hosting — current
  (stateless) shape after slice α
- [`2026-05-21-fly-deploy-design.md`](./2026-05-21-fly-deploy-design.md) —
  the original Postgres-backed design, now historical
- [`../../../api/openapi.yaml`](../../../api/openapi.yaml) —
  the wire contract this slice extends
- [`../../deploy.md`](../../deploy.md) — runbook (this slice updates
  the §Application secrets and §Bring-up sections)

## Why this exists

The public submission queue is the one thing in Urbanist Atlas that
needs to accept writes from anonymous internet traffic. Slice α
deliberately left this surface unimplemented because the storage
choice was load-bearing — building it on Postgres at that point
would have re-introduced the sibling database we were retiring.

This slice picks the writable store with the rest of the project
already converged on a "data lives in `api/seed/`, served from
memory" shape. Every plausible writable surface for the next 12
months (submissions, Phase 2 API keys, rate-limit counters, key
usage telemetry) is flat / append-only / KV — none of it is
relational. The combination doesn't add up to a relational story
either. Postgres would be overkill; SQLite covers all of it on a
single 1 GiB Fly volume with no operational surface.

The org-promotion side of approval also changes: instead of
`INSERT INTO organizations …` (the Postgres-era flow), an approved
submission opens a GitHub PR that appends the new entry to
`api/seed/orgs.toml`. The maintainer reviews/merges; the next API
deploy ships the embedded bundle with the new org in it. This
keeps the curated dataset's audit log in git, where it has always
belonged.

## Decision: SQLite on a Fly volume

| Component | Choice | Why |
|---|---|---|
| Driver | `modernc.org/sqlite` (pure Go) | No CGO, keeps the Alpine runtime image minimal and the build reproducible. sqlc's `sqlite` engine supports it. |
| Schema codegen | sqlc (sqlite engine) | Same workflow as the prior Postgres adapter; types in `internal/store/sqlite/gen/`. |
| Migrations | goose (sqlite dialect) | Same library the Postgres path used; switching dialects is a one-line change in the runner. |
| File location | `/data/atlas.db` on a 1 GiB Fly volume | A dedicated volume keeps the DB lifecycle separate from machine recycles. 1 GiB is the smallest Fly offers and is years of headroom for the projected write volume. |
| Backup | Nightly GitHub Actions cron → R2 | `sqlite3 /data/atlas.db .dump | gzip` via `flyctl ssh console`. The whole DB is bytes-to-KB for the foreseeable future. |
| Single-writer | Yes (Fly app stays at min-machines=1) | SQLite isn't a clustered DB. Submissions volume makes one machine fine for years; a future need for HA forces a re-decision. |

This is **not** a regression from the Postgres path: nothing about
the submission workflow needs relational queries, transactions
across tables, or PostGIS. The only place a future feature might
genuinely want a real RDBMS is geospatial — and the architecture
deliberately avoids lat/lon (postal-code → region graph instead).
If that day comes, migrating SQLite → managed Postgres is well-
trodden ground.

## Storage layer

### Schema

`migrations-sqlite/0001_init.sql`:

```sql
CREATE TABLE submissions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    public_id       TEXT NOT NULL UNIQUE,        -- app-generated UUIDv7 string; the one in URLs
    payload_json    TEXT NOT NULL,                -- the full Submission body as JSON
    submitter_name  TEXT,
    submitter_email TEXT,
    submitter_note  TEXT,
    status          TEXT NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','approved','rejected')),
    rejection_reason TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    processed_at    TEXT,
    promotion_pr_url TEXT,                       -- nullable; populated when the GitHub PR worker succeeds
    promotion_error  TEXT                        -- nullable; last PR-creation error for retry diagnosis
);

CREATE INDEX submissions_status_created
    ON submissions(status, created_at DESC);
```

Notes on the schema choices:

- **Public ID is UUIDv7**, generated in the Go process (the
  `crypto/rand`-based v7 generation lives in `pkg/atlas/idgen` so
  it's testable without touching the DB). The integer `id` is for
  storage efficiency and never appears on the wire.
- **`payload_json`** holds the full POSTed body verbatim — handles
  schema evolution without forcing a migration whenever the
  Submission shape changes.
- **`promotion_pr_url` + `promotion_error`** capture the GitHub PR
  worker's outcome out-of-band from the `status` field, so a
  PR-creation failure doesn't un-approve the row.

### Volume

```sh
flyctl volumes create atlas_data --size 1 --region iad -a urbanist-atlas
```

The volume mounts at `/data` (configured in `fly.toml`).
`URBANIST_DB_PATH=/data/atlas.db` is the default; an env override
exists for tests and CLI use.

### Backups

A new (replaces the deleted `.github/workflows/backup.yml`) nightly
cron:

```yaml
- name: Snapshot
  run: |
    flyctl ssh console -a urbanist-atlas \
      -C "sh -c 'sqlite3 /data/atlas.db .dump | gzip -c'" \
      > "atlas-$(date -u +%Y-%m-%d).sql.gz"
- name: Upload to R2
  # … same as the historical backup workflow
```

Bucket name: keep `urbanist-atlas-backups` (we already own it).
Retention: 30 days at the bucket level. Restore is `gunzip | sqlite3`
into a fresh DB — documented in `docs/deploy.md`.

## Wire contract

`openapi.yaml` gains four endpoints. All under `/api/v1/`. Success
shapes are `{meta, data}` consistent with the rest of the API; error
shapes are RFC 9457 problem documents.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST`  | `/api/v1/submissions` | client-secret + per-IP rate limit | Public: submit an org for review. Returns `{id, status: "pending"}`. |
| `GET`   | `/api/v1/admin/submissions` | bearer (`URBANIST_ADMIN_TOKEN`) | Paginated list. Query params: `status`, `limit`, `cursor`. |
| `POST`  | `/api/v1/admin/submissions/{id}/approve` | bearer | Marks approved; queues a GitHub PR. Returns `{status: "approved", promotion_pr_url?}`. |
| `POST`  | `/api/v1/admin/submissions/{id}/reject` | bearer | Marks rejected with reason. |

The Submission schema in `components/schemas/Submission` mirrors
the `[[org]]` TOML shape: `slug`, `name`, `short_desc`,
`website_url`, optional `contact_url`, `tags[]`, `region_slugs[]`,
plus the submitter metadata. Validation runs server-side against
the same rules `seedfiles.validateOrgs` already enforces on the
seed file.

Rate limiting on `POST /submissions` is per-IP in-process for v1
(simple sliding window), tightened to "a handful per hour per IP".
Cloudflare's WAF rate-limit rule sits in front for additional
defense.

## Approval flow

Admin endpoints flip the row's `status` and kick off the GitHub PR
worker. The PR worker is async — approval succeeds even if GitHub
is unreachable; the row stays `approved` with `promotion_error`
populated, and a manual `urbanist-atlas-server submissions retry-pr
--id=<uuid>` re-queues it.

### GitHub PR worker

Lives in `internal/githubpr/`. Uses a **fine-grained personal access
token** scoped to *this repo only*, with Contents (read/write) +
Pull requests (read/write). Token from env
`URBANIST_GITHUB_TOKEN`, set as a Fly secret.

On approval:

1. Fetch current `api/seed/orgs.toml` via GitHub Contents API.
2. Marshal the approved submission's payload into a `[[org]]` block
   (using `pelletier/go-toml/v2` for consistent formatting) and
   append.
3. Create branch `submission/<short-id>` and commit the new file.
4. Open a PR titled `Add <org name>` with the submission metadata
   (submitter, note, public id, timestamp) in the body.
5. Persist the PR URL onto the submission row.

Failure handling:

- Network error / 5xx from GitHub → log + persist `promotion_error`,
  return success to the admin caller (approval is durable, the PR
  is just queued).
- 4xx (auth / permissions) → log loudly + persist error. The
  retry CLI can re-run after a token rotation.
- Concurrent approvals are serialized in the worker (channel of 1)
  so two near-simultaneous merges can't produce conflicting
  branches.

### Why a PR (not a direct commit to main)

Keeping a human in the merge loop preserves the editorial-review
gate. Direct commits would skip the maintainer's chance to fix
formatting, copy-edit the description, or pull tags into the
canonical vocabulary before the org lands. The PR is the
review surface.

## Files to add / modify

### New

- `api/sqlc.yaml` — sqlite engine block + `queries/submissions.sql`
- `api/internal/store/sqlite/store.go`
- `api/internal/store/sqlite/gen/` — sqlc output (committed)
- `api/internal/store/sqlite/queries/submissions.sql`
- `api/migrations-sqlite/0001_init.sql` (+ `embed.go` for embedded migration FS)
- `api/internal/httpapi/submissions.go` + `submissions_test.go`
- `api/internal/githubpr/` — PR worker (+ tests with the GitHub API faked)
- `api/pkg/atlas/idgen/` — UUIDv7 + tests
- `.github/workflows/backup-sqlite.yml`

### Modified

- `api/openapi.yaml` — submission endpoints + Submission schema
- `api/internal/httpapi/openapi.yaml` — mirrored via `go generate`
- `web/src/lib/api.gen.ts` — regenerated
- `api/cmd/server/serve.go` — open the SQLite store, run migrations
  at boot, plumb it through to the new handlers
- `fly.toml` — volume mount, `URBANIST_DB_PATH`,
  `URBANIST_GITHUB_TOKEN`
- `Dockerfile` — ensure `/data` mountpoint exists
- `docs/deploy.md` — bring-up step for the volume + the GitHub token
  secret; backups runbook
- `CLAUDE.md` — §Hosting: mention the SQLite-on-volume sidecar to
  the read path

### Reuses

- chi router patterns from existing handlers
- `internal/httpapi/clientsecret.go` middleware (the Phase 1 shared
  secret); admin endpoints additionally check the bearer header
- `internal/httpapi/problem.go` — RFC 9457 error envelope
- goose (sqlite dialect) for migration application
- sqlc workflow + codegen — same as the retired Postgres adapter
- `pelletier/go-toml/v2` for the org-entry TOML formatting

## Verification

1. `go test ./...` — including a new
   `internal/store/sqlite/submissions_test.go` that opens an
   in-memory SQLite (`file::memory:?cache=shared`) and exercises
   create / list / approve / reject.
2. **End-to-end on a dev machine.** Web submission form → POST
   `/api/v1/submissions` → row in dev SQLite → admin GET lists it
   → admin POST approve → in a CI staging setup, a GitHub PR
   appears on a test branch of `urbanist-atlas-staging` (the staging
   repo, to avoid noise on the real one during the dev loop).
3. **GitHub PR worker integration test.** Stub the GitHub API with
   `httptest.NewServer`; assert the worker creates a branch,
   appends a correctly-formatted `[[org]]` block, and opens a PR
   with the expected title/body.
4. Migration applied cleanly on a fresh Fly volume; subsequent
   boots re-use existing data.
5. Backup workflow uploads a non-empty `.db.gz` to R2; restore
   roundtrip works.
6. RFC 9457 problem responses for: missing required field, unknown
   submission id, malformed JSON, bearer-token rejection,
   rate-limit exceedance.
7. **Cost check:** `fly status` still shows one app, one volume;
   monthly estimate stays at ~$5 (single shared-cpu-1x machine +
   1 GiB volume).

## Non-goals

- **Reintroducing a relational store.** If geospatial (PostGIS) or
  full-text search becomes a real requirement, SQLite + FTS5 is
  the first move; managed Postgres is the second. Neither is on
  the roadmap.
- **Cloudflare-edge writes.** Moving the write surface to a Worker
  + D1/Durable Object was considered ([context in the
  brainstorming for slice α](#)) and rejected for v1: it splits
  the codebase across Go + TS for limited gain at this scale.
- **Auto-importing approved submissions into the running binary
  without a deploy.** The PR-merge → deploy cycle is the freshness
  contract. The site updates when editorial review approves the
  PR; no live mutation.
- **Email notification for submitters.** Out of scope for the
  storage slice; can layer on later (transactional email via a
  Workers-side queue + a third-party).

## Open questions for execution

- **TOML formatting of new org entries.** `pelletier/go-toml/v2`'s
  marshal is consistent but doesn't preserve comments. The
  alternative is a hand-rolled template emitter that respects
  surrounding `# comment` blocks. The cheap default is the
  marshal; revisit if maintainers complain about lost editorial
  marginalia.
- **UUIDv7 vs ULID for the public id.** Both work; UUIDv7 has
  better library support in Go (`google/uuid` v1.7+ supports it).
  Default to UUIDv7 unless a reason emerges.
- **CI staging repo or staging branch?** The GitHub PR worker
  needs a target it can hit during integration tests without
  polluting the real PR list. A `urbanist-atlas-staging` repo
  (forkable) is the cleanest; a `staging/` branch on the real
  repo is simpler but noisier. Pick before implementation.

## Risk / rollback

- **Reversible:** the SQLite volume can be snapshotted before any
  destructive change (`flyctl ssh console -C "sqlite3 /data/atlas.db
  .backup /data/atlas.db.bak"`). The GitHub PR token is
  scope-revocable.
- **Forward compatibility:** if SQLite ever stops fitting (e.g. a
  feature demands real concurrent multi-writer access), the row
  set is small enough that a `pg_dump`-equivalent migration to
  managed Postgres is a single afternoon's work.
- **Approval rollback:** an accidentally-approved submission is
  recoverable in two ways — close the auto-PR without merging, and
  flip the row back to `pending` with a `urbanist-atlas-server
  submissions reopen --id=<uuid>` CLI. Both no-ops if the PR has
  already merged (the org is now in `orgs.toml`; revert via git).
