# Testing strategy

Three tiers on the API, one on the web. Read this when adding a
feature so you write the right kind of test (and don't write the
wrong kind by default).

## API: three tiers

```
unit tests           handler tests              store tests
──────────           ─────────────              ───────────
pkg/atlas/*_test.go  internal/httpapi/*_test.go internal/store/sqlite/*_test.go
fast, no Docker      fast, no Docker            fast, no Docker
just api-test        just api-test              just api-test
```

All three tiers run under the single `just api-test` (`go test ./...`).
There's no Docker-gated tier: the read path is in-process (MemStore) and
the only persistence is the submissions SQLite store, which tests exercise
in-process via `modernc.org/sqlite` against an in-memory / temp-file DB.

### Unit tests — `pkg/atlas/*_test.go`

Test pure functions and algorithms in isolation. Use `MemStore`
fixtures when a test needs a Store; the in-process fake is loaded
via `atlas.LoadDevFixtures(store)` or built by hand for narrower
cases.

When to add a unit test:

- Anything in `pkg/atlas` that has branching logic — sort orders,
  bucketing, label derivation, postal-code normalization, region
  ancestry walks.
- Anything that's part of the `Store` interface contract that
  shouldn't depend on a real DB to verify (e.g. how `MemStore`
  handles a missing region; how `Lookup` buckets local vs.
  regional).

What unit tests should *not* do:

- Touch chi, net/http, or `httptest`. If you find yourself
  spinning up an HTTP server, it's a handler test.
- Touch `database/sql` or the SQLite submissions store. If you find
  yourself opening a DB connection, it's a store test.

### Handler tests — `internal/httpapi/*_test.go`

Test HTTP shape against a fixture-backed store. Use the
`httptest` package + `MemStore`:

```go
func newTestServer(t *testing.T) *httptest.Server {
    store := atlas.NewMemStore()
    atlas.LoadDevFixtures(store)
    handler := New(Config{
        Store:      store,
        Logger:     slog.New(slog.DiscardHandler),
        APIVersion: "v1",
    })
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    return srv
}
```

`clientsecret_test.go` is the canonical example — copy its shape
for new middleware or handler tests.

When to add a handler test:

- A new endpoint: assert status code, content type, error envelope
  (`application/problem+json` + the right problem-type URI), and
  the JSON wire shape for success.
- New middleware: write a probe handler, wrap it in the
  middleware, assert how the response changes.
- An existing handler whose error mapping you've touched —
  especially the mapping between domain errors (`ErrPostalCodeNotFound`)
  and HTTP status codes.

What handler tests should *not* do:

- Verify business algorithms. That belongs in unit tests against
  `pkg/atlas`. Handler tests confirm the wire-up, not the math.
- Open the submissions DB. The `MemStore` is the test double; the
  SQLite store has its own tests.

### Store tests — `internal/store/sqlite/*_test.go`

Test the SQLite submissions store: the goose migrations, the
sqlc-generated bindings, and the store methods that the
`/submissions` endpoints call. These run in-process against
`modernc.org/sqlite` (pure Go, no CGO) using an in-memory or
temp-file DB, so they're fast and need **no Docker** — they run
under the default `go test ./...`:

```go
package sqlite_test

import "testing"
```

Run with:

```sh
just api-test    # store tests run alongside unit + handler tests
```

When to add a store test:

- New submissions query (added to
  `internal/store/sqlite/queries/submissions.sql` and regenerated
  via `just api-gen`).
- New migration in `api/migrations-sqlite/` — verify it applies
  cleanly via goose on top of the previous schema and that the
  data shape works.
- Status transitions and constraints (pending → approved/rejected,
  the `status` CHECK, unique `public_id`, promotion-result writes).
- Anything where the generated bindings × real SQLite behavior
  matters (NULL handling, the WAL/`busy_timeout` pragmas, ordering).

The read path has no store-test tier: it's the in-process MemStore,
fully covered by the unit and handler tiers above. There is no
external database to stand up for any tier.

## Choosing a tier

| Symptom | Tier | Why |
|---|---|---|
| Bucketing sorts orgs wrong | unit | Pure logic in `pkg/atlas`. |
| `/lookup` returns 500 instead of 404 | handler | Domain-error → HTTP mapping is in the handler. |
| `AncestorRegions` returns wrong ancestors | unit | In-process DAG walk in `pkg/atlas`. |
| Submissions migration drops a needed column | store | Real goose migration against SQLite. |
| `X-Atlas-Client` gate lets a bad header through | handler | Middleware behavior is HTTP-shape. |
| Approving a submission skips the status transition | store | Real query × SQLite store. |
| `NormalizePostalCode` mangles PT 7-digit codes | unit | Pure function. |
| ODbL headers missing on `/recent` | handler | Middleware composition. |
| sqlc-generated submissions binding panics on NULL | store | Generated bindings × real DB. |

## Web

Vitest + React Testing Library. One test file per component or
route:

```
web/src/components/SearchBox.tsx        ↔ web/src/components/SearchBox.test.tsx
web/src/routes/Home.tsx                 ↔ web/src/routes/Home.test.tsx
web/src/lib/api.ts                      ↔ web/src/lib/api.test.ts
```

Run with:

```sh
just web-test       # vitest --run, no watch
```

Conventions:

- Render the component under test with the same providers it
  needs in production (`MemoryRouter` for routes; a fresh
  `QueryClient` for anything calling TanStack Query). The
  existing tests show the shape — copy from `Home.test.tsx` or
  `Results.test.tsx`.
- Stub `fetch` (or `apiFetch`) at the network boundary, not the
  query-key boundary. Tests should exercise the same loading /
  error / success transitions the user sees.
- Don't over-snapshot. Assert on the visible text and the
  presence of the right elements; HTML structure tests rot
  without adding signal.
- `lib/api.test.ts` is the wire client's regression suite — when
  you add a new endpoint helper, add it here. Stubs the global
  `fetch` and asserts on URL, headers (including
  `X-Atlas-Client`), and parsed body shape.

What to skip:

- Tests for components that are pure JSX with no branching (a
  Footer, a static About header). The cost of maintenance
  outweighs the rare bug they'd catch.
- Tests against `api.gen.ts` (the generated TS wire types) —
  they're regenerated from the spec; testing them is testing
  `openapi-typescript`.

## CI gates

`just ci` is the union of the two halves and is what CI runs:

```sh
just api-check   # api-fmt-check + api-vet + api-staticcheck + api-test + api-gen-check
just web-check   # web-deps + web-lint + web-test + web-build + web-gen-check
```

`api-test` is the whole API suite — all three tiers run together with
no Docker-gated step to opt into. Both halves run their type-gen
no-diff check (`api-gen-check`, `web-gen-check`) so spec edits without
regenerated artifacts fail at PR time, not deploy time.

Local quick-check before pushing: `just api-test && just web-test`
catches the common regressions in under a minute. Use `just ci`
when touching the spec or generated code, since `api-gen-check`
and `web-gen-check` add a few seconds but catch the drifts CI
would otherwise reject.
