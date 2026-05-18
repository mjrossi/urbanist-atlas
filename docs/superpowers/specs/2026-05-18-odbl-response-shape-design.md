# ODbL response shape (slice #24)

**Status:** Active — implementation of slice #24 (ODbL attribution headers + collection meta envelope).
**Supersedes:** none.
**Related:**
- [`docs/roadmap.md:134`](../../roadmap.md) (slice text)
- [`LICENSE-DATA`](../../../LICENSE-DATA) (the dataset license being announced)
- [`api/openapi.yaml`](../../../api/openapi.yaml) (contract to update)
- [`web/src/lib/api.ts`](../../../web/src/lib/api.ts) (frontend client that has to keep working)

## Why this exists

`LICENSE-DATA` declares the Urbanist Atlas dataset to be ODbL-1.0. The
roadmap (slice #24) commits to making that license obligation visible
**in-band, on every API response**, so downstream consumers can't miss
the share-alike requirement.

Two surfaces:

1. **Response headers** on every `/api/v1/**` success response:
   - `X-Data-License: ODbL-1.0`
   - `X-Data-Attribution: https://urbanistatlas.com`
2. **Meta envelope** on collection responses with `license`,
   `attribution_url`, `generated_at` (RFC3339), so the share-alike
   obligation also lives inside the body for clients that strip
   headers in transit (Cloudflare proxies, mobile SDKs that surface
   only the JSON body, etc.).

This is a **wire-contract change** but a backward-compatible one for
the project's own frontend: the typed client (`web/src/lib/api.ts`)
unwraps `data` once inside `listMetros` / `listRecent`, so consumers
(`Home`, `Browse`, `Metro`, `Results`) need zero changes.

## Strategic goal

Make ODbL attribution permanent and structural, not advisory. The
license obligation rides every successful response in two
complementary forms (headers + envelope) so a downstream consumer
would have to actively strip both layers to lose track of it.

## Design

### 1. Response headers (the cheap, universal half)

A new middleware `odblHeadersMiddleware` sets two headers on every
response under `/api/v1/**`:

- `X-Data-License: ODbL-1.0`
- `X-Data-Attribution: https://urbanistatlas.com`

The middleware runs **after** CORS in the chain so the headers stay
on the response when CORS strips down OPTIONS preflights. The
middleware applies headers **before** delegating to `next.ServeHTTP`
so that when handlers call `WriteHeader(200)`, the headers are
already buffered. (chi flushes headers on the first `Write`, so order
matters.)

**Path-scoping:** the middleware is mounted inside `r.Route("/api/v1",
...)` rather than at the router root, so `/healthz` does NOT get the
ODbL headers (it's not data — it's a liveness probe). This matches
the spec's "every `/api/v1/**` success response" language.

**Why headers on errors too?** Simpler. Distinguishing 2xx from non-2xx
in middleware requires response-status sniffing, and an error response
that quotes the dataset license is harmless and arguably more honest
than one that silently strips the obligation. The roadmap says
"success response" but the operational cost of headers-on-all-status
is zero. We set them unconditionally.

**Why not on `/api/v1/openapi.yaml`?** It's served at `/api/v1/openapi.yaml`,
so it's inside the mounted path and will get the headers. That's the
correct behavior: the OpenAPI spec is part of the data product, and
the headers cost nothing on a YAML response.

### 2. Meta envelope on collection responses (the structural half)

**Shape:**

```json
{
  "meta": {
    "license": "ODbL-1.0",
    "attribution_url": "https://urbanistatlas.com",
    "generated_at": "2026-05-18T12:34:56Z"
  },
  "data": [ ... ]
}
```

The field name `data` (not `items` or `results`) follows JSON:API and
Stripe convention.

**Where it applies:** **collection responses only.** Today that means:

- `GET /api/v1/metros` — was `MetroSummary[]`, becomes
  `{ meta: Meta, data: MetroSummary[] }`
- `GET /api/v1/recent` — was `Org[]`, becomes
  `{ meta: Meta, data: Org[] }`

**Where it does NOT apply:**

- `GET /api/v1/lookup` (returns `LookupResult` — a container with
  `local` / `regional` arrays inside, not a list).
- `GET /api/v1/metros/{slug}` (returns `MetroDetail` — single
  resource).
- `GET /healthz` (text, not JSON).
- `GET /api/v1/openapi.yaml` (YAML).
- Error responses (`application/problem+json`).

The rule for "is this a collection": the response top-level is a JSON
array. Adding a new list endpoint? Use the envelope. Adding a new
container-with-arrays endpoint? Don't.

### 3. `generated_at` semantics

Per-request `time.Now().UTC()` formatted as RFC3339 (`time.RFC3339`).

**Why per-request:** simplest, no caching layer to invalidate, matches
the "what time was this response generated?" reading of the field name.
The cost of `time.Now()` per response is irrelevant.

**No nanosecond precision.** RFC3339 in Go's stdlib defaults to second
precision (`2006-01-02T15:04:05Z07:00`). That's appropriate for an
attribution timestamp.

### 4. Wrapping strategy: explicit helper, not body re-parse

Two strategies were considered:

| Strategy | Pros | Cons |
|---|---|---|
| **Middleware re-parses + re-encodes body** | Handlers don't change. | Fragile (encoding twice, breaks streaming, hides where wrapping happens). |
| **Explicit `respondCollection(w, items)` helper** | Wrapping is visible at the handler. | Touches each collection handler (~10 lines each). |

**Decision: explicit helper.** Handlers go from
`writeJSON(w, 200, toOAPIMetroSummaries(metros))` to
`respondCollection(w, toOAPIMetroSummaries(metros))`. The wrapping
is obvious at the call site, no body re-parsing, no surprises in
production. The headers stay in middleware (universally applied).

### 5. Helper signature and home

The helper lives in a new file `api/internal/httpapi/odbl.go`
alongside the middleware. One file, two related concerns.

```go
// odbl.go (sketch)

const (
    dataLicense        = "ODbL-1.0"
    dataAttributionURL = "https://urbanistatlas.com"
)

// odblHeadersMiddleware sets the ODbL attribution headers on every
// response it sees. Mount inside the /api/v1 route group, after CORS.
func odblHeadersMiddleware(next http.Handler) http.Handler { ... }

// respondCollection writes a JSON envelope with the ODbL meta block
// and the items as `data`. Used by every list endpoint.
//
// `items` is encoded as-is — callers should pass already-OAPI-typed
// slices (e.g. []oapi.MetroSummary). T uses generics so the call
// site preserves its typed slice without an explicit any cast.
func respondCollection[T any](w http.ResponseWriter, items []T) { ... }

// newMeta returns the meta block for a single response: license,
// attribution URL, and a fresh RFC3339 timestamp.
func newMeta() oapi.Meta { ... }
```

`respondCollection` uses generics (`[T any]`) so handler code keeps
the typed slice without losing type information. The body is encoded
as `oapi.MetroSummariesEnvelope` / `oapi.RecentEnvelope` (generated
types from the openapi.yaml change — see §6).

**Nil-safe slices:** `respondCollection` MUST ensure the encoded
`data` field is a JSON array, not `null`. Callers already build their
slices with `make([]T, 0, n)` (see `toOAPIMetroSummaries` and
`toOAPIOrgs`), so the helper doesn't need to re-check; but a defensive
`if items == nil { items = []T{} }` inside the helper costs nothing
and prevents future regressions.

### 6. OpenAPI changes

The spec changes are surgical:

1. **New `Meta` schema** under `components/schemas`:

   ```yaml
   Meta:
     type: object
     description: |
       Attribution block included on every collection response.
       Carries the ODbL license obligation in-band so consumers
       that strip headers still see the share-alike requirement.
     required: [license, attribution_url, generated_at]
     properties:
       license:
         type: string
         example: ODbL-1.0
         description: |
           SPDX identifier of the data license. Stable for the
           lifetime of v1 (`ODbL-1.0`).
       attribution_url:
         type: string
         format: uri
         example: https://urbanistatlas.com
         description: |
           URL downstream consumers should link to when crediting
           the source.
       generated_at:
         type: string
         format: date-time
         example: "2026-05-18T12:34:56Z"
         description: |
           RFC 3339 timestamp recording when the response was
           produced server-side. Per-request — there is no caching
           layer in v1.
   ```

2. **Two new envelope schemas** (one per collection endpoint) under
   `components/schemas`:

   ```yaml
   MetroSummariesEnvelope:
     type: object
     description: Collection envelope for `GET /api/v1/metros`.
     required: [meta, data]
     properties:
       meta: { $ref: '#/components/schemas/Meta' }
       data:
         type: array
         items: { $ref: '#/components/schemas/MetroSummary' }

   RecentEnvelope:
     type: object
     description: Collection envelope for `GET /api/v1/recent`.
     required: [meta, data]
     properties:
       meta: { $ref: '#/components/schemas/Meta' }
       data:
         type: array
         items: { $ref: '#/components/schemas/Org' }
   ```

   **Why two named envelopes instead of one generic?** OpenAPI 3.0
   doesn't support generic schemas. The two named schemas keep the
   generated Go and TS types clean (each gets a concrete struct/type
   rather than `Envelope<T>` workarounds). When a new collection
   endpoint arrives, it adds its own `*Envelope` schema — three
   lines plus a `$ref`. The repetition is shallow and worth it for
   strongly-typed code generation.

3. **Two new global response headers** declared in `components/headers`
   and referenced from each path operation's 200 response:

   ```yaml
   components:
     headers:
       XDataLicense:
         description: |
           SPDX identifier of the dataset license. Stable for the
           lifetime of v1.
         schema:
           type: string
           example: ODbL-1.0
       XDataAttribution:
         description: |
           URL downstream consumers should link to when crediting
           the source.
         schema:
           type: string
           format: uri
           example: https://urbanistatlas.com
   ```

   Then on every `/api/v1/**` 200 response declaration:

   ```yaml
   responses:
     '200':
       description: ...
       headers:
         X-Data-License:
           $ref: '#/components/headers/XDataLicense'
         X-Data-Attribution:
           $ref: '#/components/headers/XDataAttribution'
       content:
         application/json:
           schema:
             $ref: '#/components/schemas/...'
   ```

4. **Update the `/api/v1/metros` and `/api/v1/recent` 200 schema
   references** to point at `MetroSummariesEnvelope` /
   `RecentEnvelope` instead of the bare array types.

5. **Documentation pass** on the spec's top-level `info.description`
   to mention the headers + envelope convention alongside the
   existing `X-Request-ID` paragraph.

**`info.version` is NOT bumped.** Per the worktree instructions and
existing pattern (slice #6 didn't bump it either). The version field
is reserved for human-visible API version bumps.

### 7. Frontend unwrap

`web/src/lib/api.ts` already has `apiFetch<T>` returning a parsed
JSON body as `T`. The wrappers `listMetros` and `listRecent` change
their internal type parameter to the envelope and unwrap before
returning:

```ts
import type { MetroSummariesEnvelope, RecentEnvelope } from './api.gen';

export function listMetros(init?: RequestInit): Promise<MetroSummary[]> {
  return apiFetch<MetroSummariesEnvelope>('/api/v1/metros', init).then(
    (env) => env.data,
  );
}

export function listRecent(init?: RequestInit): Promise<Org[]> {
  return apiFetch<RecentEnvelope>('/api/v1/recent', init).then(
    (env) => env.data,
  );
}
```

The public return types stay `Promise<MetroSummary[]>` /
`Promise<Org[]>` so `Home`, `Browse`, and any consumer code remain
unchanged. The envelope types are re-exported from `api.ts` (so
downstream code that wants to read `meta.generated_at` can, but the
existing consumers don't have to).

**`apiFetch` itself does NOT unwrap.** It stays generic — some
endpoints don't have envelopes (e.g. `getMetro`, `lookup`), and
forcing unwrap at that layer would break them. Unwrapping is the
typed helper's job.

### 8. Tests

**Backend:**

- New `api/internal/httpapi/odbl_test.go`:
  - `TestODbLHeaders_PresentOnAPISuccessResponse` — GET `/api/v1/metros`,
    assert both headers present with expected values.
  - `TestODbLHeaders_AbsentOnHealthz` — GET `/healthz`, assert neither
    header is set (regression guard for path-scoping).
  - `TestODbLHeaders_PresentOnAPIErrorResponse` — GET
    `/api/v1/metros/totally-bogus` (a 404), assert both headers
    present. (Documents the "headers on every response under /api/v1,
    success or not" decision.)
  - `TestRespondCollection_WrapsItemsAndSetsHeaders` — direct unit
    test of `respondCollection` against a `httptest.ResponseRecorder`,
    asserting the envelope shape and the `Content-Type:
    application/json; charset=utf-8` header (re-using `writeJSON`).
  - `TestRespondCollection_NilSlice_EncodesEmptyArray` — pass
    `nil` to `respondCollection`, assert the `data` field is `[]`
    not `null`.
  - `TestNewMeta_EmitsRFC3339UTC` — call `newMeta()`, parse
    `generated_at` with `time.RFC3339`, assert UTC zone and
    `time.Since()` is small (< 5s).
- Updates to `metros_test.go` and `recent_test.go`: each existing
  happy-path test that decodes `[]oapi.MetroSummary` /  `[]oapi.Org`
  changes to decode the envelope, asserts `meta.license == "ODbL-1.0"`,
  `meta.attribution_url == "https://urbanistatlas.com"`, and parses
  `meta.generated_at` as RFC3339.

**Frontend:**

- Update `web/src/lib/api.test.ts` so the existing
  `listMetros` / `listRecent` tests return envelope-shaped JSON from
  the fetch mock and still see the unwrapped array in the resolved
  promise. Add one test per helper asserting unwrapping works on a
  non-empty body too.

### 9. Router wiring

`api/internal/httpapi/router.go` gains one line inside the
`r.Route("/api/"+apiVersion, ...)` block, before any route declarations:

```go
r.Use(odblHeadersMiddleware)
```

Order inside the group: ODbL headers run after the outer
`corsMiddleware` (which is mounted on the parent router) — chi's
middleware ordering handles this correctly. The middleware sees every
request that reaches the `/api/v1` route group, including the
404-from-router case (a path like `/api/v1/totally-unknown`), because
chi runs the group's middleware before the route resolver. That's the
behavior we want.

## Acceptance criteria

- `just ci` passes (backend + frontend).
- `just pg-reset && just migrate-up && just seed && just api-run` then:
  - `curl -sI :8080/api/v1/metros | grep -i "X-Data-License: ODbL-1.0"`
    matches.
  - `curl -sI :8080/api/v1/metros | grep -i "X-Data-Attribution: https://urbanistatlas.com"`
    matches.
  - `curl -sI :8080/healthz | grep -i "X-Data-License"` returns empty
    (regression).
  - `curl -s :8080/api/v1/metros | jq -e '.meta.license == "ODbL-1.0"'`
    is true.
  - `curl -s :8080/api/v1/metros | jq -e '.meta.attribution_url == "https://urbanistatlas.com"'`
    is true.
  - `curl -s :8080/api/v1/metros | jq -e '.meta.generated_at | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T")'`
    is true (RFC3339).
  - `curl -s :8080/api/v1/metros | jq -e '.data | type == "array"'`
    is true.
  - `curl -s :8080/api/v1/recent | jq -e '.meta and .data'` is true.
  - `curl -s :8080/api/v1/lookup?postal_code=11217&country=US | jq -e 'has("meta")'`
    is **false** (lookup is not enveloped — explicit non-goal).
- Web build (`just web-check`) passes; `npm test -- --run` is green.
- Manual smoke: open the dev server SPA, visit `/`, `/browse`,
  `/m/nyc-metro`, confirm the home and browse pages render with
  metros and recent orgs (proves the unwrap works end-to-end).

## Non-goals (explicit)

- **No envelope on `/lookup`.** `LookupResult` is a container with
  `local` / `regional` arrays inside, not a list. Wrapping it would
  break the wire contract for the highest-value endpoint with no
  consumer benefit; the ODbL headers ride the response either way.
- **No envelope on `/metros/{slug}`.** Returns a single resource.
- **No `Link` / `Vary` / `Cache-Control` headers.** Caching is a
  future-slice concern.
- **No version bump on `info.version`.** Matches established
  per-slice pattern.
- **No changes to `apiFetch`'s generic signature.** Unwrapping is
  the typed helper's responsibility.
- **No changes to error response shape.** RFC 9457 problem documents
  stay exactly as they are; only the two ODbL headers are added.
- **No seed-data changes.** This slice is purely about response
  shape.

## File structure

### New

| Path | Responsibility |
|---|---|
| `api/internal/httpapi/odbl.go` | `odblHeadersMiddleware`, `respondCollection[T]`, `newMeta`, the two header/license constants. |
| `api/internal/httpapi/odbl_test.go` | Middleware presence/absence, helper wrapping behavior, `newMeta` shape. |

### Modified

| Path | Change |
|---|---|
| `api/openapi.yaml` | `Meta`, `MetroSummariesEnvelope`, `RecentEnvelope` schemas; `XDataLicense` and `XDataAttribution` header refs; 200 response declarations on `/api/v1/metros` and `/api/v1/recent` switched to the envelope schemas; both 200 responses (and lookup's, getMetro's) gain the two header refs; `info.description` updated to mention the new headers and envelope. |
| `api/internal/httpapi/oapi/types.gen.go` | Regenerated. Adds `Meta`, `MetroSummariesEnvelope`, `RecentEnvelope`. |
| `api/internal/httpapi/openapi.yaml` | Refreshed copy (kept in sync with canonical). |
| `web/src/lib/api.gen.ts` | Regenerated. Adds the three new schemas; updates the 200 content type for `/api/v1/metros` and `/api/v1/recent`. |
| `api/internal/httpapi/router.go` | One line: `r.Use(odblHeadersMiddleware)` inside the `/api/v1` route group. |
| `api/internal/httpapi/metros.go` | `listMetrosHandler` switches `writeJSON(...)` → `respondCollection(w, toOAPIMetroSummaries(metros))`. `getMetroHandler` is unchanged (single resource). |
| `api/internal/httpapi/recent.go` | `recentHandler` switches `writeJSON(...)` → `respondCollection(w, toOAPIOrgs(orgs))`. |
| `api/internal/httpapi/metros_test.go` | Happy-path tests decode the envelope, assert meta fields, assert headers. |
| `api/internal/httpapi/recent_test.go` | Same as metros. |
| `web/src/lib/api.ts` | Import envelope types; `listMetros` / `listRecent` unwrap `.data`. Re-export `Meta`, `MetroSummariesEnvelope`, `RecentEnvelope` for advanced consumers. |
| `web/src/lib/api.test.ts` | Existing tests update fetch mocks to return enveloped JSON; one new test per helper asserts unwrapping. |

## Risks & mitigations

- **`apiFetch` consumers that happen to call `/api/v1/metros` or
  `/api/v1/recent` directly.** Risk: anyone bypassing `listMetros` /
  `listRecent` and using `apiFetch<MetroSummary[]>` will silently get
  an `Envelope`-shaped object cast to `MetroSummary[]`, and then
  `.length` will be undefined, etc. Mitigation: grep confirms only
  the typed helpers are used; the wire contract change is documented
  in `api.ts`'s file-level JSDoc; the envelope types are re-exported
  so any future direct caller has a typed path.
- **Middleware ordering wart.** If chi's middleware ordering changes,
  ODbL headers could fail to apply. Mitigation: tests cover the
  presence on `/api/v1/metros` and absence on `/healthz`; the
  middleware is mounted inside the `/api/v1` route group (not at
  router root) to make the path scope explicit in code, not just
  documented.
- **OpenAPI consumers downstream.** External consumers reading the
  spec saw `200: { content: { application/json: { schema: MetroSummary[] } } }`
  and now see `200: { content: { application/json: { schema: MetroSummariesEnvelope } } }`.
  Mitigation: this is the entire point of the slice — the wire
  contract change is announced via the spec; the spec is the
  contract. The PR commit ordering puts the spec change first so
  consumers see one commit.
- **Generic `respondCollection[T]` requires Go 1.18+.** Project is
  on Go 1.26 (per `mise.toml`); generics are fine.
- **Header casing.** Go's `http.Header.Set` canonicalizes — passing
  `"X-Data-License"` sets it as `X-Data-License`, which is what
  curl sees. Tests use `resp.Header.Get(...)`, which is
  case-insensitive. No risk.

## Implementation order (PR commits)

Per the worktree instructions and CLAUDE.md's "spec edits in their
own PR" guidance, the commits inside the single PR go:

1. `feat(api): openapi — Meta schema + collection envelope (slice #24)`
   — openapi.yaml edits + regenerated Go + TS types + synced
   internal copy. Should pass `just ci` with NO behavior change
   (handlers still write the bare arrays, which now disagree with
   the spec's declared shape — but `just ci` doesn't run the live
   server; it runs unit tests against committed types, which all
   pass because the types compile and the existing handler tests
   still decode `[]MetroSummary` etc.). Wait — that's not quite
   right. Re-read §"Tests": the existing happy-path tests assert
   `[]oapi.MetroSummary` and they'll keep passing because we
   haven't yet flipped the handler to use the envelope. The
   first commit's handler still emits the old shape; the second
   commit flips both the handler emit and the test decode in
   lockstep. So `just ci` is green after commit 1 (types-only
   change), and green after commit 2 (handlers + tests flip
   together).
2. `feat(api): ODbL headers + collection envelope middleware/helper (slice #24)`
   — new `odbl.go` + `odbl_test.go` + handler updates to call
   `respondCollection` + updated `metros_test.go` / `recent_test.go`
   to decode envelopes + router wiring.
3. `feat(web): unwrap data envelope in api.ts helpers (slice #24)`
   — `web/src/lib/api.ts` changes + `web/src/lib/api.test.ts`
   updates.

Each commit ends green on `just ci`. Each carries the
`Co-Authored-By` trailer.
