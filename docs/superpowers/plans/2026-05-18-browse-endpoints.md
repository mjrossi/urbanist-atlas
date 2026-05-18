# Browse + recent endpoints — implementation plan (slice #6)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Use `superpowers:test-driven-development` for every code-bearing step.

**Goal:** Ship the three backend endpoints `GET /api/v1/metros`, `GET /api/v1/metros/{slug}`, `GET /api/v1/recent` behind the existing OpenAPI contract, with full handler-test coverage including a regression for the national-tier filter.

**Architecture:** `pkg/atlas` gains `IsMetroKind` plus three new `Store` methods (`ListMetros`, `GetMetro`, `ListRecent`) implemented in both `MemStore` and the Postgres store. Three thin chi handlers in `internal/httpapi/` convert domain types to the existing `oapi.MetroSummary` / `oapi.MetroDetail` wire types. SQL uses a recursive CTE to walk region descendants (mirrors the existing `AncestorRegions` upward walk).

**Tech Stack:** Go 1.26, chi, pgx/v5, sqlc, testcontainers-go. No new dependencies.

**Spec:** [`docs/superpowers/specs/2026-05-18-browse-endpoints-design.md`](../specs/2026-05-18-browse-endpoints-design.md). Read **§2 (metro-kind set)**, **§3 (storage)**, and **§5 (handlers)** before starting.

**Preconditions:**

1. Working in worktree `.worktrees/browse-backend` on branch `slice-06-browse-endpoints`, branched from `main` at the commit that committed this plan.
2. Docker is running and `just pg-up && just migrate-up && just seed` succeeds.
3. `just api-check` is green on baseline. If not, stop and report.
4. `api/openapi.yaml` is unchanged from `main`. Verify with `just api-oapi-gen` producing no diff.

---

## File Structure

### New

| Path | Responsibility |
|---|---|
| `api/pkg/atlas/metro_kinds.go` | `IsMetroKind` predicate + `MetroKinds()` accessor. |
| `api/pkg/atlas/metro_kinds_test.go` | Unit tests. |
| `api/pkg/atlas/browse.go` | `MetroSummary`, `MetroDetail` domain types + top-level orchestration funcs (`ListMetros`, `GetMetro`, `ListRecent`) if useful. |
| `api/pkg/atlas/browse_test.go` | Unit tests against MemStore. |
| `api/internal/httpapi/metros.go` | `listMetrosHandler`, `getMetroHandler` + adapters. |
| `api/internal/httpapi/metros_test.go` | httptest coverage. |
| `api/internal/httpapi/recent.go` | `recentHandler` + adapter. |
| `api/internal/httpapi/recent_test.go` | httptest coverage (incl. national-filter regression). |
| `api/internal/store/postgres/queries/browse.sql` | sqlc queries. |
| `api/internal/store/postgres/gen/browse.sql.go` | sqlc-generated, committed. |

### Modified

| Path | Change |
|---|---|
| `api/pkg/atlas/store.go` | Add three methods to `Store` interface. |
| `api/pkg/atlas/memstore.go` | Implement the three methods on `MemStore`. |
| `api/pkg/atlas/memstore_test.go` | (if exists) extend; otherwise covered by `browse_test.go`. |
| `api/pkg/atlas/devfixtures.go` | Verify fixtures already include enough metros / orgs to exercise tests; supplement if needed (one approved org per supported country's metro is enough). |
| `api/internal/store/postgres/store.go` | Implement the three methods. |
| `api/internal/httpapi/router.go` | Wire the three new routes inside the `/api/v1` group. |

---

## Tasks

### Phase 0 — baseline

- [x] Confirm worktree state: `git rev-parse --abbrev-ref HEAD` is `slice-06-browse-endpoints`, working tree clean.
- [x] Run `just api-check` and confirm green.
- [x] Run `just api-oapi-gen` and confirm zero diff (contract unchanged).

### Phase 1 — `IsMetroKind` predicate

- [x] Write `api/pkg/atlas/metro_kinds_test.go` first. Assert `IsMetroKind` returns true for `us:metro`, `ca:cma`, `ca:regional-district`, `pt:area-metropolitana`; returns false for `us:state`, `ca:province`, `pt:distrito`, `pt:nacional`, and `""`. Assert `MetroKinds()` returns those four kinds in deterministic order. Run the test, watch it fail to compile.
- [x] Write `api/pkg/atlas/metro_kinds.go` per spec §2. The `metroKinds` map is the source of truth; `IsMetroKind` and `MetroKinds()` derive from it. Doc comment names what's in/out per spec.
- [x] Run `go test ./pkg/atlas/...` and confirm tests pass.
- [x] Commit: `feat(api): add IsMetroKind predicate (slice #6)`.

### Phase 2 — Domain types and Store interface

- [x] Add `MetroSummary` and `MetroDetail` to `api/pkg/atlas/browse.go`. Mirror the wire shape from `api/openapi.yaml:606-628`:
  - `MetroSummary{ Region Region; OrgCount int64 }`
  - `MetroDetail{ Region Region; Orgs []Org }`
  - Doc comments noting these are domain types; wire types live in `oapi/`.
- [x] Extend `api/pkg/atlas/store.go` `Store` interface with three methods (signatures from spec §4):
  - `ListMetros(ctx context.Context) ([]MetroSummary, error)`
  - `GetMetro(ctx context.Context, slug string) (*MetroDetail, error)` — nil means not-found.
  - `ListRecent(ctx context.Context) ([]Org, error)`
- [x] Compile check: `go build ./...` will fail on both store implementations until they get the new methods. That's expected.
- [x] Commit: `feat(api): add browse Store methods (slice #6)`.

Note: also added `CreatedAt time.Time` with `json:"-"` to `atlas.Org`
so newest-first ordering in `ListRecent` and `GetMetro` has a stable
source. The wire contract is unchanged.

### Phase 3 — MemStore implementations + tests

- [x] Write `api/pkg/atlas/browse_test.go` first. Build a MemStore with `LoadDevFixtures` and assert:
  - `ListMetros` returns ≥ 1 entry, ordered `OrgCount DESC, Name ASC`.
  - `ListMetros` excludes non-metro kinds (e.g., states, provinces, distritos).
  - `ListMetros` excludes metros with zero approved orgs.
  - `GetMetro("nyc-metro")` returns a non-nil pointer with the NYC region + at least one org. (If the dev fixture slug differs, use the actual slug. Adapt assertions to the fixture content.)
  - `GetMetro("does-not-exist")` returns `(nil, nil)` — not an error.
  - `GetMetro` on a non-metro slug (e.g., a state slug from the fixtures) also returns `(nil, nil)`.
  - `ListRecent` returns ≤ 10 entries, ordered newest-first by `CreatedAt`.
  - `ListRecent` does NOT include any org whose ONLY region attachments are `scope_tier='national'`. Use MUBi if it's seeded; otherwise seed a national-tier region + org for the test specifically.
- [x] Implement on `MemStore` (in `api/pkg/atlas/memstore.go`):
  - `ListMetros`: walk `s.regions`, filter by `IsMetroKind`, count orgs whose `Regions` slice contains the region (or any descendant — see note), sort.
  - `GetMetro`: find region by slug, check `IsMetroKind`, gather descendant region IDs via downward graph walk, gather orgs tagged to any of them.
  - `ListRecent`: filter `s.orgs` by "has at least one non-national region attachment," sort by `CreatedAt DESC`, cap at 10.
  - **Note on descendants for MemStore:** the existing `AncestorRegions` walks upward (child → parents). For descendants, walk the same graph in the opposite direction. Add an unexported helper `descendantRegionIDs(rootID int64) []int64` if it doesn't already exist.
- [x] Run `go test ./pkg/atlas/...`. All tests pass.
- [x] Commit: `feat(api): MemStore browse implementations (slice #6)`.

Note: Phase 2 and Phase 3 share one commit to keep every checkpoint
buildable (the Store interface widening in Phase 2 alone leaves
internal/store/postgres without `ListMetros`/`GetMetro`/`ListRecent`).

### Phase 4 — Postgres SQL + sqlc

- [x] Write `api/internal/store/postgres/queries/browse.sql` with four named queries:
  - `ListMetros :many` — recursive CTE walking `region_parents` downward from each metro region, counts distinct approved org IDs in `organization_regions`, filters `scope_tier <> 'national'`, returns `(region_id, country, kind, name, slug, scope_tier, sort_priority, org_count)`. Ordered `org_count DESC, name ASC`. The metro-kind set comes in as `$1::text[]`.
  - `GetMetroBySlug :one` — single region by slug AND `kind = ANY($2::text[])`. Returns NULL if not found or not a metro.
  - `OrgsForMetro :many` — recursive CTE descending from a region ID, return distinct orgs tagged to that region or any descendant. Order by `created_at DESC`.
  - `ListRecent :many` — top 10 orgs by `created_at DESC`, excluding orgs whose only region attachments are `scope_tier='national'`. Filter via NOT EXISTS on the org's non-national regions.
- [x] Run `just api-sqlc-gen`. Check `api/internal/store/postgres/gen/browse.sql.go` is generated; check that it compiles when included with the rest of the package.
- [x] Commit: `feat(api): sqlc browse queries (slice #6)`.

### Phase 5 — Postgres store implementations + integration test

- [x] Implement the three methods on the Postgres store in `api/internal/store/postgres/store.go`:
  - `ListMetros` calls `ListMetros(ctx, atlas.MetroKinds())` and maps rows → `[]atlas.MetroSummary`.
  - `GetMetro`: first call `GetMetroBySlug(ctx, slug, atlas.MetroKinds())`. If no row, return `(nil, nil)`. Otherwise call `OrgsForMetro` and assemble.
  - `ListRecent` calls `ListRecent(ctx)` and maps to `[]atlas.Org`.
- [x] Extend `api/internal/store/postgres/pipeline_test.go` (or add a new `*_test.go` next to it) with a testcontainers-backed test that:
  - Loads seed data via the existing helpers.
  - Calls each new Store method and asserts: shape correctness, ordering, national-tier exclusion.
- [x] Run `go test -tags=integration ./internal/store/postgres/...` and confirm pass. (Requires Docker.)
- [x] Commit: `feat(api): Postgres browse implementations (slice #6)`.

### Phase 6 — HTTP handlers + httptest

- [x] Write `api/internal/httpapi/metros_test.go` first, mirroring `lookup_test.go`:
  - `TestListMetros_HappyPath_ReturnsOAPIShape`: status 200, `Content-Type: application/json`, body is `[]oapi.MetroSummary`, length ≥ 1, descending org_count.
  - `TestGetMetro_HappyPath`: GET `/api/v1/metros/{seed-metro-slug}` → 200, body has `region.slug = slug` and `orgs.length ≥ 1`.
  - `TestGetMetro_404`: GET `/api/v1/metros/totally-bogus` → 404, `Content-Type: application/problem+json`, body has `type`, `title`, `request_id` (echoing `X-Request-ID`).
  - `TestGetMetro_NonMetroSlug_404`: a state/province slug → 404 (the slug exists in regions but isn't a metro-equivalent).
- [x] Write `api/internal/httpapi/recent_test.go`:
  - `TestListRecent_HappyPath_ReturnsOAPIShape`: 200, JSON array, length ≤ 10, newest-first.
  - `TestListRecent_ExcludesNationalTier`: seed a national-tier org (or rely on MUBi from devfixtures); assert it's absent from the response.
- [x] Run tests; they fail because handlers don't exist yet.
- [x] Implement `api/internal/httpapi/metros.go`:
  - `listMetrosHandler(store atlas.Store, logger *slog.Logger) http.HandlerFunc` — ~10 lines.
  - `getMetroHandler(...)` — pulls slug from `chi.URLParam(r, "slug")`, calls `store.GetMetro`, returns 404 on nil pointer.
  - `toOAPIMetroSummary([]atlas.MetroSummary) []oapi.MetroSummary` adapter — mirror `toOAPILookupResult` style.
  - `toOAPIMetroDetail(*atlas.MetroDetail) oapi.MetroDetail` adapter.
- [x] Implement `api/internal/httpapi/recent.go`:
  - `recentHandler(store, logger)` — 10 lines.
  - Reuse the existing `toOAPIOrg` adapter (or add one if Org doesn't already have one — check by grepping for it; if absent, add it in `metros.go` since Metro also needs it).
- [x] Run handler tests, confirm all pass.
- [x] Commit: `feat(api): browse + recent handlers (slice #6)`.

Note: routes had to be wired in the same commit as the handlers (the
RED test from Phase 6 returns 404 from the router, not from the
handler logic). Phase 7's checkbox folds into this commit.

### Phase 7 — Router wiring

- [x] Modify `api/internal/httpapi/router.go` inside the `r.Route("/api/"+apiVersion, ...)` block, after the existing `r.Get("/lookup", ...)` line:
  ```go
  r.Get("/metros", listMetrosHandler(cfg.Store, logger))
  r.Get("/metros/{slug}", getMetroHandler(cfg.Store, logger))
  r.Get("/recent", recentHandler(cfg.Store, logger))
  ```
- [x] Run `just api-check`. All tests pass; no lint issues.
- [x] Commit: `feat(api): wire browse + recent routes (slice #6)`.

Note: folded into the Phase 6 commit; see the Phase 6 note for why.

### Phase 8 — Final verification

- [ ] `just api-check` green.
- [ ] `just pg-reset && just migrate-up && just seed` succeeds.
- [ ] `just api-run` in background; in another shell run each curl from the spec §"Acceptance criteria" block and confirm output matches.
- [ ] Stop the server.
- [ ] Update `docs/roadmap.md` if it tracks per-slice status (the existing format does, see the "## Status" section header at line 12).
- [ ] Commit any docs-only updates separately: `docs(roadmap): mark slice #6 shipped`.
- [ ] Use `superpowers:finishing-a-development-branch` to open a PR.

---

## Non-goals

Per spec §"Non-goals":

- No openapi.yaml edits.
- No `limit` param on `/recent`.
- No pagination.
- No Cache-Control headers.
- No memory store changes beyond the three new methods.
- No homepage frontend work (that's slice #14, separate worktree).

## Risks

Per spec §"Risks & mitigations". The big one: ensuring the descendant walk in `ListMetros`/`GetMetro` correctly attributes Brooklyn-tagged orgs to the NYC-metro region. Test this explicitly in Phase 3 by:

1. Pick a seed metro that has both directly-tagged orgs and descendant-tagged orgs.
2. Assert `ListMetros`'s `org_count` for that metro equals the union of both sets, deduplicated.
3. Assert `GetMetro`'s `orgs` array contains both.
