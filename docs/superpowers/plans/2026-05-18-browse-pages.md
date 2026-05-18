# Browse + metro pages — implementation plan (slice #14)

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Use `superpowers:test-driven-development` for every code-bearing step.

**Goal:** Ship the `/browse` and `/m/:metroSlug` pages, plus wire the homepage `aside` cards to data, all against the existing OpenAPI contract. Develop against typed fixtures while the backend slice (#6) is in flight in a parallel worktree; delete fixtures in a cleanup commit after the backend lands.

**Architecture:** Three new API helpers in `web/src/lib/api.ts` (`listMetros`, `getMetro`, `listRecent`), each gated by `import.meta.env.VITE_USE_FIXTURES` for dev. Three new React Query keys in `queryKeys.ts`. Two new route components (`Browse`, `Metro`) consuming the existing broadsheet CSS vocabulary. `Home.tsx` aside cards swap "Coming soon" stubs for `useQuery`-driven lists.

**Tech Stack:** React 19, Vite 6, TypeScript strict, @tanstack/react-query v5, react-router v7, openapi-typescript, Vitest + RTL. **No new dependencies.**

**Spec:** [`docs/superpowers/specs/2026-05-18-browse-pages-design.md`](../specs/2026-05-18-browse-pages-design.md). Read §1 (routes/pages) and §4 (fixtures) before starting.

**Preconditions:**

1. Working in worktree `.worktrees/browse-frontend` on branch `slice-14-browse-pages`, branched from `main` at the commit that committed this plan.
2. `just web-check` is green on baseline. If not, stop and report.
3. `web/src/lib/api.gen.ts` is current; `npm run generate:api` produces zero diff.
4. The new types (`MetroSummary`, `MetroDetail`) already re-export from `web/src/lib/api.ts:24-25`. Verify they're there before starting.

---

## File Structure

### New

| Path | Responsibility |
|---|---|
| `web/src/lib/fixtures/browse.ts` | Typed dev fixtures (`metrosFixture`, `metroDetailFixture`, `recentFixture`). **Deleted in cleanup commit.** |
| `web/src/routes/Browse.tsx` | `/browse` page — metro list with org counts. |
| `web/src/routes/Browse.test.tsx` | Vitest + RTL coverage. |
| `web/src/routes/Metro.tsx` | `/m/:metroSlug` page — metro orgs list. |
| `web/src/routes/Metro.test.tsx` | Vitest + RTL coverage incl. 404 path. |

### Modified

| Path | Change |
|---|---|
| `web/src/lib/api.ts` | Add `listMetros`, `getMetro`, `listRecent`; add `VITE_USE_FIXTURES` short-circuit. |
| `web/src/lib/queryKeys.ts` | Add `metros`, `metro`, `recent` factories. |
| `web/src/router.tsx` | Add `/browse` and `/m/:metroSlug` children of `App`. |
| `web/src/routes/Home.tsx` | Replace two "Coming soon" stubs with React Query-driven lists. |

### Will be deleted in cleanup commit (post-backend-merge)

| Path | Reason |
|---|---|
| `web/src/lib/fixtures/browse.ts` | Real backend in place. |
| `VITE_USE_FIXTURES` branches in `web/src/lib/api.ts` | Same. |

---

## Tasks

### Phase 0 — baseline

- [x] Confirm worktree state: `git rev-parse --abbrev-ref HEAD` is `slice-14-browse-pages`, working tree clean.
- [x] Run `just web-check` and confirm green.
- [x] Run `npm --prefix web run generate:api` and confirm zero diff (contract unchanged).
- [x] Verify `MetroSummary` and `MetroDetail` are re-exported in `web/src/lib/api.ts` (lines 24-25 per the spec). If not present, stop — the openapi codegen step is broken.

### Phase 1 — Fixtures

- [x] Create `web/src/lib/fixtures/browse.ts`. Export three constants typed against `components['schemas']`:
  ```ts
  import type { components } from '../api.gen.ts';
  type MetroSummary = components['schemas']['MetroSummary'];
  type MetroDetail = components['schemas']['MetroDetail'];
  type Org = components['schemas']['Org'];

  export const metrosFixture: MetroSummary[] = [ /* 5-6 entries: nyc-metro, sf-metro, lisbon-metro, vancouver-metro, toronto-metro */ ];
  export const metroDetailFixture: Record<string, MetroDetail> = { /* nyc-metro, lisbon-metro at minimum */ };
  export const recentFixture: Org[] = [ /* 5 entries, recent created_at */ ];
  ```
  Use realistic data shaped on the existing seed (see `api/seed/orgs.toml` for org examples).
- [x] No tests for the fixture file itself; the type checker is the guarantee.
- [x] Commit: `feat(web): typed dev fixtures for slice #14 browse pages`.

### Phase 2 — API helpers

- [x] Extend `web/src/lib/api.ts`:
  - Add `const USE_FIXTURES: boolean = import.meta.env.VITE_USE_FIXTURES === 'true';` near the `apiBase` const. (Implemented as a per-call `useFixtures()` helper so `vi.stubEnv` works during tests without juggling module-cache resets; Vite still inlines the literal in prod builds, so the dynamic fixture import is tree-shaken.)
  - Add `listMetros(init?)`, `getMetro(slug, init?)`, `listRecent(init?)` mirroring the existing `lookup`. Each function checks `USE_FIXTURES` and short-circuits to the fixture module (use dynamic `import('./fixtures/browse.ts')` inside the branch to keep the import out of the prod bundle's static graph).
  - `getMetro`: when `USE_FIXTURES` and the slug isn't in `metroDetailFixture`, throw an `ApiError(404, 'Not Found', undefined, undefined)` to match the real 404 path.
- [x] Write or extend `web/src/lib/api.test.ts` (create it if it doesn't exist — slice #17 in the roadmap calls for it; this slice gets a head start). Cover:
  - `listMetros` returns the fixture when `VITE_USE_FIXTURES=true`.
  - `getMetro('nyc-metro')` returns the fixture detail.
  - `getMetro('nope')` throws `ApiError` with status 404.
  - `listRecent` returns ≤ 10 entries.
  - When `VITE_USE_FIXTURES` is unset, the functions call `fetch` (mock global `fetch` to a 200 + JSON body and assert the URL hit).
- [x] Run `npm --prefix web test`. All pass.
- [x] Commit: `feat(web): API helpers for browse + recent (slice #14)`.

### Phase 3 — Query keys

- [x] Extend `web/src/lib/queryKeys.ts`:
  ```ts
  metros: () => ['metros'] as const,
  metro: (slug: string) => ['metro', slug] as const,
  recent: () => ['recent'] as const,
  ```
- [x] No standalone tests; coverage comes via component tests.
- [x] Commit: included in the next code commit (no atomic commit for this trivial change).

### Phase 4 — Browse page

- [x] Write `web/src/routes/Browse.test.tsx` first, mirroring the structure of `Results.test.tsx` (vi.hoisted mock for `listMetros`, MemoryRouter wrapping, QueryClient with `retry: false`):
  - Happy path: fixture-shaped metros render with names and org counts; rendered order matches input (descending).
  - Loading state: while query is pending, a "Loading metros…" or similar element is visible.
  - Error state: when `listMetros` rejects with `ApiError`, an inline error message renders (not a crash).
  - Each metro row contains a link with `href="/m/{slug}"`.
- [x] Implement `web/src/routes/Browse.tsx`. Use the `.page` single-column layout (per the design system; see `Home.tsx` for `.broadsheet-body`, but Browse is more list-y so `.page` is correct). Include:
  - `<h1>` or `.section-label` masthead-style heading "Browse by metro".
  - Brief two-sentence lede.
  - A list of metros. Each row: name (Link to `/m/:slug`), small `.dateline`-style country line, org count chip on the right.
- [x] Run Browse tests, confirm pass.
- [x] Commit: `feat(web): /browse page (slice #14)`.

### Phase 5 — Metro page

- [x] Write `web/src/routes/Metro.test.tsx`:
  - Happy path: GET `/m/nyc-metro` (with mocked `getMetro` returning fixture) renders the metro name + the orgs list.
  - 404 path: mocked `getMetro` throws `ApiError(404, ...)` → inline empty-state message ("This metro isn't in the atlas yet"), not a crash.
  - Empty orgs (200 with empty array): renders a friendly empty state.
- [x] Implement `web/src/routes/Metro.tsx`:
  - `useParams<{ metroSlug: string }>()` for the slug.
  - `useQuery({ queryKey: queryKeys.metro(slug), queryFn: () => getMetro(slug) })`.
  - On 404 (`ApiError` with status 404), render an inline empty-state.
  - On 200, render a classified-section structure (one section, "Organizations serving {name}") listing each org. Reuse `<Entry>` via a `LookupOrg`-shaped view-model with `matched_region_slugs: []` (path 1; Entry's empty-array branch was already covered in `Entry.test.tsx`). `Dateline` is omitted in favor of a simpler `.page-header` block here — `Dateline` is postal-code-shaped and a metro page has no postal code; the broadsheet section heading carries the same role.
- [x] Run Metro tests, confirm pass.
- [x] Run `Results.test.tsx` and `Entry.test.tsx` and confirm they STILL pass (regression check for any Entry-touching refactors).
- [x] Commit: `feat(web): /m/:metroSlug page (slice #14)`.

### Phase 6 — Router wiring

- [ ] Modify `web/src/router.tsx` to add two new children of the `App` route after `r/:postalCode`:
  ```ts
  { path: 'browse', Component: Browse },
  { path: 'm/:metroSlug', Component: Metro },
  ```
  Import them at the top of the file.
- [ ] Run `just web-check` (full CI gate). All pass.
- [ ] Commit: `feat(web): wire /browse + /m/:metroSlug routes (slice #14)`.

### Phase 7 — Homepage asides

- [ ] If `web/src/routes/Home.test.tsx` does not exist, create it. Mock `listMetros` and `listRecent` via the same `vi.hoisted` pattern. Cover:
  - Top 6 metros render in the first aside; "Browse all metros →" link points to `/browse`.
  - Top 5 recent orgs render in the second aside.
  - Error states fall back to the existing "Coming soon" copy + a tiny `(retry)` affordance (or just a graceful inline error — pick one).
  - Loading states show subdued placeholders that don't change layout dimensions much.
- [ ] Modify `web/src/routes/Home.tsx`:
  - Add `useQuery` hooks for `listMetros` (slice to 6) and `listRecent` (slice to 5).
  - Replace the inner content of the first `.aside-card` (currently the "Browse by metro" stub) with the metros list + a footer link to `/browse`.
  - Replace the inner content of the second `.aside-card` with the recent-orgs list.
  - Preserve the `.section-label` headers and the `.aside-card-status` class for loading/error states.
- [ ] Run Home tests + all sibling tests; confirm pass.
- [ ] Commit: `feat(web): wire homepage asides to /metros and /recent (slice #14)`.

### Phase 8 — Verification (fixture mode)

- [ ] Run `VITE_USE_FIXTURES=true npm --prefix web run dev` and manually verify in a browser:
  - `/` shows populated metros aside (6 entries) and recent aside (5 entries).
  - `/browse` lists the fixture metros in descending org count.
  - `/m/nyc-metro` renders detail correctly.
  - `/m/totally-fake` renders the inline empty-state, not a crash.
- [ ] `just web-check` green.
- [ ] If anything looks visually off (e.g., aside layout shifts during load), tighten the loading placeholder dimensions. **No global CSS changes.**

### Phase 9 — Wait for backend, then switch to live

This phase begins **only after slice #6 (backend) has merged to `main`**. Coordinate with the maintainer.

- [ ] `git fetch origin && git rebase origin/main` in this worktree to pick up the backend.
- [ ] Confirm `npm --prefix web run generate:api` produces no diff (the openapi.yaml shouldn't have changed; the backend merge regenerated the Go types only).
- [ ] Run the backend locally: in another shell, `just pg-reset && just migrate-up && just seed && just api-run`.
- [ ] In the web worktree: `npm --prefix web run dev` (NO `VITE_USE_FIXTURES`) and re-verify all four scenarios from Phase 8 against the real backend.
- [ ] Fix any wire-shape mismatches surfaced by the live run. (Should be none if fixtures were correctly typed.)

### Phase 10 — Cleanup commit

- [ ] Delete `web/src/lib/fixtures/browse.ts`.
- [ ] Remove the `USE_FIXTURES` const and the three `if (USE_FIXTURES)` branches from `web/src/lib/api.ts`. The dynamic imports go away with them.
- [ ] Remove any `VITE_USE_FIXTURES` references in tests; replace with direct `fetch` mocks where needed.
- [ ] Run `just web-check` and confirm green.
- [ ] Commit: `chore(web): remove dev fixtures now that /metros + /recent are live (slice #14)`.

### Phase 11 — Ship

- [ ] Use `superpowers:finishing-a-development-branch` to open a PR for this branch.
- [ ] Update `docs/roadmap.md` Status section if it tracks slice #14.

---

## Non-goals

Per spec §"Non-goals":

- No new dependencies (MSW, helmet, etc.).
- No homepage layout restructuring beyond the inside of the two `.aside-card` divs.
- No 404 page (slice #15 territory).
- No org detail pages (deferred).
- No `EntryList`/`Entry` refactor with behavior change on `/r/:postalCode`.
- No SSR.
- No CSS module migration.

## Risks

Per spec §"Risks & mitigations". Specifically:

- **Fixture/wire drift:** mitigated by typing fixtures against `components['schemas']`. The TS compiler refuses drift at edit time.
- **Async fixture leaking into prod bundle:** the dynamic `import('./fixtures/browse.ts')` inside the truthy branch lets Vite tree-shake the fixture when `VITE_USE_FIXTURES` is unset (which is true for all prod builds). Verify in Phase 8 that the prod build doesn't reference `browse.ts` — `npm --prefix web run build` then `grep -r "fixtures" web/dist/` should return nothing.
- **`/m/:slug` over-rendering:** standard React Query caching covers it; no special handling.

## Coordination with slice #6 (backend)

The backend slice runs in `.worktrees/browse-backend` on branch
`slice-06-browse-endpoints`, also branched from this same `main`
commit. The two slices share **no source files** — only the openapi
contract — so there is no merge-conflict surface.

Backend merges first. This worktree's Phase 9 is the swap-to-live step.
