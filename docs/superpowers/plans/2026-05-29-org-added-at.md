# Org `added_at` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:subagent-driven-development (recommended) or
> superpowers:executing-plans to implement this plan task-by-task. Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sort the homepage "Recently indexed" strip on a real,
seed-sourced `added_at` date instead of file position.

**Architecture:** Rename the unpopulated `atlas.Org.CreatedAt` →
`AddedAt`, source it from a required date-only `added_at` field in the
seed TOML, backfill the 202 existing orgs (+ PT fixture) from
slice-section evidence, expose it on the wire (`format: date`), and stamp
it from the submission approval path.

**Tech Stack:** Go (`pkg/atlas`, `go-toml/v2`, `oapi-codegen`),
OpenAPI, React/TypeScript (`openapi-typescript`).

**Branch & PR:** Do this work on a **new branch `org-added-at` cut from
`main`**, and open a **PR into `main`** when complete. Do NOT build on
`mr-weekend-edits` (reserved for small pre-launch copy edits). This plan
and its spec already live on `org-added-at`.

---

## Overview

Give every org a real, honest "added" date so the homepage "Recently
indexed" strip sorts on genuine recency instead of file position. Rename
the unpopulated `atlas.Org.CreatedAt` → `AddedAt`, source it from a
required `added_at` field in the seed TOML, backfill the 202 existing
orgs (+ PT fixture) from slice-section evidence, expose it on the wire,
and stamp it from the submission approval path going forward.

**Spec:** `docs/superpowers/specs/2026-05-29-org-added-at-design.md`

## Resuming

### How to Check Current State

```bash
cd /Users/mrossi/dev/urbanist-atlas
# Phase 1 (rename) done? -> Org.AddedAt exists, Org.CreatedAt gone:
grep -n 'AddedAt' api/pkg/atlas/atlas.go || echo "P1 not done"
grep -rn 'Org.*CreatedAt\|CreatedAt:.*t0\|CreatedAt:.*stamp' api/pkg/atlas/ api/internal/httpapi/recent_test.go && echo "P1 incomplete (Org.CreatedAt refs remain)"
# Phase 2 (loader parses added_at)? :
grep -n 'AddedAt\|added_at' api/internal/seedfiles/orgs.go || echo "P2 not done"
# Phase 3 (backfill)? every org should have added_at:
echo "orgs: $(grep -c '^\[\[org\]\]' api/seed/orgs.toml)  added_at lines: $(grep -c 'added_at' api/seed/orgs.toml)"
ls docs/superpowers/plans/2026-05-29-org-added-at-backfill.md 2>/dev/null || echo "mapping table not written"
# Phase 4 (required enforcement)?:
grep -n 'added_at.*required\|missing added_at\|IsZero' api/internal/seedfiles/build.go || echo "P4 not done"
# Phase 5 (wire)?:
grep -n 'added_at' api/openapi.yaml || echo "P5 not done"
grep -n 'AddedAt\|added_at' api/internal/httpapi/oapi/types.gen.go web/src/lib/api.gen.ts || echo "codegen not run"
# Phase 6 (submission stamp)?:
grep -n 'added_at' api/internal/githubpr/toml.go || echo "P6 not done"
# Phase 7 (frontend)?:
grep -rn 'added_at\|addedAt\|Added ' web/src/routes/Home.tsx || echo "P7 not done"
# Build + test status:
cd api && go build ./... && go test ./... 2>&1 | tail -20
```

### How to Resume

1. Run the state-check commands above.
2. Find the first uncompleted phase below and continue from its first
   unchecked task.
3. Confirm you are on the `org-added-at` branch (off `main`), not
   `mr-weekend-edits`: `git branch --show-current`.
4. Each phase ends in its own commit (signed — never bypass signing; see
   `feedback_never_bypass_signing`). The wire-contract change (Phase 5
   openapi.yaml + regen) is committed separately from feature code, per
   the repo's wire-contract rule in CLAUDE.md.

## Critical Constraints (read before starting)

- **Two `CreatedAt` fields exist.** Rename **ONLY `atlas.Org.CreatedAt`**
  and its usages (list below). The unrelated `Submission.CreatedAt`
  (`api/pkg/atlas/submission.go`, `api/internal/store/sqlite/**`,
  `api/internal/githubpr/worker.go`, `oapi/types.gen.go` `created_at`)
  is a real submission-queue timestamp — **do not touch it**. Never run
  a blanket rename.
- **Commit signing:** the maintainer uses SSH commit signing enforced by
  branch protection. Never pass `-c commit.gpgsign=false` or
  `--no-gpg-sign`.
- **Generated files are committed, not hand-edited.** Regenerate
  `oapi/types.gen.go` via `just api-gen` and `web/src/lib/api.gen.ts` via
  `npm run generate:api`. Keep the embedded
  `api/internal/httpapi/openapi.yaml` copy in sync (`just api-check`
  fails on drift).
- **No new dependencies.** `go-toml/v2`, `oapi-codegen`,
  `openapi-typescript` are already vendored/used.
- **Branch:** `org-added-at`, cut from `main`. All work and the final PR
  target `main`. Never commit this work onto `mr-weekend-edits`.

## Success Criteria

- [ ] `atlas.Org.AddedAt` is the populated recency field; zero
  `Org.CreatedAt` references remain (`Submission.CreatedAt` untouched).
- [ ] Every `[[org]]` in `api/seed/orgs.toml` (202) and
  `api/seed/orgs_pt.toml` carries a date-only `added_at`.
- [ ] FileStore boot fails loudly, naming the slug, if any org lacks
  `added_at`.
- [ ] `GET /api/v1/recent` returns orgs ordered by real `added_at` DESC,
  `id` DESC; `added_at` appears in the JSON (`format: date`).
- [ ] `openapi.yaml`, `types.gen.go`, `api.gen.ts` all carry
  `added_at`; `just api-check` passes.
- [ ] Approving a submission renders `added_at = <approval date>` in the
  PR org block (golden test).
- [ ] The "Recently indexed" strip and org detail page show the date.
- [ ] `go test ./...` (api) and `npm test` (web) pass; `go vet` +
  `staticcheck` clean.

---

## Phase 1: Rename `Org.CreatedAt` → `AddedAt` (pure refactor)

**Goal:** One canonical recency field named `AddedAt`, no behavior
change yet (still unpopulated → still sorts by ID), tree compiles, tests
green. This isolates the mechanical rename from the feature.

### Tasks

- [ ] Task 1.1: Rename the struct field and rewrite its doc comments.
  - **Files:** `api/pkg/atlas/atlas.go` (field at line ~101; comments at
    ~80-83 and ~87)
  - Change `CreatedAt time.Time `json:"-" toml:"-"`` →
    `AddedAt time.Time `json:"-" toml:"-"``. (Keep `json:"-"`: wire
    exposure is via the `oapi.Org` date-typed field in Phase 5, not by
    serializing `atlas.Org` directly — see Notes. Keep `toml:"-"`: the
    `OrgEntry` wrapper parses the TOML key in Phase 2.)
  - Rewrite the comment to describe `AddedAt` as the populated,
    seed-sourced recency field (drop the "never populated / future spec"
    language).
  - **Verify:** `grep -n AddedAt api/pkg/atlas/atlas.go` shows the field;
    `grep -n CreatedAt api/pkg/atlas/atlas.go` is empty.

- [ ] Task 1.2: Update the sort sites + comments in MemStore.
  - **Files:** `api/pkg/atlas/memstore.go` (lines ~58 comment, ~311-317
    sort). Also check `OrgsForRegions` (~430) for any `.CreatedAt`.
  - Replace `a.CreatedAt`/`b.CreatedAt` with `AddedAt`. Update the
    `// Sort newest-first … tied CreatedAt` comment to `AddedAt`. The
    stale "doesn't drift between MemStore and Postgres … browse.sql"
    note may be left or trimmed (Postgres is gone post-cutover) — not
    required, but if trimming, do it here.
  - **Verify:** `grep -n CreatedAt api/pkg/atlas/memstore.go` empty.

- [ ] Task 1.3: Update interface doc comments.
  - **Files:** `api/pkg/atlas/store.go` (lines ~41, ~75 — doc comments
    referencing `Org.CreatedAt`).
  - **Verify:** `grep -n CreatedAt api/pkg/atlas/store.go` empty.

- [ ] Task 1.4: Update tests that set/assert the field.
  - **Files:**
    - `api/pkg/atlas/storetest/storetest.go` (lines ~39, 60-61, 194-221):
      rename subtest `OrgsForRegions_PopulatesCreatedAt` →
      `…PopulatesAddedAt`, helper `testOrgsForRegionsPopulatesCreatedAt`
      → `…PopulatesAddedAt`, the `CreatedAt: stamp` literal, and the
      `orgs[0].CreatedAt` assertions.
    - `api/pkg/atlas/browse_test.go` (lines ~46-53, 478): `CreatedAt:` →
      `AddedAt:` in `Org{…}` literals.
    - `api/internal/httpapi/recent_test.go` (lines ~82, 89): `CreatedAt:`
      → `AddedAt:`.
  - **Verify:** `grep -rn 'Org{[^}]*CreatedAt' api/` empty; the
    `grep -rn CreatedAt api/` that remains is ONLY Submission-related
    (submission.go, store/sqlite/**, githubpr/worker.go,
    oapi/types.gen.go:`created_at`).

- [ ] Task 1.5: Build + test + commit.
  - **Verify:** `cd api && go build ./... && go vet ./... && go test ./...`
    all pass; `staticcheck ./...` clean.
  - Commit: `Rename Org.CreatedAt to AddedAt (no behavior change)`.

---

## Phase 2: Loader parses `added_at` (not yet required)

**Goal:** The seed loader reads a date-only `added_at` from TOML and
populates `Org.AddedAt`. Not enforced as required yet, so the (still
un-backfilled) bundle keeps loading.

### Tasks

- [ ] Task 2.1 (TDD): Write a failing loader test for `added_at` parsing.
  - **Files:** `api/internal/seedfiles/` (new or existing
    `build_test.go` / `orgs_test.go`)
  - Test: a minimal in-memory `fs.FS` (or `fstest.MapFS`) seed bundle
    with one org carrying `added_at = 2026-05-21` loads to a store whose
    org has `AddedAt == time.Date(2026,5,21,0,0,0,0,time.UTC)`.
  - **Verify:** test fails (field not parsed yet).

- [ ] Task 2.2: Add `AddedAt` to the `OrgEntry` wrapper + convert in the
  load loop.
  - **Files:** `api/internal/seedfiles/orgs.go`,
    `api/internal/seedfiles/build.go`
  - In `orgs.go`: add a sibling field on `OrgEntry`:
    `AddedAt toml.LocalDate `toml:"added_at"`` (mirrors how
    `RegionSlugs` is parsed off the wrapper because the embedded
    `atlas.Org.AddedAt` is `toml:"-"`). Update the OrgEntry doc comment
    (it currently lists `CreatedAt` among skipped `toml:"-"` fields →
    `AddedAt`).
  - In `build.go` (org loop ~106-117): convert the `toml.LocalDate` to
    `time.Time` at midnight UTC and assign before `AddOrg`, e.g.
    `o := entry.Org; o.AddedAt = time.Date(entry.AddedAt.Year,
    time.Month(entry.AddedAt.Month), entry.AddedAt.Day, 0,0,0,0,
    time.UTC)`. (Confirm `toml.LocalDate` field names via go-toml/v2:
    `Year int`, `Month int`, `Day int`.) Add the `time` import.
  - **Verify:** Task 2.1 test passes.

- [ ] Task 2.3: Build + test + commit.
  - **Verify:** `cd api && go build ./... && go test ./...` pass.
  - Commit: `Parse added_at from org seed TOML into Org.AddedAt`.

---

## Phase 3: Backfill the 202 existing orgs + PT fixture

**Goal:** Every `[[org]]` gets a `added_at` line, with an auditable
mapping table committed alongside.

### Tasks

- [ ] Task 3.1: Produce the slug → date → source mapping table.
  - **Files:** new `docs/superpowers/plans/2026-05-29-org-added-at-backfill.md`
  - Build the table by applying the spec §4 rules in priority order:
    1. Per-org inline date if the org's own comment cites one.
    2. Org under a `=== Slice 7.8 … ===` marker → that section's date:
       "top-31–50 US metros" and "Non-top-50 US metro depth" →
       `2026-05-22`; "CA CMAs #6–10" → `2026-05-23`.
    3. Pre-7.8 block, split by git blame: lines from `402db80` → slice
       7.7 → `2026-05-21`; lines from `7693e07` → founding seed →
       `2026-05-17`.
    4. Residual pre-7.8 orgs (blame to a confounded reorder commit
       `bf3d8ae`/`a5a07776`) → nearest preceding inline slice marker,
       default `2026-05-21`; flag these rows with source `position`.
  - Helper to get per-org blame commit for a slug line:
    `git blame -L '/^slug = "<slug>"/,+1' -- api/seed/orgs.toml`
  - The table has columns: `slug | added_at | source-rule`. Include a
    summary count per date at the top.
  - **Verify:** table has exactly 202 org rows (matches
    `grep -c '^\[\[org\]\]' api/seed/orgs.toml`); every row has a date in
    {2026-05-17, 2026-05-21, 2026-05-22, 2026-05-23} (or a per-org
    override) and a source.

- [ ] Task 3.2: Write `added_at` into `api/seed/orgs.toml`.
  - **Files:** `api/seed/orgs.toml`
  - For each `[[org]]`, insert `added_at = <date>` on its own line within
    the block (suggest immediately after `slug = …` for consistency).
    Use the mapping table. Preserve all existing comments and ordering.
    Prefer a scripted insertion keyed on slug to avoid hand-error, then
    eyeball the diff.
  - **Verify:** `grep -c 'added_at' api/seed/orgs.toml` == 202; and a
    parse check: `grep -c '^\[\[org\]\]'` == `grep -c '^added_at'` (or
    however indented) — every block has exactly one.

- [ ] Task 3.3: Backfill `api/seed/orgs_pt.toml`.
  - **Files:** `api/seed/orgs_pt.toml`
  - Add `added_at` to every PT org using the slice-#4.6 date (the PT
    validation-fixture slice; use the date from that slice's design doc /
    the PT seed commit). One date for all PT orgs is fine — note it in
    the mapping table.
  - **Verify:** every `[[org]]` in `orgs_pt.toml` has `added_at`.

- [ ] Task 3.4: Boot + integration check + commit.
  - **Verify:** `cd api && go test ./...` (the integration suite that
    loads orgs_pt.toml passes); `just api-run` boots without error and
    `curl localhost:<port>/api/v1/recent` returns a sensibly ordered set
    (newest dates first). Stop the server after.
  - Commit: `Backfill added_at for seed orgs from slice-section dates`
    (include orgs.toml, orgs_pt.toml, and the mapping table).

---

## Phase 4: Enforce `added_at` required (fail-loud)

**Goal:** The loader rejects any org missing `added_at`, guaranteeing the
dataset can never regress to the undated state.

### Tasks

- [ ] Task 4.1 (TDD): Failing test — missing `added_at` errors.
  - **Files:** `api/internal/seedfiles/*_test.go`
  - Test: a seed bundle with one org lacking `added_at` makes `Build`
    return an error whose message contains the offending slug.
  - **Verify:** fails (loader currently tolerates zero).

- [ ] Task 4.2: Add the required-field check in the load loop.
  - **Files:** `api/internal/seedfiles/build.go`
  - After converting, if `entry.AddedAt` is the zero `toml.LocalDate`
    (year 0), return
    `fmt.Errorf("org %q: missing required added_at", entry.Slug)`.
  - **Verify:** Task 4.1 passes.

- [ ] Task 4.3 (invariant): Test that the full bundle has no zero
  `AddedAt`.
  - **Files:** a test that builds from the embedded real seed
    (`api/seed/embed.go` exposes the FS) and asserts no loaded org has a
    zero `AddedAt`. (This is the regression guard the whole change
    exists to provide.)
  - **Verify:** passes (depends on Phase 3 backfill).

- [ ] Task 4.4: Build + test + commit.
  - **Verify:** `cd api && go build ./... && go test ./...` pass.
  - Commit: `Require added_at on every seed org (fail-loud at boot)`.

---

## Phase 5: Expose `added_at` on the wire (own commit)

**Goal:** `added_at` (`format: date`) on the `Org` schema, regenerated
into both type files, mapped in the handler. Per CLAUDE.md, the spec edit
+ regen is its own commit ahead of feature code — but since the data
model already exists, this phase's commit is self-contained.

### Tasks

- [ ] Task 5.1: Edit the OpenAPI spec.
  - **Files:** `api/openapi.yaml` — in the `Org` schema, add
    `added_at: { type: string, format: date, description: "Date the org
    was added to the atlas." }` and add `added_at` to the schema's
    `required` list. (Match surrounding style/indentation.)
  - **Verify:** `added_at` present under the `Org` schema; YAML still
    valid.

- [ ] Task 5.2: Regenerate both type files + sync embedded copy.
  - Run `just api-gen` (regenerates `api/internal/httpapi/oapi/types.gen.go`
    and re-syncs the embedded `api/internal/httpapi/openapi.yaml` via the
    `//go:generate` directive) and `cd web && npm run generate:api`
    (regenerates `web/src/lib/api.gen.ts`).
  - **Verify:** `grep -n 'AddedAt' api/internal/httpapi/oapi/types.gen.go`
    (likely `AddedAt openapi_types.Date `json:"added_at"``) and
    `grep -n 'added_at' web/src/lib/api.gen.ts` both hit; `just api-check`
    passes (no embedded-copy drift).

- [ ] Task 5.3: Map `AddedAt` in `toOAPIOrg`.
  - **Files:** `api/internal/httpapi/orgs.go`
  - Add to the returned `oapi.Org{…}`:
    `AddedAt: openapi_types.Date{Time: o.AddedAt}` (import alias matches
    generated code, typically
    `openapi_types "github.com/oapi-codegen/runtime/types"`).
  - **Verify:** `go build ./...`; a handler test (extend
    `recent_test.go`) asserts the JSON response carries
    `"added_at":"2026-05-21"` (date-only, no time component) for a known
    org.

- [ ] Task 5.4: Build + test + commit.
  - **Verify:** `cd api && go test ./...` and `cd web && npm test` pass.
  - Commit: `Expose org added_at on the API (openapi + regen + mapping)`.

---

## Phase 6: Stamp `added_at` from the submission approval path

**Goal:** Approved submissions render `added_at = <approval date>` in the
generated PR org block, so new orgs are always dated and compliant.

### Tasks

- [ ] Task 6.1 (TDD): Update/extend the `RenderOrgBlock` golden test.
  - **Files:** `api/internal/githubpr/worker_test.go` (or the
    toml-render test)
  - Expect the rendered block to include an `added_at = "<date>"` line
    (date-only) for the approval date passed in.
  - **Verify:** fails (not rendered yet).

- [ ] Task 6.2: Render `added_at` in `RenderOrgBlock` + thread the
  approval date.
  - **Files:** `api/internal/githubpr/toml.go`, and the approval handler
    that calls it (find via `grep -rn RenderOrgBlock api/internal`).
  - Decision: pass the approval date explicitly. Change signature to
    `RenderOrgBlock(sub atlas.Submission, addedAt time.Time) string` (or
    a `civil`/date param) and emit
    `fmt.Fprintf(&b, "added_at = %s\n", addedAt.Format("2006-01-02"))`
    placed to match the seed convention (after `slug`). The approval
    handler passes the server-clock approval date (date-only). Do NOT
    reuse `sub.CreatedAt` (that's submission time, not approval time).
  - **Verify:** Task 6.1 passes; `go build ./...`.

- [ ] Task 6.3: Build + test + commit.
  - **Verify:** `cd api && go test ./...` pass.
  - Commit: `Stamp added_at (approval date) in submission PR org block`.

---

## Phase 7: Surface `added_at` in the frontend

**Goal:** The "Recently indexed" strip and the org detail page display
the date in broadsheet style.

### Tasks

- [ ] Task 7.1: Render the date on the recent strip cards.
  - **Files:** `web/src/routes/Home.tsx` (the `RecentlyFiled` component)
  - Add a small dateline, e.g. "Added May 2026" or "Added May 21, 2026",
    formatted from the `added_at` string (use
    `new Date(added_at).toLocaleDateString(...)` or a small formatter).
    `added_at` is now a non-optional field on the `Org` wire type from
    Task 5.2, so no type guard needed. Match the existing "Newly indexed"
    badge styling / broadsheet small-caps language.
  - **Verify:** `cd web && npm run dev`, load the homepage, confirm the
    strip shows real dates and that the four cards are the
    newest-dated orgs (not just the file tail). `npm run build` clean.

- [ ] Task 7.2: Render the date on the org detail page.
  - **Files:** the org detail route component (find via
    `grep -rln 'OrgDetail\|/orgs/' web/src/routes`)
  - Add an "Added <date>" line consistent with the card.
  - **Verify:** detail page shows the date; `npm run build` clean.

- [ ] Task 7.3: Lint + test + commit.
  - **Verify:** `cd web && npm run lint && npm test && npm run build`
    pass.
  - Commit: `Show org added_at on recent strip and detail page`.

---

## Phase 8: Final end-to-end verification

**Goal:** Prove the whole feature works and nothing regressed.

### Tasks

- [ ] Task 8.1: Full backend gate.
  - **Verify:** `cd api && go build ./... && go vet ./... &&
    staticcheck ./... && go test ./... && just api-check` all clean.

- [ ] Task 8.2: Full frontend gate.
  - **Verify:** `cd web && npm run lint && npm test && npm run build`
    clean; `npm run generate:api` produces no diff (codegen committed).

- [ ] Task 8.3: Manual smoke.
  - **Verify:** `just api-run`; `curl -s localhost:<port>/api/v1/recent
    | jq '.data[] | {slug, added_at}'` shows newest-first by real date
    with `format: date` strings; homepage strip matches. Stop server.

- [ ] Task 8.4: Confirm the rename scope held.
  - **Verify:** `grep -rn CreatedAt api/` returns ONLY Submission-related
    hits (submission.go, store/sqlite/**, githubpr/worker.go,
    oapi/types.gen.go `created_at`) — zero `Org`-related ones.

## Notes

- **Why `atlas.Org.AddedAt` keeps `json:"-"`:** all org responses are
  serialized through the generated `oapi.Org` type (via `toOAPIOrg`),
  whose `added_at` is an `openapi_types.Date` that marshals date-only.
  The internal `atlas.Org` is not serialized directly on any current
  code path, so giving it a `json:"added_at"` tag would risk emitting an
  RFC3339 datetime if that ever changed. Keeping `json:"-"` and exposing
  via the oapi layer is the honest, low-risk choice. (This refines spec
  §1, which suggested `json:"added_at"`; flag to maintainer if they want
  `atlas.Org` directly serializable instead — that would need a custom
  date type.)
- **Backfill dates (locked):** founding seed `2026-05-17` (commit
  `7693e07`, maintainer-confirmed over the 05-20 slice-7.6 spec date);
  slice 7.7 `2026-05-21`; slice 7.8 US `2026-05-22`; slice 7.8 CA
  `2026-05-23`.
- **go-toml/v2 `toml.LocalDate`** has `Year`, `Month`, `Day` int fields
  and parses bare `added_at = 2026-05-21` TOML dates. Verify exact field
  names against the vendored version before relying on them in build.go.
- Each phase is an atomic, signed commit. The work stays on
  `mr-weekend-edits` unless the maintainer asks for a PR branch.
