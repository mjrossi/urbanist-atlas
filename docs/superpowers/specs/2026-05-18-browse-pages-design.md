# Browse + metro pages (slice #14)

**Status:** Active — implementation of slice #14 (frontend half of the browse/metros pair).
**Supersedes:** none.
**Related:**
- [`2026-05-18-browse-endpoints-design.md`](./2026-05-18-browse-endpoints-design.md) (backend half — slice #6, runs in parallel)
- [`docs/roadmap.md:101`](../../roadmap.md) (slice text)
- [`api/openapi.yaml:132-193`](../../../api/openapi.yaml) (contract)

## Why this exists

The homepage today has two "Coming soon" placeholder cards
(`web/src/routes/Home.tsx:36-53`) and no `/browse` page. The
broadsheet design CLAUDE.md describes requires both: a list of metros
with org counts ("wander rather than search"), and a feed of recently
approved orgs ("standing invitation to discover an effort you haven't
heard of yet").

This slice ships:

1. **Two new pages.** `/browse` lists every metro region with its
   org-count badge. `/m/:metroSlug` lists the orgs that serve that
   metro, reusing the classified-section layout from `Results`.
2. **Wires the existing homepage `aside` cards.** Top ~6 metros by
   org count and ~5 most-recently-approved orgs replace the static
   "Coming soon" stub text.
3. **Develop-time fixture support.** While slice #6 (the backend half)
   is in flight in a separate worktree, this slice ships a small typed
   fixture module so the pages build and tests pass against generated
   types. The fixture is deleted in a cleanup commit after the
   backend lands.

The contract is already defined (`MetroSummary`, `MetroDetail`, `Org`
in `web/src/lib/api.gen.ts`, re-exported from `web/src/lib/api.ts:24-25`).
**No openapi changes, no regen required.**

## Strategic goal

Close the visible v1 homepage gap and stand up the two pages that the
roadmap anticipates from #13 onward. Stay strictly within the broadsheet
design language already established — no new dependencies, no
restructuring of the existing layouts.

## Design

### 1. Routes & pages

#### `/browse` — `web/src/routes/Browse.tsx`

A single-column `.page` layout (per the existing CSS), with a
classified-section list of metros:

- Page title: "Browse by metro".
- Lede paragraph: brief framing, two sentences max.
- One section per scope tier? No — flat list. With ~10 metros total,
  grouping adds noise. Ordering is `org_count DESC` (already enforced
  by the API).
- Each row: metro name (linked to `/m/:slug`), small caps dateline of
  the metro's country/state, org count chip ("4 groups", "12 groups").
- Empty state: "No metros indexed yet — try the search box at the top
  of every page." (Realistic only during fixture-mode dev.)
- Error state: reuse the `ApiError` rendering pattern from `Results`.

#### `/m/:metroSlug` — `web/src/routes/Metro.tsx`

Same classified-section list layout as `Results`, but rooted at one
metro instead of a postal code:

- Page title: the metro name, with a smaller dateline (country/state).
- One classified section, "Organizations serving {metro}", listing the
  orgs returned by `GET /api/v1/metros/{slug}`. Reuses `EntryList` /
  `Entry` from `web/src/components/`.
- 404 state: when the API returns 404, render an inline empty-state
  ("This metro isn't in the atlas yet — try [browse]") rather than
  routing to a dedicated 404 page. Slice #15 ships the 404 page; we
  don't depend on it.
- Empty `orgs` array (200 with `[]`): render a friendly empty state.

#### Homepage aside cards — modify `web/src/routes/Home.tsx`

Replace the "Coming soon" stub in each `.aside-card` with a React
Query-driven list:

- **Browse by metro card:** show top 6 metros from `listMetros()`,
  each linked to `/m/:slug`. Below the list, a "Browse all metros →"
  link to `/browse`.
- **Recently added card:** show top 5 orgs from `listRecent()`, each
  with name + city/dateline + outbound link.

While data is loading, both cards show a subdued "Loading…" line that
mirrors the existing copy length so layout doesn't shift. On error,
they fall back to the existing "Coming soon" copy plus a tiny "(retry)"
button rather than blowing up the homepage.

### 2. API helpers

Extend `web/src/lib/api.ts` with three new functions, mirroring the
existing `lookup`:

```ts
export function listMetros(init?: RequestInit): Promise<MetroSummary[]>
export function getMetro(slug: string, init?: RequestInit): Promise<MetroDetail>
export function listRecent(init?: RequestInit): Promise<Org[]>
```

When `import.meta.env.VITE_USE_FIXTURES === 'true'`, each function
short-circuits to the fixture module instead of calling the API. This
keeps the production code path identical and avoids leaking fixture
imports into prod bundles (Vite tree-shakes the fixture file when the
env var isn't set, but the explicit branch is the durable guarantee).

### 3. Query keys

Extend `web/src/lib/queryKeys.ts`:

```ts
export const queryKeys = {
  lookup: (postal_code: string, country: Country) =>
    ['lookup', postal_code, country] as const,
  metros: () => ['metros'] as const,
  metro: (slug: string) => ['metro', slug] as const,
  recent: () => ['recent'] as const,
} as const;
```

### 4. Fixtures (temporary)

New file `web/src/lib/fixtures/browse.ts`:

- Exports `metrosFixture: MetroSummary[]` with 4-6 entries covering
  US, CA, PT metros, varying org_counts.
- Exports `metroDetailFixture: Record<string, MetroDetail>` keyed by
  slug, with 2-3 keys mapping to seed-data slugs (`nyc-metro`,
  `lisbon-metro`, etc.).
- Exports `recentFixture: Org[]` with 5 entries.

All values are typed against `components['schemas']` so TypeScript
guarantees structural compatibility with the real wire shape. No
runtime validation needed.

The file is deleted in the post-backend-merge cleanup commit, along
with the `VITE_USE_FIXTURES` branches in `api.ts`.

### 5. Router additions

`web/src/router.tsx` gains two children of `App`:

```ts
{ path: 'browse', Component: Browse },
{ path: 'm/:metroSlug', Component: Metro },
```

Position them after the existing `r/:postalCode` route. No
`errorElement` per the existing convention (slice #15 territory).

### 6. Styling

No new CSS. Reuse:

- `.broadsheet-body` / `.col-lede` / `.col-aside` / `.aside-card`
  for any existing-page touches.
- `.page` / classified-section markup from `Results.tsx` for Browse
  and Metro.
- `.section-label` for small-caps section headers.
- `.dateline` for the metro country/state line.

If a new class is genuinely needed (e.g., `.metro-count-chip`), add it
to `web/src/styles/global.css` with a comment naming the slice. Default
to NO new classes; reach for them only if the broadsheet vocabulary
genuinely lacks the affordance.

## Acceptance criteria

- `just web-check` passes (lint, test, build, gen-check) **both** with
  `VITE_USE_FIXTURES=true` (during dev) **and** unset (against the real
  backend post-merge).
- `npm run dev` (fixtures on):
  - `/` shows two populated `aside` cards: 6 metros and 5 recent orgs.
  - `/browse` lists the fixture metros, descending by org count.
  - `/m/nyc-metro` lists the fixture detail for NYC, with org list.
  - `/m/totally-fake` renders an inline empty-state message, not an
    unhandled error.
- `npm run dev` (fixtures off, backend running):
  - All three behaviors above repeat with real seed data.
  - Network panel shows requests to `/api/v1/metros`, `/api/v1/recent`,
    `/api/v1/metros/{slug}`.
- New Vitest specs:
  - `Browse.test.tsx` — renders, list order, link behavior.
  - `Metro.test.tsx` — renders with orgs, 404 → empty state.
  - `Home.test.tsx` — IF not already present, asides render their
    queries. (If present, extend it.)
- Type check passes: all wire-shape consumption goes through
  `components['schemas']`.

## Non-goals (explicit)

- **No new dependencies.** Per CLAUDE.md, confirm before adding libs.
  Specifically: no MSW, no react-helmet, no router add-ons.
- **No homepage layout restructuring.** Existing two-column broadsheet,
  search box, drop-cap stay untouched. Only the inside of the two
  `aside-card` divs changes.
- **No 404 page.** Slice #15. `/m/:slug` for an unknown slug renders an
  inline empty-state, not a dedicated 404.
- **No org detail pages.** Deferred (`docs/roadmap.md:132`).
- **No `EntryList`/`Entry` refactor** unless it's pure extraction with
  zero behavior change on `/r/:postalCode`. Tests for the existing
  route must still pass.
- **No fixture investments beyond the slice.** MSW is the right
  long-term tool but adding it for a 2-week parallel window violates
  the dep-gate. Inline typed fixtures only.
- **No SSR / prerendering work.** Pure SPA per existing setup.
- **No CSS module migration.** Plain CSS via `global.css` per CLAUDE.md.

## File structure

### New

| Path | Responsibility |
|---|---|
| `web/src/routes/Browse.tsx` | The `/browse` page. |
| `web/src/routes/Browse.test.tsx` | Vitest + RTL coverage. |
| `web/src/routes/Metro.tsx` | The `/m/:metroSlug` page. |
| `web/src/routes/Metro.test.tsx` | Vitest + RTL coverage incl. 404. |
| `web/src/lib/fixtures/browse.ts` | Typed fixture data (temporary). |

### Modified

| Path | Change |
|---|---|
| `web/src/lib/api.ts` | Add `listMetros`, `getMetro`, `listRecent`; add `VITE_USE_FIXTURES` short-circuits. |
| `web/src/lib/queryKeys.ts` | Add `metros`, `metro`, `recent` key factories. |
| `web/src/router.tsx` | Add `/browse` + `/m/:metroSlug` routes. |
| `web/src/routes/Home.tsx` | Wire the two `aside-card` divs to `useQuery` + new helpers. |
| `web/src/routes/Home.test.tsx` (if exists) | Extend for asides; else create. |

### Deleted (cleanup commit, after backend lands)

| Path | Reason |
|---|---|
| `web/src/lib/fixtures/browse.ts` | Real backend live. |

Plus removal of `VITE_USE_FIXTURES` env branches in `api.ts` in the
same cleanup commit.

## Risks & mitigations

- **Fixture/real divergence.** Solution: every fixture value is typed
  against `components['schemas']`, so the compiler refuses any drift.
- **Async fixture branch leaking into prod bundle.** Mitigation: the
  `import.meta.env.VITE_USE_FIXTURES` branch is a static-string compare
  Vite tree-shakes; additionally the fixture module is only imported
  inside the truthy branch.
- **Homepage error state degrading UX.** If the API is unreachable on
  prod (deployment race, regional outage), the asides shouldn't break
  the page. Each `useQuery` consumer renders a graceful inline error
  fallback ("(retry)") and the rest of the page stays usable.
- **`/m/:slug` over-rendering on rapid slug changes.** Standard React
  Query caching covers it; no special handling needed.
