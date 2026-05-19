# Heroku Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Pivot Urbanist Atlas's Phase 1 hosting from Fly Managed Postgres to Heroku (Basic dyno + Postgres Essential-0, `us` region) per the decision in `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md`, replacing the in-flight PR #11 with a fresh Heroku-targeted PR.

**Architecture:** Single Heroku app, buildpack-based deploys (`heroku/go`), release-phase migrations via Procfile, Postgres add-on auto-managing `DATABASE_URL`. The connection-string env variable is renamed across the codebase from `URBANIST_DB_URL` to `DATABASE_URL` (no fallback) so Heroku credential rotations Just Work and the binary stays portable to any managed-Postgres host.

**Tech Stack:** Heroku Common Runtime (us), `heroku/go` buildpack, Heroku Postgres Essential-0, urfave/cli (existing), Cloudflare Pages (existing, slice #21).

**Spec:** `docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`.

---

## File Structure

### New files

- `Procfile` — at repo root. Two-line file declaring `release` (migrations) + `web` (server). Replaces `fly.toml`'s release_command + http_service config.
- `docs/superpowers/plans/2026-05-18-heroku-deploy-implementation.md` — this plan.

### Modified files

- `api/cmd/server/migrate.go`, `serve.go`, `seed.go`, `loadregions.go`, `loadpostal.go` — flip `cli.EnvVars("URBANIST_DB_URL")` → `cli.EnvVars("DATABASE_URL")` + matching error-message text.
- `api/cmd/server/main.go` — register the `loaddata` subcommand (cherry-picked from PR #11).
- `api/cmd/server/loaddata.go` — NEW from PR #11 (with `URBANIST_DB_URL` → `DATABASE_URL` already applied at port time).
- `api/internal/loaddata/loaddata.go` — NEW from PR #11.
- `api/internal/store/postgres/loaddata_test.go` — NEW from PR #11.
- `justfile` — drop the `fly-*` group, add a `heroku-*` group + `db-backup`, update the `loaddata` recipe to delegate to the binary subcommand, and rewrite the dev-Postgres header comment that mentions `URBANIST_DB_URL`.
- `mise.development.toml` — rename `URBANIST_DB_URL` → `DATABASE_URL` and the comment that names it.
- `mise.local.toml.example` — rename in the commented-out example.
- `CLAUDE.md` — §Hosting rewritten against Heroku; §Tech conventions env-var list updated; one-line addendum about `DATABASE_URL` being the universal-convention exception.
- `README.md` — drop "Fly Managed Postgres" from the `api/` summary bullet + rewrite §Deploy against Heroku.
- `api/README.md` — rename `URBANIST_DB_URL` in usage examples.
- `docs/roadmap.md` — slice #20 row rewritten against Heroku.
- `docs/deploy.md` — NEW (written end-to-end against Heroku; the slice-20 branch wrote a Fly-targeted version but it's not on `main` so this lands as a new file).
- `docs/superpowers/specs/2026-05-18-qa-deploy-design.md` — Architecture DB row + Slice #20 section rewritten; `URBANIST_DB_URL` mentions updated.
- `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` — Decision section filled in.

### Deleted files

- `fly.toml` — replaced by `Procfile`.
- `Dockerfile` — replaced by the `heroku/go` buildpack.

### Untouched (verify with grep at end)

- `pkg/atlas/*`, `api/internal/store/postgres/*` (except the new test), `api/internal/httpapi/*`, `api/migrations/*`, `api/openapi.yaml`, `api/seed/*` — none of this code reads `URBANIST_DB_URL` directly.
- `web/**` — Pages-deployed, unaffected.
- `.github/workflows/ci.yml` — local-CI env, not deploy.
- `mise.toml`, `mise.ci.toml` — only `mise.development.toml` has the dev DB URL.
- Every `pg-*` dev justfile recipe — they reference port `:55432` directly, not the env var.

---

## Task 1: Create the Heroku branch from `main`

**Files:**
- (no edits — branch creation only)

- [ ] **Step 1: Verify clean working tree on `main`**

```bash
git checkout main
git status
```

Expected: `nothing to commit, working tree clean`. If dirty, stash or commit before proceeding.

- [ ] **Step 2: Pull latest `main`**

```bash
git pull origin main
```

- [ ] **Step 3: Create and switch to the new branch**

```bash
git checkout -b slice-20-heroku-deploy
```

Expected: `Switched to a new branch 'slice-20-heroku-deploy'`.

---

## Task 2: Cherry-pick loaddata code from PR #11 (Go files only)

**Files:**
- Restore from `slice-20-fly-deploy-loaddata`: `api/cmd/server/loaddata.go`, `api/cmd/server/main.go`, `api/internal/loaddata/loaddata.go`, `api/internal/store/postgres/loaddata_test.go`

Per the spec, PR #11's commits mix loaddata-code changes with Fly-specific docs/config that we're discarding. Cleanest approach: restore the four Go files directly, leave `docs/deploy.md`, `fly.toml`, and `justfile` from PR #11 *alone* — we rewrite the first against Heroku in Task 9, delete the second in Task 7, and modify the third in Task 8.

- [ ] **Step 1: Restore the four Go files from the PR #11 branch**

```bash
git checkout slice-20-fly-deploy-loaddata -- \
    api/cmd/server/loaddata.go \
    api/cmd/server/main.go \
    api/internal/loaddata/loaddata.go \
    api/internal/store/postgres/loaddata_test.go
```

Expected: silent success; `git status` shows the four files staged for commit.

- [ ] **Step 2: Verify Go compiles**

```bash
cd api && go build ./... && cd ..
```

Expected: silent success. If failure cites missing imports, the cherry-pick is incomplete — re-inspect the PR #11 branch.

- [ ] **Step 3: Commit**

```bash
git add api/cmd/server/loaddata.go api/cmd/server/main.go \
    api/internal/loaddata/loaddata.go api/internal/store/postgres/loaddata_test.go
git commit -m "$(cat <<'EOF'
feat(loaddata): port loaddata subcommand from slice-20-fly branch

Brings forward the four loaddata Go files from PR #11
(slice-20-fly-deploy-loaddata) so they survive the Heroku pivot:

- api/cmd/server/loaddata.go — urfave/cli wrapper
- api/cmd/server/main.go — subcommand registration
- api/internal/loaddata/loaddata.go — orchestration (LoadAll +
  exported Countries() so the integration test stays in sync with
  the country list)
- api/internal/store/postgres/loaddata_test.go — testcontainers
  integration coverage (regions/postal/orgs populated, idempotent
  re-run)

PR #11's Fly-targeted docs/deploy.md, fly.toml release_command
change, and justfile fly-group polish are intentionally left
behind — Heroku replacements land in later commits on this branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 4: Run the existing test suite to confirm green baseline**

```bash
just api-test
```

Expected: all unit tests pass. (Integration tests are gated behind `//go:build integration` and run separately in Task 6.)

---

## Task 3: Verify baseline integration tests against `URBANIST_DB_URL`

Establish a known-green baseline *before* renaming, so any post-rename failure is unambiguously caused by the rename.

**Files:** (no edits)

- [ ] **Step 1: Bring up dev Postgres**

```bash
just pg-up
```

Expected: ends with `dev postgres ready on :55432 (db: urbanist_atlas_dev)`.

- [ ] **Step 2: Run integration tests**

```bash
just api-test-integration
```

Expected: all integration tests pass (including the new `TestPipeline_LoaddataLoadAll`). Duration is ~30–40 s — testcontainers boots its own Postgres so this doesn't depend on `pg-up`.

- [ ] **Step 3: Run full check suite**

```bash
just api-check
```

Expected: clean (vet, race, generated-no-diff). If any step fails here, the cherry-pick from Task 2 was incomplete or the working tree drifted from main.

---

## Task 4: Rename `URBANIST_DB_URL` → `DATABASE_URL` in Go code (5 files)

**Files:**
- Modify: `api/cmd/server/migrate.go` (env source on `--db-url` flag, error message)
- Modify: `api/cmd/server/serve.go` (same)
- Modify: `api/cmd/server/seed.go` (same)
- Modify: `api/cmd/server/loadregions.go` (same)
- Modify: `api/cmd/server/loadpostal.go` (same)

- [ ] **Step 1: Rename in `migrate.go`**

```bash
grep -n "URBANIST_DB_URL" api/cmd/server/migrate.go
```

Expected: two hits — one in `cli.EnvVars("URBANIST_DB_URL")` (the flag's env source), one in the "is required" error message.

Edit both occurrences:

```go
// Before
Sources: cli.EnvVars("URBANIST_DB_URL"),
// ...
return errors.New("migrate: --db-url or URBANIST_DB_URL is required")

// After
Sources: cli.EnvVars("DATABASE_URL"),
// ...
return errors.New("migrate: --db-url or DATABASE_URL is required")
```

- [ ] **Step 2: Repeat the rename in `serve.go`**

Same two-spot pattern (`cli.EnvVars(...)` and the error message). Use grep first to locate, then edit.

- [ ] **Step 3: Repeat in `seed.go`, `loadregions.go`, `loadpostal.go`**

Each has the same two-spot pattern. After all five files, run:

```bash
grep -rn "URBANIST_DB_URL" api/
```

Expected: no hits in any Go file. If hits remain, finish the rename before continuing.

- [ ] **Step 4: Verify Go compiles**

```bash
cd api && go build ./... && cd ..
```

Expected: silent success.

- [ ] **Step 5: Run integration tests**

```bash
just api-test-integration
```

Expected: clean. Integration tests use testcontainers and set the connection string directly via the test harness — they don't depend on `URBANIST_DB_URL` or `DATABASE_URL` being set in the env, so they should pass identically after the rename.

- [ ] **Step 6: Run unit + check suite**

```bash
just api-check
```

Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add api/cmd/server/migrate.go api/cmd/server/serve.go api/cmd/server/seed.go \
    api/cmd/server/loadregions.go api/cmd/server/loadpostal.go
git commit -m "$(cat <<'EOF'
refactor(cli): rename URBANIST_DB_URL → DATABASE_URL on every cli flag

The Postgres connection string follows the universal DATABASE_URL
convention every managed-Postgres host (Heroku, Fly MPG, Render,
Neon, Railway) sets automatically. The previous URBANIST_DB_URL
name was an instance of CLAUDE.md's URBANIST_*-prefix convention
applied to a value that isn't really app-specific — and on Heroku
specifically, the add-on rotates DATABASE_URL on credential
rotations, so a mirroring step would silently break the deploy on
every rotation.

All other config (URBANIST_ADMIN_TOKEN, URBANIST_CLIENT_SECRET,
URBANIST_PORT, URBANIST_LOG_FORMAT, URBANIST_STORE,
URBANIST_CORS_ORIGINS, URBANIST_SEED_DIR) keeps the URBANIST_*
prefix — this is a one-off exception for the connection string.

Touches the five urfave/cli flag definitions across serve / migrate
/ seed / loadregions / loadpostal. Integration tests unaffected
(testcontainers sets the conn string directly via the test harness).

CLAUDE.md convention update + dev tooling rename land in follow-up
commits on this branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Rename in dev tooling

**Files:**
- Modify: `mise.development.toml` (env entry + comment)
- Modify: `mise.local.toml.example` (commented-out example)
- Modify: `justfile` (dev-Postgres header comment that names the var)

- [ ] **Step 1: Update `mise.development.toml`**

Locate:

```toml
# Points at the docker-managed dev Postgres on :55432 (see the
# `pg-*` justfile recipes for lifecycle).
URBANIST_DB_URL = "postgres://urbanist:urbanist@localhost:55432/urbanist_atlas_dev?sslmode=disable"
```

Replace `URBANIST_DB_URL` with `DATABASE_URL`. Leave the comment block above otherwise unchanged.

- [ ] **Step 2: Update `mise.local.toml.example`**

Locate the commented-out example:

```toml
# [env]
# URBANIST_DB_URL      = "postgres://urbanist@localhost:5432/atlas?sslmode=disable"
# URBANIST_ADMIN_TOKEN = "my-personal-dev-token"
```

Rename `URBANIST_DB_URL` → `DATABASE_URL` (keep the comment-out, keep the indentation alignment).

- [ ] **Step 3: Update `justfile` header comment**

Locate the `pg-up` header comment (around line 167):

```
# Credentials and DB name are dev-only and match what
# mise.development.toml hands to URBANIST_DB_URL:
```

Replace `URBANIST_DB_URL` with `DATABASE_URL`.

- [ ] **Step 4: Verify mise + justfile parse**

```bash
mise env --json | grep DATABASE_URL
just --list 2>&1 | head -20
```

Expected: `DATABASE_URL` shows in mise's env output (proves the rename is loaded); `just --list` renders without parse errors. If `mise env` shows nothing for `DATABASE_URL`, make sure your shell has `MISE_ENV=development` set (per CLAUDE.md's local-dev guidance).

- [ ] **Step 5: Verify dev loop end-to-end with DATABASE_URL**

```bash
just pg-reset  # ensure clean DB
just migrate-up
just loaddata
```

Expected: all three succeed. Run a quick smoke against the local API:

```bash
just api-run &
SERVER_PID=$!
sleep 2
just lookup 10001 US
kill $SERVER_PID
```

Expected: `lookup` returns populated JSON for ZIP 10001.

- [ ] **Step 6: Commit**

```bash
git add mise.development.toml mise.local.toml.example justfile
git commit -m "$(cat <<'EOF'
refactor(dev): rename URBANIST_DB_URL → DATABASE_URL in dev tooling

Companion to the cli-flag rename: aligns the dev-loop env var with
the universal Postgres convention so `mise.development.toml`,
`mise.local.toml.example`, and the `pg-up` header comment in the
justfile all use DATABASE_URL.

The pg-* recipes themselves don't reference the env var
(they wire :55432 directly), so they're untouched. CLAUDE.md
convention note lands in a follow-up doc commit on this branch.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Delete `fly.toml` + `Dockerfile`, add `Procfile`

**Files:**
- Delete: `fly.toml`
- Delete: `Dockerfile`
- Create: `Procfile` (at repo root)

- [ ] **Step 1: Delete the Fly artifacts**

```bash
git rm fly.toml Dockerfile
```

Expected: both files staged for deletion.

- [ ] **Step 2: Create `Procfile`**

At the repo root (`/Users/mrossi/dev/urbanist-atlas/Procfile`):

```
release: urbanist-atlas-server migrate up
web: urbanist-atlas-server serve --port=$PORT
```

Two lines, no trailing whitespace, no blank lines. (Heroku Procfile parser is whitespace-tolerant, but the canonical form is the cleanest.)

- [ ] **Step 3: Verify Go code doesn't reference the deleted files**

```bash
grep -rn "fly.toml\|Dockerfile" --include="*.go" .
```

Expected: no hits. (The Go code never referenced these; this is a paranoia check.)

- [ ] **Step 4: Commit**

```bash
git add Procfile
git commit -m "$(cat <<'EOF'
feat(heroku): replace fly.toml + Dockerfile with Procfile

The Phase 1 dogfood deploys to Heroku per the slice #19.5 hosting
decision (docs/superpowers/specs/2026-05-18-hosting-cost-spike.md
+ 2026-05-18-heroku-deploy-design.md). Heroku uses the
heroku/go buildpack to compile the Go binary from go.mod, so the
slice-#19 Dockerfile becomes dead weight and is deleted alongside
fly.toml.

Procfile declares:
- release: runs `migrate up` in a release dyno on every deploy,
  blocking traffic flip on migration failure (same guarantee
  fly.toml's release_command gave us)
- web: long-running server bound to Heroku's injected $PORT

loaddata stays a one-off via `heroku run urbanist-atlas-server
loaddata`, not a Procfile entry.

The heroku-* justfile recipes and the docs/deploy.md rewrite land
in follow-up commits.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Replace `fly-*` justfile recipes with `heroku-*` + `db-backup`, update `loaddata` recipe

**Files:**
- Modify: `justfile`

- [ ] **Step 1: Locate the `fly-*` recipes**

```bash
grep -n "\[group('fly')\]" justfile
```

Expected: five hits — `fly-deploy`, `fly-status`, `fly-logs`, `fly-secrets`, `fly-ssh` (the recipes themselves are on the line after each `[group('fly')]` decorator).

- [ ] **Step 2: Delete the entire `fly` group block**

Edit `justfile`. Remove the `# ── fly: deploy & ops ─────────────────────────────────` section header (if present) and all five recipes + their preceding `[group('fly')]` decorators. The section currently sits between the `loaddata` recipe block and the `# ── smoke:` group; verify by reading the surrounding lines.

After deletion the `loaddata` recipe should be followed directly by the `# ── postgres: dev container lifecycle ───` section (which is where it was originally before slice #19).

Actually wait: per the file inspection in Task 5, the `fly` group is *between* the `postgres` and `smoke` groups, not after `loaddata`. Verify by re-reading the section boundaries before deleting, so you don't accidentally remove a neighbouring group.

- [ ] **Step 3: Add the `heroku` group**

Add this block in the same location the `fly` group occupied (between `postgres` and `smoke` groups; matches the original layout intent):

```
# ── heroku: deploy & ops ──────────────────────────────

# build + push current branch to Heroku (release phase runs migrations)
[group('heroku')]
heroku-deploy:
    git push heroku main

# tail live Heroku logs
[group('heroku')]
heroku-logs:
    heroku logs --tail -a urbanist-atlas

# list app config (names + masked values)
[group('heroku')]
heroku-config:
    heroku config -a urbanist-atlas

# open an interactive shell inside a one-off dyno
[group('heroku')]
heroku-ssh:
    heroku run bash -a urbanist-atlas

# seed the live database (regions → postal → orgs)
[group('heroku')]
heroku-loaddata:
    heroku run urbanist-atlas-server loaddata -a urbanist-atlas

# capture an on-demand Postgres backup; tail of `heroku pg:backups` shows retention
[group('heroku')]
db-backup:
    heroku pg:backups:capture -a urbanist-atlas
```

- [ ] **Step 4: Update the justfile group-list header comment**

Locate the header comment near the top:

```
# Groups: api, data, postgres, web, smoke, ci. Each group corresponds
# to a section comment below.
```

Replace with:

```
# Groups: api, data, postgres, web, heroku, smoke, ci. Each group
# corresponds to a section comment below.
```

(`fly` is removed; `heroku` is added.)

- [ ] **Step 5: Verify the justfile parses and `just --list` renders cleanly**

```bash
just --list
```

Expected: groups appear in declaration order, `fly-*` recipes are gone, `heroku-*` recipes + `db-backup` are present and grouped under `heroku`. No parse errors.

- [ ] **Step 6: Commit**

```bash
git add justfile
git commit -m "$(cat <<'EOF'
chore(justfile): replace fly-* recipes with heroku-* + db-backup

Drops the slice-#19 `fly` group (5 recipes) and replaces it with a
`heroku` group of equivalent verbs:

  fly-deploy  → heroku-deploy   (`git push heroku main`)
  fly-logs    → heroku-logs     (`heroku logs --tail`)
  fly-secrets → heroku-config   (`heroku config`, since Heroku
                                 doesn't distinguish secrets and
                                 plain config the way Fly does)
  fly-status  → (dropped; `heroku ps` is not in muscle memory the
                 way `flyctl status` was — re-add if needed)
  fly-ssh     → heroku-ssh      (`heroku run bash`)

Also adds:
- heroku-loaddata: wraps `heroku run urbanist-atlas-server loaddata`
  so the one-off seed-on-deploy step is muscle-memory.
- db-backup: wraps `heroku pg:backups:capture` for the maintainer's
  ad-hoc snapshot habit; Heroku Postgres Essential-0's continuous
  WAL + scheduled backups cover the steady-state case.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Update the `loaddata` justfile recipe to delegate to the binary

Per PR #11's design, the dev `just loaddata` recipe and the deploy `heroku run urbanist-atlas-server loaddata` step run the same code path. The recipe on `main` still does the original seven-step shell chain.

**Files:**
- Modify: `justfile` (the `loaddata` recipe in the `data` group)

- [ ] **Step 1: Locate the current `loaddata` recipe**

```bash
grep -n -A4 "^loaddata:" justfile
```

Expected: the recipe shells out to `just loadregions ... && just loadpostal ... && just seed`.

- [ ] **Step 2: Replace with the binary-delegating version**

Replace the existing `loaddata` recipe with the slice-#11 form:

```
# load all bundled fixtures in the right order:
# regions first (so leaf slugs resolve), then postal codes, then orgs.
# Wraps the `loaddata` binary subcommand so dev runs go through the
# exact same orchestration the Heroku deploy uses
# (heroku run urbanist-atlas-server loaddata). The country list
# lives in api/internal/loaddata/loaddata.go — add new countries
# there, not here.
[group('data')]
[doc('load every bundled fixture in dependency order (regions → postal → orgs)')]
loaddata:
    cd api && go run ./cmd/server loaddata
```

(The header comment is identical to PR #11's, with "Fly deploy uses (flyctl ssh console …)" replaced by "Heroku deploy uses (heroku run …)".)

- [ ] **Step 3: Verify the recipe works**

```bash
just pg-reset
just migrate-up
just loaddata
```

Expected: regions + postal + orgs loaded for US, CA, and PT (the three bundled countries). Same output as the seven-step chain.

- [ ] **Step 4: Commit**

```bash
git add justfile
git commit -m "$(cat <<'EOF'
refactor(justfile): delegate `just loaddata` to the binary subcommand

Brings the on-main `just loaddata` recipe in line with the
`urbanist-atlas-server loaddata` subcommand ported in the earlier
commit on this branch. Dev and deploy now run the same code path:
`just loaddata` locally, `heroku run urbanist-atlas-server loaddata`
on Heroku.

Recipe header comment is updated to reference the Heroku invocation
instead of the now-discarded Fly equivalent.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Rewrite `docs/deploy.md` end-to-end against Heroku

The slice-20 branch's `docs/deploy.md` is Fly-targeted and discarded. We write a fresh `docs/deploy.md` against Heroku, following the same section structure (so slice #21's append in PR #12 still slots in cleanly).

**Files:**
- Create: `docs/deploy.md` (at repo root under `docs/`)

The doc structure mirrors the Fly version's section headings so PR #12's "Cloudflare Pages + DNS" appendix slots in unchanged:

1. Front matter (intro, link to design)
2. Launch chunk slice table (updated for Heroku)
3. Hosting topology
4. Prerequisites
5. Heroku: API + database (steps 1–6)
6. Ongoing operations
7. Secrets (incl. rotation procedure)
8. QA → prod transition (future)
9. Troubleshooting

- [ ] **Step 1: Write `docs/deploy.md`**

Create the file with the exact content below. (Headings, commands, env-var names, and rotation procedure are all spec-derived.)

````markdown
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
````

- [ ] **Step 2: Verify the file renders**

```bash
wc -l docs/deploy.md
grep -c "^## " docs/deploy.md
```

Expected: line count > 200; section heading count = 8 (Hosting topology, Prerequisites, Heroku: API + database, Ongoing operations, Secrets, QA → prod transition, Troubleshooting — plus the implicit lead intro).

- [ ] **Step 3: Commit**

```bash
git add docs/deploy.md
git commit -m "$(cat <<'EOF'
docs(deploy): runbook for Heroku Basic + Postgres Essential-0

End-to-end operational guide for taking the project from a clean
Heroku + Cloudflare account to a working QA deployment. Replaces
the Fly-targeted runbook that PR #11 wrote on the
slice-20-fly-deploy-loaddata branch (closed by this PR).

Section structure mirrors the Fly runbook so slice #21's
"Cloudflare Pages + DNS" appendix (PR #12) slots in unchanged
after a rebase.

Sections: Hosting topology, Prerequisites, Heroku: API + database
(create app, provision Essential-0, set config + secrets, first
deploy, seed, smoke, backup schedule), Ongoing ops, Secrets
(incl. rotation), QA → prod transition, Troubleshooting.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Update `CLAUDE.md` (§Hosting + §Tech conventions)

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Update §Hosting**

Locate:

```
## Hosting

- **API:** Fly.io. Single Dockerfile, single binary, Fly Managed Postgres.
- **Web:** Cloudflare Pages connected to `web/`. PR preview deploys per
  branch.
```

Replace the `- **API:**` bullet with:

```
- **API:** Heroku Common Runtime (region `us`, Virginia). Heroku/go
  buildpack compiles the binary from `api/cmd/server`; Heroku Postgres
  Essential-0 add-on provides the database. `Procfile` declares the
  `release` (migrations) and `web` (serve) processes. See
  [`docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`](./docs/superpowers/specs/2026-05-18-heroku-deploy-design.md)
  for the design and
  [`docs/deploy.md`](./docs/deploy.md) for the runbook.
```

Leave the `- **Web:**` bullet unchanged.

- [ ] **Step 2: Update §Tech conventions env-var list**

Locate (around line 80):

```
- **Config:** all via urfave/cli flags with env-var fallbacks
  (`URBANIST_DB_URL`, `URBANIST_ADMIN_TOKEN`, `URBANIST_PORT`, etc.). No `viper`.
```

Replace with:

```
- **Config:** all via urfave/cli flags with env-var fallbacks
  (`URBANIST_ADMIN_TOKEN`, `URBANIST_PORT`, `URBANIST_LOG_FORMAT`,
  `URBANIST_CORS_ORIGINS`, `URBANIST_STORE`, `URBANIST_SEED_DIR`,
  etc.). The Postgres connection string is the one exception:
  follows the universal `DATABASE_URL` convention (every managed-
  Postgres host — Heroku, Fly MPG, Render, Neon, Railway — sets
  this name automatically). No `viper`.
```

- [ ] **Step 3: Update the `serve` flag description (around line 94)**

Locate:

```
`serve` accepts `--store=memory|postgres` (postgres default) and
`--db-url` (with `URBANIST_DB_URL` env fallback). The memory store
stays available for tests and offline CLI use.
```

Change `URBANIST_DB_URL` to `DATABASE_URL`:

```
`serve` accepts `--store=memory|postgres` (postgres default) and
`--db-url` (with `DATABASE_URL` env fallback). The memory store
stays available for tests and offline CLI use.
```

- [ ] **Step 4: Audit for remaining `URBANIST_DB_URL` or `Fly Managed Postgres` mentions in CLAUDE.md**

```bash
grep -n "URBANIST_DB_URL\|Fly Managed Postgres\|fly.toml\|Dockerfile" CLAUDE.md
```

Expected: no hits. If any remain, fix them.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(claude.md): point Hosting + Config conventions at Heroku

- §Hosting: Fly.io → Heroku Common Runtime (us, Virginia), Postgres
  Essential-0 add-on, Procfile-driven deploys. Points at the new
  Heroku design doc + runbook.
- §Tech conventions: drop URBANIST_DB_URL from the env-var list;
  add one-line note that the Postgres connection string follows
  the universal DATABASE_URL convention as the single exception
  to the URBANIST_* prefix rule.
- `serve` flag description: URBANIST_DB_URL → DATABASE_URL fallback.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Update `README.md` + `api/README.md`

**Files:**
- Modify: `README.md` (the `api/` summary bullet + §Deploy)
- Modify: `api/README.md` (env-var usage examples)

- [ ] **Step 1: Update the `api/` summary bullet in `README.md`**

Locate (around line 19):

```
- **[`api/`](./api)** — Go service (chi + sqlc + goose + Fly Managed Postgres),
```

Replace `Fly Managed Postgres` with `Heroku Postgres Essential-0`:

```
- **[`api/`](./api)** — Go service (chi + sqlc + goose + Heroku Postgres Essential-0),
```

- [ ] **Step 2: Rewrite the §Deploy section of `README.md`**

Locate (around line 70):

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
```

Replace with:

```
### Deploy

The API ships via the `heroku/go` buildpack to Heroku (region `us`,
Virginia, Common Runtime) backed by Heroku Postgres Essential-0;
`Procfile` at the repo root declares release-phase migrations + the
web process. The web SPA deploys to Cloudflare Pages from `web/`.
Initial provisioning steps (creating the Heroku app, attaching the
Postgres add-on, wiring DNS, setting secrets) are documented in
[`docs/deploy.md`](./docs/deploy.md) — see slice #20 / #21 in the
[roadmap](./docs/roadmap.md) for status. Ongoing ops use the
`heroku-*` recipes (`just heroku-deploy`, `just heroku-logs`,
`just heroku-config`, `just heroku-ssh`, `just heroku-loaddata`,
`just db-backup`).

The hosting decision behind the Heroku choice is documented at
[`docs/superpowers/specs/2026-05-18-hosting-cost-spike.md`](./docs/superpowers/specs/2026-05-18-hosting-cost-spike.md)
and
[`docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`](./docs/superpowers/specs/2026-05-18-heroku-deploy-design.md).
The full chunk design (slices #19/#20/#21/#23) lives at
[`docs/superpowers/specs/2026-05-18-qa-deploy-design.md`](./docs/superpowers/specs/2026-05-18-qa-deploy-design.md).
```

- [ ] **Step 3: Audit `api/README.md` for URBANIST_DB_URL**

```bash
grep -n "URBANIST_DB_URL" api/README.md
```

If any hits, rename each to `DATABASE_URL` in place. (The current grep showed at most a couple of usage-example lines.)

- [ ] **Step 4: Audit `README.md` for any remaining Fly references**

```bash
grep -n "Fly\|fly\.toml\|flyctl" README.md
```

Expected: no hits (the §Deploy rewrite removes the last ones). If any remain, fix them.

- [ ] **Step 5: Commit**

```bash
git add README.md api/README.md
git commit -m "$(cat <<'EOF'
docs(readme): repoint README + api/README at Heroku

- README.md: `api/` summary bullet now names Heroku Postgres
  Essential-0; §Deploy rewritten against the heroku/go buildpack +
  Procfile + heroku-* justfile recipes; links the cost-spike + Heroku
  design docs alongside the existing qa-deploy-design link.
- api/README.md: usage examples renamed URBANIST_DB_URL → DATABASE_URL
  per the cli-flag refactor.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Update `docs/roadmap.md` slice #20

**Files:**
- Modify: `docs/roadmap.md`

- [ ] **Step 1: Locate slice #20**

```bash
grep -n "^| 20 \|^| \*\*20\*\*" docs/roadmap.md
```

Expected: a single line around row 172 with the current Fly-MPG-flavored text.

- [ ] **Step 2: Replace the slice #20 row's "what it ships" cell**

The current cell reads (roughly):

```
| 20 | **Production Postgres + first deploy** | Provision the database target chosen in slice #19.5, set `URBANIST_DB_URL` + `URBANIST_ADMIN_TOKEN` + `URBANIST_CLIENT_SECRET` as Fly secrets, run the first `flyctl deploy` (release_command runs migrations), seed via `urbanist-atlas-server loaddata`. The runbook in `docs/deploy.md` is written against the chosen target — *not* hardcoded to Fly Managed Postgres until/unless #19.5 lands there. |
```

Replace with:

```
| 20 | **Heroku deploy + Postgres Essential-0** | Create the Heroku app (region `us`, Common Runtime, `heroku/go` buildpack), provision Heroku Postgres Essential-0 add-on (auto-sets `DATABASE_URL`), set `URBANIST_ADMIN_TOKEN` + `URBANIST_CLIENT_SECRET` + non-secret config via `heroku config:set`, push to deploy (release-phase Procfile entry runs migrations), seed via `heroku run urbanist-atlas-server loaddata`. Runbook in `docs/deploy.md` (rewritten end-to-end for Heroku per slice #19.5's decision). PR #11 (the slice-20-fly-deploy-loaddata branch) is closed; this slice lands on `slice-20-heroku-deploy`. |
```

- [ ] **Step 3: Verify no stale `Fly Managed Postgres` mentions in slice rows**

```bash
grep -n "Fly Managed Postgres\|MPG\|URBANIST_DB_URL\|flyctl" docs/roadmap.md
```

Expected: at most one or two hits in the slice #19.5 row (which references "Fly Managed Postgres" as the *thing being evaluated* by the spike — fine to leave as historical context). Any other hits need updating.

- [ ] **Step 4: Commit**

```bash
git add docs/roadmap.md
git commit -m "$(cat <<'EOF'
docs(roadmap): rewrite slice #20 against the Heroku decision

The slice #19.5 hosting spike chose Heroku Basic + Postgres
Essential-0 over Fly Managed Postgres. Slice #20's "what it ships"
cell now reflects that: create the Heroku app + buildpack,
provision the Essential-0 add-on, push to deploy, seed via
`heroku run`. Names the closed PR #11 (Fly branch) so the roadmap
audit-trail is intact.

Slice #19.5's row keeps its "Fly Managed Postgres" mention as
historical context — the spike was *about* verifying MPG pricing.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Update `docs/superpowers/specs/2026-05-18-qa-deploy-design.md`

**Files:**
- Modify: `docs/superpowers/specs/2026-05-18-qa-deploy-design.md`

- [ ] **Step 1: Update the Architecture DB row**

Locate (around line 54):

```
| DB | Fly Managed Postgres `urbanist-atlas-db` | (private to the Fly app) |
```

Replace with:

```
| DB | Heroku Postgres Essential-0 add-on | (private to the Heroku app via `DATABASE_URL`) |
```

- [ ] **Step 2: Rewrite the "Slice #20 — Fly MPG + first deploy" section header + body**

Locate (around line 147):

```
#### Slice #20 — Fly MPG + first deploy (operational + optional code)

After #19 merges. Provisions Managed Postgres, sets secrets,
deploys, seeds the dataset. Mostly `flyctl` CLI invocations; one
small optional code change.

Deliverables:

- New runbook at `docs/deploy.md` documenting the operational
  steps (auth, launch, MPG create + attach, secrets set, deploy,
  seed).
- Three Fly secrets staged:
  - `URBANIST_DB_URL` (required) — manual mapping from MPG's
    `DATABASE_URL` because the binary reads our env name.
  - `URBANIST_ADMIN_TOKEN` (Phase 2 pre-stage; no-op until admin
    endpoints land).
  - `URBANIST_CLIENT_SECRET` (required by slice #23 in prod).
- Optional `urbanist-atlas-server loaddata` subcommand
  (`api/cmd/server/loaddata.go`) wrapping the existing
  loadregions / loadpostal / seed chain in dependency order, so
  every redeploy seed step is a single
  `flyctl ssh console -C "urbanist-atlas-server loaddata"`. The
  existing chain lives in `justfile:139-146`; the Go subcommand
  mirrors it. Recommended in.
```

Replace with:

```
#### Slice #20 — Heroku deploy + Postgres Essential-0 (operational + code rename)

After #19.5 (the cost-spike decision) lands. Provisions the Heroku
app + Postgres Essential-0 add-on, sets secrets + non-secret
config, deploys via `git push heroku main`, seeds the dataset.
Mostly `heroku` CLI invocations; one cross-cutting code rename
(`URBANIST_DB_URL` → `DATABASE_URL` on the urfave/cli flags) so
the binary aligns with the de-facto managed-Postgres convention.

The full Heroku design is at
[`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md);
implementation plan at
[`../plans/2026-05-18-heroku-deploy-implementation.md`](../plans/2026-05-18-heroku-deploy-implementation.md).

Deliverables:

- New runbook at `docs/deploy.md` documenting the operational
  steps (auth, `heroku create` + buildpack + add-on, config + secrets,
  deploy, seed, backup schedule, smoke).
- Two app-level secrets staged via `heroku config:set`:
  - `URBANIST_ADMIN_TOKEN` (Phase 2 pre-stage; no-op until admin
    endpoints land).
  - `URBANIST_CLIENT_SECRET` (required by slice #23 in prod).
- `DATABASE_URL` is **not** manually set — the Heroku Postgres
  add-on owns it and rotates it automatically.
- `urbanist-atlas-server loaddata` subcommand
  (`api/cmd/server/loaddata.go`) — ported forward from the closed
  slice-20-fly-deploy-loaddata branch (PR #11). Wraps the existing
  loadregions / loadpostal / seed chain so every redeploy seed
  step is a single `heroku run urbanist-atlas-server loaddata`.
  The dev `just loaddata` recipe delegates to the same binary
  subcommand for parity.
- Procfile at the repo root: `release` runs migrations, `web` runs
  serve. Replaces the slice-#19 `fly.toml` + `Dockerfile`, which
  are deleted in this slice.
- Connection-string env-var rename: every Postgres-touching
  subcommand (`serve`, `migrate`, `seed`, `loadregions`,
  `loadpostal`) flips its urfave/cli flag's env source from
  `URBANIST_DB_URL` to `DATABASE_URL`. `mise.development.toml`,
  `mise.local.toml.example`, the `pg-up` header comment in
  `justfile`, `api/README.md`, and CLAUDE.md's convention list
  all rename in lockstep.
```

- [ ] **Step 3: Audit for any remaining `URBANIST_DB_URL` or MPG mentions in the spec**

```bash
grep -n "URBANIST_DB_URL\|Fly Managed Postgres\|MPG\|flyctl mpg" docs/superpowers/specs/2026-05-18-qa-deploy-design.md
```

Expected: no hits (or only a "superseded by" note if you added one — none was specified in the design, so no hits is the target).

- [ ] **Step 4: Add a status banner at the top noting the supersession**

Locate the existing front matter (around line 3):

```
**Status:** Active — implementation of the Phase 1 launch chunk that
takes Urbanist Atlas from "works on localhost" to "live at QA URLs."
**Supersedes:** none.
```

Insert a `**Superseded-in-part-by:**` line after `**Supersedes:**`:

```
**Status:** Active — implementation of the Phase 1 launch chunk that
takes Urbanist Atlas from "works on localhost" to "live at QA URLs."
**Supersedes:** none.
**Superseded-in-part-by:** [`2026-05-18-hosting-cost-spike.md`](./2026-05-18-hosting-cost-spike.md)
+ [`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md)
— the DB row in the architecture table and the Slice #20 section are
rewritten against Heroku Postgres Essential-0 instead of Fly Managed
Postgres. Slices #19 / #21 / #23 are unaffected (slice #19's
`Dockerfile` + `fly.toml` are deleted by the Heroku pivot).
```

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-05-18-qa-deploy-design.md
git commit -m "$(cat <<'EOF'
docs(spec): supersede MPG references with Heroku in qa-deploy-design

The slice #19.5 hosting decision pivoted from Fly Managed Postgres
to Heroku. This commit threads the supersession through the
qa-deploy-design spec:

- Architecture DB row: MPG → Heroku Postgres Essential-0.
- Slice #20 section: rewritten end-to-end against `git push heroku
  main` + `heroku-postgresql:essential-0` add-on, with pointers to
  the Heroku design doc and implementation plan. Drops the
  URBANIST_DB_URL → DATABASE_URL mirroring step (the cli-flag
  rename on this branch makes the binary read DATABASE_URL
  directly).
- Front matter: adds a Superseded-in-part-by line pointing at the
  cost-spike + Heroku design docs so a reader landing on the
  qa-deploy spec sees the pivot immediately.

Slices #19 / #21 / #23 sections are unchanged (slice #19's artifacts
are deleted in this branch but the slice as written still describes
the intent at the time).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Fill in the spike doc's Decision section + commit the Heroku design doc

**Files:**
- Modify: `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` (Decision section)
- Add (already on disk, uncommitted): `docs/superpowers/specs/2026-05-18-heroku-deploy-design.md`

- [ ] **Step 1: Locate the Decision section in the spike doc**

```bash
grep -n "^## Decision" docs/superpowers/specs/2026-05-18-hosting-cost-spike.md
```

- [ ] **Step 2: Replace the Decision section body**

Currently:

```
## Decision

> _Filled in by the maintainer after reviewing this doc. One of:
> "Stay on MPG" / "Finalist 1: Fly sibling Postgres" / "Finalist 2:
> Hetzner whole stack" / "Finalist 3: Neon"._

Once recorded here, slice #20 in `docs/roadmap.md` and the Slice #20
section of
[`2026-05-18-qa-deploy-design.md`](./2026-05-18-qa-deploy-design.md)
get rewritten to match, and PR #11 either merges as-is (status quo)
or is rebased with the new runbook (any pivot).
```

Replace with:

```
## Decision

**Adopt Heroku Basic dyno + Heroku Postgres Essential-0 in the `us`
(Virginia, Common Runtime) region.** This was not one of the four
finalists in the original spike; brainstorming with the maintainer
surfaced operator familiarity (the maintainer's prior employer is
Heroku) as a decision axis the spike under-weighted, and re-verified
May-2026 pricing across Heroku, Render, Cloudflare Workers
Containers, Hetzner, and Fly that confirmed Heroku at ~$12/mo total
is a defensible Phase 1 spend in exchange for: included near-PITR
backups (Aurora-backed Essential-0 with continuous WAL off-premise),
zero learning tax, and a clean Phase 2 transition.

The ~$7/mo delta over the spike's recommended Finalist 1 (Fly
sibling Postgres at ~$5/mo) is a deliberate trade — see the cost
table and trade-off discussion in
[`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md)
§Why this exists.

The full implementation design + cascading file rewrites are
specified in
[`2026-05-18-heroku-deploy-design.md`](./2026-05-18-heroku-deploy-design.md);
the implementation plan is at
[`../plans/2026-05-18-heroku-deploy-implementation.md`](../plans/2026-05-18-heroku-deploy-implementation.md).

PR #11 (slice-20-fly-deploy-loaddata) is closed; the work continues
on `slice-20-heroku-deploy`. PR #12 (slice #21, Pages/DNS) rebases
on the new `docs/deploy.md` after the Heroku PR lands. The
reversibility plan (migrate back to Fly if dogfood reveals a fit
problem) is in the Heroku design doc's §Reversibility section.
```

- [ ] **Step 3: Stage the Heroku design doc that's already on disk but untracked**

```bash
git status docs/superpowers/specs/2026-05-18-heroku-deploy-design.md
```

Expected: shows as `Untracked` (it was written via Write earlier in the brainstorming session but never committed).

- [ ] **Step 4: Commit both together**

```bash
git add docs/superpowers/specs/2026-05-18-hosting-cost-spike.md \
    docs/superpowers/specs/2026-05-18-heroku-deploy-design.md
git commit -m "$(cat <<'EOF'
docs(spec): record Heroku decision + commit the Heroku design doc

- hosting-cost-spike.md §Decision: filled in with the Heroku
  choice and a one-paragraph rationale pointing at the Heroku
  design doc and this implementation plan. Notes the ~$7/mo delta
  over Finalist 1 as a deliberate familiarity-dividend trade.
- heroku-deploy-design.md: the design doc itself (written during
  brainstorming, not committed at the time). Specifies the
  architecture, build mechanism (heroku/go buildpack), env-var
  consolidation (DATABASE_URL), Procfile, secrets, backups,
  justfile changes, cascading doc rewrites, reversibility back to
  Fly, and verification plan.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: Commit this implementation plan

**Files:**
- Add (already on disk, uncommitted): `docs/superpowers/plans/2026-05-18-heroku-deploy-implementation.md`

- [ ] **Step 1: Stage and commit**

```bash
git add docs/superpowers/plans/2026-05-18-heroku-deploy-implementation.md
git commit -m "$(cat <<'EOF'
docs(plan): Heroku deploy implementation plan

Bite-sized task plan for executing the slice #19.5 → slice #20
Heroku pivot end-to-end on `slice-20-heroku-deploy`. Tasks cover:

1. Branch from main
2. Cherry-pick loaddata Go files from PR #11
3. Verify baseline (api-check, integration tests)
4. Rename URBANIST_DB_URL → DATABASE_URL in 5 Go files
5. Rename in dev tooling (mise, justfile comment)
6. Delete fly.toml + Dockerfile, add Procfile
7. Replace fly-* justfile recipes with heroku-* + db-backup
8. Update `just loaddata` to delegate to binary
9. Rewrite docs/deploy.md against Heroku
10. Update CLAUDE.md (§Hosting + §Tech conventions)
11. Update README.md + api/README.md
12. Update docs/roadmap.md slice #20
13. Update qa-deploy-design.md spec
14. Fill in spike Decision + commit Heroku design doc
15. (this commit)
16. Final verification + grep audits
17. Open PR + close PR #11

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Final verification

**Files:** (no edits — audit only)

- [ ] **Step 1: Full check suite clean**

```bash
just api-check
just api-test-integration
just web-check 2>/dev/null || true   # web is unaffected but harmless to run
```

Expected: all green.

- [ ] **Step 2: Grep audits — every callout from the spec's §Verification**

```bash
# No URBANIST_DB_URL in code, dev tooling, or current docs (only
# historical mentions in the cost-spike doc and old changelog if
# any).
grep -rn "URBANIST_DB_URL" \
    --include="*.go" --include="*.toml" --include="*.yaml" \
    --include="justfile" --include="*.md" \
    .
```

Expected: at most a handful of hits in the cost-spike doc's
"alternatives considered" history and possibly the qa-deploy-design
spec's superseded-by note. No hits in Go code, mise files, justfile,
CLAUDE.md, README, api/README, deploy.md, or the Heroku design doc.

```bash
# No Fly Managed Postgres or MPG references outside cost-spike
# history.
grep -rn "Fly Managed Postgres\|MPG\|flyctl mpg" \
    --include="*.md" --include="*.go" --include="*.toml" \
    .
```

Expected: only hits in the cost-spike doc's comparison table and any
"superseded by" notes you've added. No hits in CLAUDE.md, README,
roadmap, deploy.md, or any Go code.

```bash
# fly.toml and Dockerfile are gone from the tree.
ls fly.toml Dockerfile 2>&1
```

Expected: both report `No such file or directory`.

```bash
# Procfile exists and has the two expected lines.
cat Procfile
```

Expected:

```
release: urbanist-atlas-server migrate up
web: urbanist-atlas-server serve --port=$PORT
```

```bash
# justfile renders cleanly.
just --list | grep -E "^    (heroku-|db-backup)" | wc -l
```

Expected: `6` (heroku-deploy, heroku-logs, heroku-config, heroku-ssh, heroku-loaddata, db-backup).

```bash
# No surviving fly-* recipes.
just --list | grep -c "^    fly-"
```

Expected: `0`.

- [ ] **Step 3: Read the full diff and self-review**

```bash
git log --oneline main..HEAD
```

Expected: ~10–15 atomic commits, each tagged with its conventional-commit type, all telling a coherent story.

```bash
git diff main --stat
```

Expected: deletions for `fly.toml` + `Dockerfile`; additions for `Procfile` + new docs + ported Go files + the new Heroku design doc and implementation plan; modifications across the 5 Go cli files + justfile + mise + the rewritten docs.

---

## Task 17: Open the PR + close PR #11

**Files:** (no edits — git/gh operations)

- [ ] **Step 1: Push the branch**

```bash
git push -u origin slice-20-heroku-deploy
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat: Heroku deploy + Postgres Essential-0 (slice #20, post-#19.5 pivot)" --body "$(cat <<'EOF'
## Summary
- Implements slice #20 against the **Heroku Basic + Postgres Essential-0** target chosen in slice #19.5 (see `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` §Decision and `2026-05-18-heroku-deploy-design.md`).
- **Closes PR #11** (slice-20-fly-deploy-loaddata) — the loaddata Go files are ported forward; the Fly-targeted `docs/deploy.md`, `fly.toml`, and justfile changes are discarded.
- **Deletes** `fly.toml` + `Dockerfile` (slice #19 artifacts, replaced by Procfile + `heroku/go` buildpack).
- **Renames** `URBANIST_DB_URL` → `DATABASE_URL` across the codebase (5 cli flags + dev tooling + docs) so Heroku's auto-rotated connection string Just Works and the binary stays portable to any managed-Postgres host.

## What's in the PR
- `Procfile`: release-phase migrations + web process.
- `docs/deploy.md` (new): end-to-end runbook for the Heroku deploy.
- `docs/superpowers/specs/2026-05-18-heroku-deploy-design.md` (new): the design doc.
- `docs/superpowers/plans/2026-05-18-heroku-deploy-implementation.md` (new): this PR's plan.
- Updates: `CLAUDE.md`, `README.md`, `api/README.md`, `docs/roadmap.md`, `docs/superpowers/specs/2026-05-18-qa-deploy-design.md`, `docs/superpowers/specs/2026-05-18-hosting-cost-spike.md` (Decision filled in).
- Justfile: `fly-*` group dropped; `heroku-*` group + `db-backup` added; `loaddata` recipe delegates to binary subcommand.

## Tests
- `just api-check` clean.
- `just api-test-integration` clean (testcontainers unaffected by the env rename; new `TestPipeline_LoaddataLoadAll` passes).
- Local smoke loop with `DATABASE_URL` set works end-to-end (pg-reset → migrate-up → loaddata → api-run → lookup).

## Test plan
- [x] `just api-check` clean
- [x] `just api-test-integration` clean (incl. new TestPipeline_LoaddataLoadAll)
- [x] `just loaddata` invokes the new binary subcommand via DATABASE_URL
- [x] No URBANIST_DB_URL or Fly Managed Postgres references outside cost-spike history (grep audit)
- [ ] After merge + Heroku setup: `git push heroku main` triggers the release-phase `migrate up`; `heroku run urbanist-atlas-server loaddata` seeds the live DB
- [ ] After seed: `curl -H "X-Atlas-Client: \$SECRET" https://urbanist-atlas-<hash>.herokuapp.com/api/v1/lookup?postal_code=10001&country=US` returns 200

PR #12 (slice #21, Pages/DNS) will rebase on this branch's `docs/deploy.md` after merge and retarget the DNS step from `urbanist-atlas.fly.dev` to the Heroku-managed target.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: prints the PR URL. Save it for the next step.

- [ ] **Step 3: Close PR #11 with a reference to the new PR**

```bash
NEW_PR_URL="<paste the URL printed above>"
gh pr close 11 --comment "Closing in favour of $NEW_PR_URL — the Phase 1 hosting target pivoted from Fly Managed Postgres to Heroku Basic + Postgres Essential-0 per the slice #19.5 cost-spike decision (see hosting-cost-spike.md §Decision + heroku-deploy-design.md). The loaddata code from this PR is ported forward unchanged; only the Fly-targeted deploy.md / fly.toml / fly-* justfile recipes are discarded."
```

- [ ] **Step 4: Verify PR #11 is closed and PR #12 is still open**

```bash
gh pr list --state all --limit 5
```

Expected: PR #11 shows `CLOSED`, PR #12 still shows `OPEN`, new PR shows `OPEN`.

---

## Self-Review (post-write)

**Spec coverage check** (Heroku design doc → tasks):
- §Architecture (Heroku app, `us` region, Basic dyno, Postgres Essential-0): Task 9 (deploy.md) + Task 10 (CLAUDE.md) + Task 13 (qa-deploy-design).
- §Build mechanism (buildpack, delete Dockerfile): Tasks 6, 9 (deploy.md Step 1).
- §Connection-string env consolidation (5 Go files + dev tooling): Tasks 4, 5.
- §Procfile: Task 6.
- §Config (non-secret env): Task 9 (deploy.md Step 3).
- §Secrets: Task 9 (deploy.md Step 3 + §Secrets).
- §Seed files in slug (`URBANIST_SEED_DIR`): Task 9 (deploy.md §3 + §Troubleshooting).
- §Backups: Task 7 (db-backup recipe) + Task 9 (deploy.md §7 schedule step).
- §TLS / custom hostname: Task 9 mentions it; full DNS wiring is in slice #21 (PR #12) per the design's intentional split.
- §Justfile: Tasks 7, 8.
- §PR disposition (close PR #11): Task 17.
- §Cascading doc rewrites: Tasks 10, 11, 12, 13, 14.
- §Reversibility: linked from deploy.md §Troubleshooting at the end of Task 9; full content already in the Heroku design doc.
- §Verification: Task 16.

All design sections have corresponding tasks. ✓

**Placeholder scan:** Every step has either exact code, exact commands with expected output, or exact file content. The two intentional verify-at-implementation-time flags (buildpack monorepo env-var name in Task 9 Step 1, slug working directory in Task 9 §6) are called out as such with the canonical doc URL — these are honest forward flags, not placeholders.

**Type / name consistency check:** `DATABASE_URL` is the env name throughout (never `URBANIST_DB_URL` in any forward-looking content). `urbanist-atlas` is the Heroku app name throughout. The justfile group is `heroku` (not `Heroku` or `hk`) consistently. ✓

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-05-18-heroku-deploy-implementation.md`.

Two execution options:

1. **Subagent-Driven (recommended)** — dispatches a fresh subagent per task, reviews between tasks, fast iteration. Good fit for this plan because the tasks are well-bounded and each ends in a verification step + commit.

2. **Inline Execution** — executes tasks in this session using executing-plans, with batch execution and checkpoints for review.

Which approach?
