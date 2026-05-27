# API architecture

The Go-side architecture of Urbanist Atlas. Read this when adding a
new endpoint, a new middleware, a new error type, or otherwise
extending the API.

This doc is evergreen. Point-in-time design specs (slice rationales,
the original library split decision, the ODbL wrapping shape
debate) live under [`docs/superpowers/specs/`](./superpowers/specs/);
roadmap status lives in [`docs/roadmap.md`](./roadmap.md). Both move
faster than this file.

## Library-first split

Three packages, in dependency order:

```
api/pkg/atlas/          ← importable library. No HTTP, no chi, no env vars.
api/internal/store/     ← Store implementations (MemStore in pkg, Postgres here).
api/internal/httpapi/   ← chi handlers + middleware. Thin wrappers.
api/cmd/server/         ← urfave/cli main. Wires flags → Config → New(...).
```

Rules:

- **`pkg/atlas`** owns every domain type (`Org`, `Region`,
  `LookupResult`, `Country`, `ScopeTier`, …) and every algorithm
  (`Lookup`, `placeLabel`, `NormalizePostalCode`, sort buckets). It
  also owns the `atlas.Store` interface. Nothing in this package
  imports anything else from the project; a future `cmd/atlas` CLI
  consumes it directly.
- **`internal/httpapi`** owns chi handlers, middleware, request
  parsing, response encoding, and the oapi-codegen-shaped wire
  types under `internal/httpapi/oapi/`. Handlers stay around
  ten lines: parse the request → call a `pkg/atlas` function →
  encode the result. No business logic here.
- **`cmd/server`** is the urfave/cli entry. Subcommands: `serve`,
  `migrate {up|down|status}`, `loadregions`, `loadpostal`, `seed`,
  `loaddata`, `etl {download|regenerate}`. Each subcommand is a
  thin assembly: read flags + env → build a `Config` → call into
  `pkg/atlas` or `internal/httpapi`.

The split lets the same business logic back the JSON API, an
offline CLI (loaders, ETL, the planned `cmd/atlas`), and any
future consumer. It also keeps tests honest — `pkg/atlas` tests
can't quietly depend on chi or net/http, which forces the
abstraction to stay clean.

## Store abstraction

`atlas.Store` ([`api/pkg/atlas/store.go`](../api/pkg/atlas/store.go))
is the persistence seam. Seven methods compose to satisfy every
endpoint:

| Method | Used by | Notes |
|---|---|---|
| `ResolveLeafRegion(country, postalCode)` | `/lookup` | Returns `ErrPostalCodeNotFound` for unknown codes. |
| `AncestorRegions(leafID)` | `/lookup` | Walks the region DAG **upward**; dedupes diamonds. |
| `OrgsForRegions(regionIDs)` | `/lookup` | Hydrates each org's full attachment list. |
| `ListRegions()` | `/regions` | Regions in the default browse set (metros + cities, per `atlas.DefaultBrowseKinds`) with ≥1 attached org; walks the DAG **downward** from each match; excludes national-tier. Each summary also carries a `browse_parent_slug` — the SPA's grouping hook for nesting cities under their parent metro. The endpoint ships without filter parameters — the right axis (taxonomy via `kind`, DAG via `ancestor`, …) gets designed when a concrete browse UI use case appears. |
| `GetRegion(slug)` | `/regions/{slug}` | Resolves any non-national region. Returns `(nil, nil)` for unknown or national-tier slugs. Builds a **lookup-style scope** by walking both ancestors (upward) and descendants (downward) from the focus, then bucketing orgs by attachment `scope_tier` via `atlas.BucketOrgsByScope` — same rule `/lookup` uses. Detail responses carry `local: LookupOrg[]`, `regional: LookupOrg[]`, and `ancestry: Region[]` (closest-first, excludes self + national). Net effect: clicking SF from Browse returns the same set of advocates `/lookup` returns for an SF ZIP. |
| `GetOrgBySlug(slug)` | `/orgs/{slug}` | Returns `ErrOrgNotFound` for unknown or non-approved slugs. |
| `ListRecent()` | `/recent` | Hardcoded cap of 10; excludes national-only orgs. |

`/lookup` and `/regions/{slug}` share rendering primitives (the
Local/Regional bucketing) but walk the DAG differently:

- **`/lookup`** is keyed by a postal code → leaf region. It walks
  **upward** from the leaf (and only upward), so the result is
  "orgs serving the resolved point." A Naperville ZIP correctly
  excludes Chicago-city-only orgs because Chicago is a sibling
  subtree, not an ancestor.
- **`/regions/{slug}`** is keyed by a region slug. It walks
  **both directions** from the focus, so the result is "orgs in
  scope for this region as a unit." Browsing `/regions/chicago-metro`
  pulls in Chicago city orgs (descendants), Chicagoland orgs
  (ancestor), and orgs covering Illinois (ancestor too) — same set
  Lookup would surface for any Chicago ZIP.

The `/regions` list endpoint's `org_count` stays purely a
downward-walk number — preserving the editorial differentiation
between "Chicago Metro" (4) and "Chicago" (3) on the index. The
expanded lookup-style scope only kicks in when a user lands on the
detail page.

Two implementations:

- **`atlas.MemStore`** ([`api/pkg/atlas/memstore.go`](../api/pkg/atlas/memstore.go)) —
  in-process fixtures. Used by handler tests, the
  `--store=memory` runtime flag, and any offline `pkg/atlas`
  exploration. Loaded via `atlas.LoadDevFixtures(store)`.
- **`postgres.Store`** ([`api/internal/store/postgres/`](../api/internal/store/postgres/)) —
  the production store. Queries are written in SQL under
  `queries/`, code-generated to Go via sqlc (regenerate with
  `just api-sqlc-gen`), driven by pgx. `serve --store=postgres`
  (the default) selects it; `DATABASE_URL` configures it.

Postgres-backed implementations can optimize internally — e.g. fold
`AncestorRegions` + `OrgsForRegions` into a single recursive CTE —
without changing the interface contract. Tests against MemStore
verify behavior; integration tests against testcontainers Postgres
verify the SQL.

## Wire contract

[`api/openapi.yaml`](../api/openapi.yaml) is the single source of
truth for every request and response shape. Both halves of the app
generate types from it; neither hand-rolls structs.

```
api/openapi.yaml
├─ oapi-codegen ──► api/internal/httpapi/oapi/types.gen.go   (Go)
└─ openapi-typescript ──► web/src/lib/api.gen.ts             (TS)
```

Two ergonomic consequences:

1. **Embedded copy** — `go:embed` can't escape its source file's
   package, so a synchronized duplicate lives at
   `api/internal/httpapi/openapi.yaml`. A `//go:generate` directive
   keeps it in sync; `TestEmbeddedOpenAPISpecMatchesCanonical`
   fails fast if they drift; `just api-gen-check` enforces this in
   CI.
2. **Runtime discovery** — the embedded copy is served at
   `GET /api/v1/openapi.yaml` (content-type `application/yaml`) so
   external consumers can fetch the contract without cloning the
   repo. This endpoint is bypass-listed from the Phase 1 client-
   secret gate.

Workflow when editing the spec:

```sh
# 1. Edit api/openapi.yaml.
# 2. Regenerate both halves.
just api-oapi-gen   # → embedded copy + Go types
just web-oapi-gen   # → TS types
# 3. Commit spec + regenerated artifacts together.
```

## Error envelope — RFC 9457 problem+json

Every error response is an
[RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)
document with `Content-Type: application/problem+json`. The shape
matches the `ProblemDetails` schema in `openapi.yaml`:

```json
{
  "type":       "https://urbanistatlas.com/problems/not-found",
  "title":      "Not Found",
  "status":     404,
  "detail":     "postal code not found — try a nearby code, …",
  "instance":   "/api/v1/lookup",
  "request_id": "a3f9b2c1d4e5f607"
}
```

`request_id` is an extension echoing the inbound `X-Request-ID`
header (or a fresh hex string when none was provided). Users can
quote it when reporting bugs; server logs are keyed on the same id.

Stable problem-type URIs live as constants in
[`api/internal/httpapi/problem.go`](../api/internal/httpapi/problem.go).
Current catalog:

| Constant | URI | Emitted by |
|---|---|---|
| `problemValidation` | `…/problems/validation` | `/lookup` parameter errors |
| `problemNotFound` | `…/problems/not-found` | `/lookup`, `/regions/{slug}`, `/orgs/{slug}` unknown ids |
| `problemInternal` | `…/problems/internal` | recoverer middleware + store errors |
| `problemUnauthorized` | `…/problems/unauthorized` | Phase 1 client-secret gate |

To add a new problem type:

1. Add the URI constant to `problem.go`.
2. Mirror the type into `openapi.yaml`'s `ProblemDetails`
   description (the URI catalog is part of the wire contract).
3. Call `writeProblem(...)` from the handler that needs it.

The recoverer middleware
([`api/internal/httpapi/middleware.go`](../api/internal/httpapi/middleware.go))
maps any handler panic to `problemInternal` so unhandled errors
stay structured. Don't write `http.Error(...)` anywhere in the
data path — that bypasses the contract.

## Response envelope — ODbL attribution

Every success response under `/api/v1/**` carries two headers:

```
X-Data-License:     ODbL-1.0
X-Data-Attribution: https://urbanistatlas.com
```

set unconditionally by `odblHeadersMiddleware`
([`api/internal/httpapi/odbl.go`](../api/internal/httpapi/odbl.go))
inside the `/api/v1` route group. `/healthz` is intentionally
exempt; it's a liveness probe, not a data endpoint.

Collection responses additionally wrap their payload in a
`{ meta, data }` envelope:

```json
{
  "meta": {
    "license":         "ODbL-1.0",
    "attribution_url": "https://urbanistatlas.com",
    "generated_at":    "2026-05-21T12:34:56Z"
  },
  "data": [ … ]
}
```

The wrapper helper is `respondCollection[T any](w, items)`. Use it
from any list handler (`/regions`, `/recent`). Single-resource
handlers (`/lookup`, `/regions/{slug}`, `/orgs/{slug}`) keep using
`writeJSON(...)` directly — there's no useful meta to attach to a
scalar response, and the headers already carry attribution.

`data` is coerced to `[]` (never `null`) when the input slice is
nil, so clients can assume `data` is always an array.

## Phase 1 gate — `X-Atlas-Client`

Until Phase 2 (slice #26) ships per-user API keys, the data
endpoints under `/api/v1/**` (except `/openapi.yaml`) require a
shared `X-Atlas-Client` header. The expected secret comes from
`URBANIST_CLIENT_SECRET`; the SPA bundles it from
`VITE_API_CLIENT_SECRET`.

`clientSecretMiddleware`
([`api/internal/httpapi/clientsecret.go`](../api/internal/httpapi/clientsecret.go))
gates the route group. Implementation notes:

- Empty secret = no-op. Local dev with neither env var set works.
- Comparison via `subtle.ConstantTimeCompare`.
- Mismatch → 401 with `problemUnauthorized` + `Content-Type: application/problem+json`.
- Bypass list: `/healthz` (root level) and `/api/v1/openapi.yaml`
  (registered before the gated group). Liveness probes and
  contract discovery don't carry the secret.

The middleware is a deterrent against casual scrapers, not a
security boundary. The secret is shipped in the SPA bundle and
visible in devtools. Phase 2 replaces it with per-user keys + rate
limiting; see slices #26–#28 in [`docs/roadmap.md`](./roadmap.md).

## Middleware order

`internal/httpapi/router.go` composes the chain. Order is
deliberate:

```go
r.Use(requestIDMiddleware)        // every later layer sees the same rid
r.Use(recovererMiddleware(logger)) // panics in business code stay structured
r.Use(loggingMiddleware(logger))   // access log records the final status
r.Use(corsMiddleware(...))         // CORS preflights short-circuit before /api/v1
```

Then the `/api/v1` subtree adds:

```go
r.Use(odblHeadersMiddleware)      // attribution on every data response
// /openapi.yaml registered BEFORE the gated group
r.Group(func(r chi.Router) {
    r.Use(clientSecretMiddleware(cfg.ClientSecret)) // Phase 1 gate
    // handlers
})
```

Two ordering rules to preserve when extending:

- `requestIDMiddleware` must stay first — every error envelope
  carries the rid, and every log line keys on it.
- `recovererMiddleware` must wrap before any business handler so
  panics in `pkg/atlas` or the store don't escape.

## How to add an endpoint

1. Edit `api/openapi.yaml` — request params, response schema, error
   types. Commit this in its own PR if it's a significant shape
   change.
2. `just api-oapi-gen` to regenerate Go types + embedded copy.
3. `just web-oapi-gen` to regenerate TS types (if the SPA will
   consume it).
4. Add the business logic to `pkg/atlas` — a new function on the
   `Store` interface plus its `MemStore` implementation, or a new
   pure function on existing data. Write a unit test against
   `MemStore` fixtures.
5. Add the handler to `internal/httpapi/` — parse → call atlas →
   encode. Keep it ~10 lines. Write an httptest+MemStore handler
   test asserting the wire shape and status codes.
6. Add the Postgres-side query in `internal/store/postgres/queries/`,
   regenerate with `just api-sqlc-gen`, and write a testcontainers
   integration test under `//go:build integration`.
7. Wire the handler in `router.go`. Inside the gated group unless
   it's a discovery endpoint like `/openapi.yaml`.
8. If it's a collection, use `respondCollection[T]` so the meta
   envelope lands consistently.

For testing conventions across the three tiers, see
[`docs/testing-strategy.md`](./testing-strategy.md).
