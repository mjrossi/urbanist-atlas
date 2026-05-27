# Browse + recent endpoints (slice #6)

**Status:** Shipped — the load-bearing pieces (downward DAG walk,
national-tier filter, `{meta, data}` envelope, hardcoded 10-row cap
on `/recent`) are still in production. Endpoint surface was renamed
twice after this spec landed:

- `/api/v1/metros` (this spec) → `/api/v1/places` (broadened to
  include city-kind regions) → `/api/v1/regions` (final, with the
  detail endpoint broadened to resolve any non-national slug).

Wherever this doc says `/metros`, `MetroSummary`, `MetroDetail`,
`ListMetros`, `GetMetro`, or `IsMetroKind`, the current names are
`/regions`, `RegionSummary`, `RegionDetail`, `ListRegions()`,
`GetRegion`, and `IsDefaultBrowseKind`. The `MetroKinds` predicate
*also* survives in `metro_kinds.go` but is narrower than
`DefaultBrowseKinds` — `/lookup`'s `placeLabel` keeps using it so a
Brooklyn ZIP still labels as "Brooklyn, NYC — New York Metro"
rather than picking the city as the broad ancestor.

The descendant-walk SQL, the `HAVING COUNT > 0` empties filter, and
the `scope_tier='national'` defensive prune in `OrgsForRegion`
(formerly `OrgsForMetro`) are unchanged.

**Supersedes:** none.
**Superseded-by (for endpoint naming + broadening):** the "Browse
goes generic" entry in [`docs/roadmap.md`](../../roadmap.md).
**Related:**
- [`2026-05-18-browse-pages-design.md`](./2026-05-18-browse-pages-design.md) (frontend half — slice #14, runs in parallel)
- [`docs/roadmap.md`](../../roadmap.md) (slice text)
- [`api/openapi.yaml`](../../../api/openapi.yaml) (current contract — `/api/v1/regions`)

## Why this exists

The homepage design in CLAUDE.md calls for a two-column broadsheet with
"Browse by metro" and "Recently added" panels in the right column.
Currently those `aside` blocks are "Coming soon" stubs
(`web/src/routes/Home.tsx:36-53`), and there is no `/browse` page. Both
gaps trace to three backend endpoints that are declared in
`api/openapi.yaml` but have no handlers.

This slice fills the three handlers:

- `GET /api/v1/metros` — list of metro-equivalent regions with org counts (`MetroSummary[]`)
- `GET /api/v1/metros/{slug}` — a single metro plus its approved orgs (`MetroDetail`)
- `GET /api/v1/recent` — the most recently approved orgs across the atlas (`Org[]`)

The contract is already locked. Schemas `MetroSummary` and `MetroDetail`
exist (`api/openapi.yaml:606-628`). The Go types are in
`api/internal/httpapi/oapi/types.gen.go`. **No spec edit, no regeneration
required.** This slice is pure implementation behind a stable contract.

The frontend half (slice #14) is in a parallel worktree and consumes the
same contract; the two halves do not share files.

## Strategic goal

Close the most visible gap in the v1 homepage so the atlas feels like a
browsable directory, not just a postal-code search bar. Browse / recent
also become the building blocks for any future "what's near me" or
"discover" features, but those are out of scope here.

## Design

### 1. Endpoint behaviors

#### `GET /api/v1/metros`

Returns every region whose `kind` is in the metro-equivalent set and that
has **at least one approved organization** (directly tagged or
inherited via the region graph), with an `org_count`. The empty-result
case is `[]`, not 404.

**Ordering:** `org_count DESC, region.name ASC` (largest metros first;
alphabetical tiebreak for stability).

**No pagination.** With ~30–50 total orgs across all supported
countries (`docs/roadmap.md:94`), the metro count is bounded in the low
tens for v1. Pagination is a future-slice concern.

#### `GET /api/v1/metros/{slug}`

Returns one metro region plus the approved orgs that serve it
(directly *or* via the region graph — same ancestor resolution as
`/lookup`, but rooted at a single region rather than a postal code).
404 if the slug doesn't exist or isn't a metro-equivalent kind. 200
with empty `orgs` array if the metro exists but has zero orgs (won't
happen given `/metros` filters those out, but the handler shouldn't
assume it).

**Org ordering:** newest-approved first (consistent with `/recent`).

#### `GET /api/v1/recent`

Returns the **10** most recently approved organizations across the
whole atlas, ordered by `created_at DESC`. National-tier orgs are
excluded (consistent with the default `/lookup` filter from slice
#4.6).

**No `limit` query param.** The contract doesn't declare one, and
opening it would require an OpenAPI spec edit — explicitly out of
scope. Hardcode 10 in the SQL, document it in the handler GoDoc.

### 2. Metro-kind set (the "stable kind suffix" decision)

The roadmap calls for deriving the metro set from "a stable kind suffix
rather than a hardcoded enum." Three plausible readings:

1. `endsWith(":metro")` — naive suffix match. Excludes `ca:cma`,
   `ca:regional-district`, `pt:area-metropolitana`. Wrong.
2. Curated `WHERE kind IN ('us:metro','ca:cma','ca:regional-district','pt:area-metropolitana')` —
   hardcoded enum buried in SQL. Also wrong, per the roadmap.
3. A named predicate `IsMetroKind(RegionKind) bool` in `pkg/atlas`,
   driven by a small curated `var metroKinds = map[string]bool{...}`,
   that handler + SQL both consult. Adding a new country's
   metro-equivalent is a one-line append.

**Decision:** option 3. Lives at `api/pkg/atlas/metro_kinds.go`:

```go
// MetroKinds names the region kinds that count as "metro-equivalent"
// for the purpose of /api/v1/metros and the homepage Browse panel.
// This set is editorial, not derived from a suffix pattern, because the
// administrative geographies that qualify don't share a stable
// lexical suffix.
//
// In: us:metro (MSA/CSA), ca:cma (Census Metropolitan Area),
// ca:regional-district (BC's unique multi-municipal layer that plays
// a metro role), pt:area-metropolitana (AML, AMP).
//
// Out: states/provinces, distritos, NUTS regions, autonomous communities,
// national tier. Adding a new country's metro-equivalent kind is a
// one-line append here.
var metroKinds = map[atlas.RegionKind]bool{
    "us:metro":              true,
    "ca:cma":                true,
    "ca:regional-district":  true,
    "pt:area-metropolitana": true,
}

func IsMetroKind(k atlas.RegionKind) bool { return metroKinds[k] }

func MetroKinds() []atlas.RegionKind {
    out := make([]atlas.RegionKind, 0, len(metroKinds))
    for k := range metroKinds {
        out = append(out, k)
    }
    sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
    return out
}
```

`MetroKinds()` is the slice handed to the SQL query as `ANY($1::text[])`.
The sort ensures deterministic SQL plans.

### 3. Storage

New sqlc queries in `api/internal/store/postgres/queries/browse.sql`:

- `ListMetros` — joins `regions` with a count of `organization_regions`
  (after walking the region DAG, since an org tagged to "Brooklyn" should
  count toward "NYC metro"). Filter by `kind = ANY($1)` and
  `scope_tier <> 'national'`. Group by region, count distinct org IDs,
  order by count DESC then name ASC.
- `GetMetro` — single region by slug + kind check (returns NULL row if
  not a metro-equivalent, surfaces as 404 in the handler).
- `OrgsForMetro` — ancestor-walk the metro region downward (descendants
  in the DAG) and return distinct approved orgs tagged to any descendant.
  Order by `created_at DESC`.
- `ListRecent` — top 10 approved orgs by `created_at DESC`, excluding any
  org whose ONLY region attachments are `scope_tier='national'`.

For "approved orgs": v1 has no `status` column on `organizations` (only
`submissions` have status). All rows in `organizations` are approved by
construction (they got there via seed or via an admin promote). Today
this means "all orgs."

### 4. Store interface

`atlas.Store` (in `api/pkg/atlas/store.go`) gains three methods:

```go
ListMetros(ctx context.Context) ([]MetroSummary, error)
GetMetro(ctx context.Context, slug string) (*MetroDetail, error) // nil if not found
ListRecent(ctx context.Context) ([]Org, error)
```

`*MetroDetail` (pointer) so `nil` cleanly signals not-found without an
extra `(bool, error)` return.

The postgres implementation lives in
`api/internal/store/postgres/store.go`, alongside the existing
`Lookup` method.

### 5. HTTP handlers

Three thin handlers in `api/internal/httpapi/`, each ~10 lines, mirroring
`lookup.go`:

- `metros.go` — `listMetrosHandler(store, logger)` + `getMetroHandler(store, logger)`
- `recent.go` — `recentHandler(store, logger)`

Each handler:

1. Parses request (slug param for `getMetro`, none otherwise).
2. Calls the store method.
3. Encodes the response as JSON, sets `Content-Type: application/json`.
4. On `*MetroDetail == nil`, returns 404 via the existing
   `writeProblem` helper from `problem.go`.
5. On error, returns 500 via the same helper.

### 6. Router wiring

`api/internal/httpapi/router.go` adds three routes inside the
`/api/v1` group:

```go
r.Get("/metros", listMetrosHandler(cfg.Store, logger))
r.Get("/metros/{slug}", getMetroHandler(cfg.Store, logger))
r.Get("/recent", recentHandler(cfg.Store, logger))
```

## Acceptance criteria

- `just api-check` passes (gofmt, vet, staticcheck, tests, gen-check).
- `just pg-reset && just migrate-up && just seed && just api-run` then:
  - `curl -s :8080/api/v1/metros | jq 'length'` returns ≥ 1.
  - `curl -s :8080/api/v1/metros | jq '.[0]'` returns an object matching
    `MetroSummary` (has `region` and `org_count`).
  - `curl -s :8080/api/v1/metros | jq -e '[.[] | .org_count] | (length>0) and (.[0] >= .[-1])'`
    confirms descending org_count ordering.
  - `curl -s :8080/api/v1/metros/nyc-metro | jq '.region.slug'` returns `"nyc-metro"`.
  - `curl -s :8080/api/v1/metros/nyc-metro | jq '.orgs | length'` returns ≥ 1.
  - `curl -sf :8080/api/v1/metros/does-not-exist` exits non-zero (404)
    with a `problem+json` body.
  - `curl -s :8080/api/v1/recent | jq 'length'` returns a value `≤ 10` and
    `≥ 1`.
- New `httptest` integration tests in `api/internal/httpapi/metros_test.go`
  and `recent_test.go` exercise: success path, 404 path (metros only),
  national-tier filter (recent), ordering.
- `MUBi` (the PT national-tier org seeded in #4.6) MUST NOT appear in
  `/recent` results.

## Non-goals (explicit)

- **No openapi.yaml edits.** Contract is complete. Confirm with
  `just api-oapi-gen` produces no diff before starting.
- **No `limit` query param** on `/recent` (would require a spec edit).
- **No pagination** on `/metros`.
- **No `Cache-Control`** headers (Phase-2 concern; touched by slice #24
  if/when the response wrapper middleware lands).
- **No new endpoint registrations** beyond the three.
- **No memory store**. Postgres is the only `atlas.Store` implementation
  today. `pipeline_test.go` uses testcontainers; the new handler tests
  follow that pattern.
- **No homepage data flow changes** beyond what `/metros` + `/recent`
  return. Anything UX is in slice #14.

## File structure

### New

| Path | Responsibility |
|---|---|
| `api/pkg/atlas/metro_kinds.go` | `IsMetroKind` predicate + `MetroKinds()` accessor. |
| `api/pkg/atlas/metro_kinds_test.go` | Unit tests for the predicate (asserts the four currently-supported kinds + a known non-metro kind). |
| `api/internal/httpapi/metros.go` | `listMetrosHandler`, `getMetroHandler`. |
| `api/internal/httpapi/metros_test.go` | httptest coverage. |
| `api/internal/httpapi/recent.go` | `recentHandler`. |
| `api/internal/httpapi/recent_test.go` | httptest coverage (incl. national-filter regression). |
| `api/internal/store/postgres/queries/browse.sql` | `ListMetros`, `GetMetro`, `OrgsForMetro`, `ListRecent`. |
| `api/internal/store/postgres/gen/browse.sql.go` | sqlc-generated (committed). |

### Modified

| Path | Change |
|---|---|
| `api/pkg/atlas/store.go` | Add `ListMetros`, `GetMetro`, `ListRecent` to `Store` interface; add `MetroSummary`, `MetroDetail` types if not already present. |
| `api/internal/store/postgres/store.go` | Implement the three new methods. |
| `api/internal/httpapi/router.go` | Wire the three new routes. |

## Risks & mitigations

- **Org-count derivation drift.** If the metros count uses
  `organization_regions` directly without DAG walking, a Brooklyn-only
  org won't count toward NYC metro. The query MUST descend through
  `region_parents` (recursive CTE) the same way `/lookup` ascends. The
  existing `AncestorRegions` CTE in `lookup.sql` is the pattern; rename
  the direction for `DescendantRegions` here.
- **Empty seed data.** If seed orgs don't tag any metro, `/metros`
  returns `[]` even though the underlying regions exist. The handler
  should return 200 + `[]`, not an error. The acceptance criteria
  assume the existing seed (which does tag NYC, SF, Vancouver, Toronto,
  Lisbon orgs).
- **National-tier in `/recent`.** A regression of the slice-#4.6 filter
  would surface MUBi in `/recent`. The test in `recent_test.go` MUST
  explicitly assert the absence of any national-tier org.
