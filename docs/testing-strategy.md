# Testing strategy

Three tiers on the API, one on the web. Read this when adding a
feature so you write the right kind of test (and don't write the
wrong kind by default).

## API: three tiers

```
unit tests           handler tests              integration tests
──────────           ─────────────              ─────────────────
pkg/atlas/*_test.go  internal/httpapi/*_test.go internal/store/postgres/*_test.go
                                                (//go:build integration)
fast, no Docker      fast, no Docker            slow, Docker required
just api-test        just api-test              just api-test-integration
```

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
- Touch `database/sql`, `pgx`, or testcontainers. If you find
  yourself opening a connection, it's an integration test.

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
- Connect to Postgres. The `MemStore` is the test double.

### Integration tests — `internal/store/postgres/*_test.go`

Test SQL queries, migrations, and the sqlc-generated bindings
against a real `postgres:17-alpine` container — the same image
production runs. Gated by `//go:build integration` so the default
`go test ./...` stays fast and Docker-free:

```go
//go:build integration

package postgres_test

import "testing"
```

Run with:

```sh
just api-test-integration
```

Requires a running Docker daemon. CI does not run these tests
today (they need Docker-in-Docker config); the maintainer runs
them locally before opening PRs that touch SQL.

When to add an integration test:

- New SQL query (added to `internal/store/postgres/queries/` and
  regenerated via `just api-sqlc-gen`).
- New migration in `api/migrations/` — verify it applies cleanly
  on top of the previous schema and that the data shape works.
- Recursive CTEs or any SQL where the postgres-specific behavior
  matters (window functions, JSONB, full-text). Postgres-fork
  bugs are real; testcontainers is how we catch them.
- Performance-sensitive paths where `MemStore`'s in-process loop
  doesn't represent reality (large IN lists, joins).

The integration suite double-checks the `Store` contract — every
method that has a `MemStore` test should have a Postgres test
that exercises the same shape against the real DB.

## Choosing a tier

| Symptom | Tier | Why |
|---|---|---|
| Bucketing sorts orgs wrong | unit | Pure logic in `pkg/atlas`. |
| `/lookup` returns 500 instead of 404 | handler | Domain-error → HTTP mapping is in the handler. |
| Migration drops a needed column | integration | Real schema, real Postgres. |
| `X-Atlas-Client` gate lets a bad header through | handler | Middleware behavior is HTTP-shape. |
| Recursive CTE returns wrong ancestors | integration | Postgres-specific SQL. |
| `NormalizePostalCode` mangles PT 7-digit codes | unit | Pure function. |
| ODbL headers missing on `/recent` | handler | Middleware composition. |
| sqlc-generated code panics on NULL | integration | Generated bindings × real DB. |

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
just api-check   # api-vet + api-test + api-gen-check
just web-check   # web-deps + web-lint + web-test + web-build + web-gen-check
```

Neither half runs the integration suite. Both halves run their
type-gen no-diff check (`api-gen-check`, `web-gen-check`) so spec
edits without regenerated artifacts fail at PR time, not deploy
time.

Local quick-check before pushing: `just api-test && just web-test`
catches the common regressions in under a minute. Use `just ci`
when touching the spec or generated code, since `api-gen-check`
and `web-gen-check` add a few seconds but catch the drifts CI
would otherwise reject.
