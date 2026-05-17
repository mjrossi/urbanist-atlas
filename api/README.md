# Urbanist Atlas — API

Go service powering [urbanistatlas.com](https://urbanistatlas.com). Public JSON API at `/api/v1`, served by Fly.io.

## Layout

```
api/
├── cmd/server/         # urfave/cli entry point. Subcommands:
│                       #   serve, migrate {up|down|status},
│                       #   loadregions, loadpostal, seed
├── pkg/atlas/          # Public, importable library. Pure Go.
│                       # Org/Region/LookupResult types, the lookup
│                       # algorithm, the Store interface, and an
│                       # in-memory Store impl for tests + CLI use.
├── internal/store/     # Non-public Store implementations.
│   └── postgres/       # Postgres store: queries written in SQL,
│                       # codegen'd to Go via sqlc.
├── internal/httpapi/   # chi handlers. Thin wrappers over pkg/atlas.
├── migrations/         # goose-style SQL migrations, embedded into
│                       # the binary.
└── seed/               # Human-reviewed seed data:
                        #   regions_<cc>.toml (taxonomy + DAG edges)
                        #   postal_codes_<cc>.csv (postal → leaf)
                        #   orgs.toml (curated org directory)
```

## Conventions

See the root `CLAUDE.md` and the approved plan at
`~/.claude/plans/we-are-planning-a-smooth-candy.md`. In short:

- Standard library first; deliberate exceptions are listed in CLAUDE.md.
- Router: `go-chi/chi` v5.
- CLI / startup: `urfave/cli` v3.
- DB: `sqlc` → `pgx/v5` driver.
- Migrations: `pressly/goose` v3, embedded.
- Logging: `log/slog` (stdlib).
- Errors: stdlib `errors` + `fmt.Errorf("...: %w", err)`.

## Wire contract

The single source of truth for request/response shapes is
[`api/openapi.yaml`](./openapi.yaml). The server embeds this file and
serves it at `GET /api/v1/openapi.yaml` so external consumers can
discover it at runtime. Go types in `internal/httpapi/oapi/` are
oapi-codegen output; TS types in `web/src/lib/api.gen.ts` are
openapi-typescript output. Neither half hand-rolls request/response
structs.

### Embedded copy: keep it in sync

`go:embed` cannot escape the source file's package directory, so a
real copy of `openapi.yaml` lives at
`internal/httpapi/openapi.yaml` next to the handler that embeds it.
Whenever you edit the canonical spec, run:

```
just api-oapi-gen   # `go generate ./...` — refreshes both the embedded copy AND the oapi-codegen types
```

`TestEmbeddedOpenAPISpecMatchesCanonical` (in
`internal/httpapi/openapi_handler_test.go`) fails fast if the two
files drift, and `just api-check` re-runs `go generate` and asks git
whether anything changed, so a forgotten `go generate` won't slip
through review.

## Local dev

Migrations and the Postgres store are owned by the server binary —
no external goose CLI required.

```
# 1. provision Postgres locally; URBANIST_DB_URL is read by every
#    Postgres-touching subcommand.
createdb urbanist_atlas_dev
export URBANIST_DB_URL=postgres://localhost:5432/urbanist_atlas_dev?sslmode=disable

# 2. apply schema migrations (embedded into the binary).
just migrate-up
just migrate-status

# 3. load the bundled fixtures — regions first (so leaf slugs resolve),
#    then postal-code crosswalks, then orgs. `just loaddata` does all
#    three countries in the right order:
just loaddata

# … or step-by-step (idempotent — each recipe is re-runnable):
just loadregions seed/regions_us.toml US
just loadpostal  seed/postal_codes_us.csv US
just loadregions seed/regions_ca.toml CA
just loadpostal  seed/postal_codes_ca.csv CA
just loadregions seed/regions_pt.toml PT
just loadpostal  seed/postal_codes_pt.csv PT
just seed

# 4. serve. Defaults to --store=postgres so dev configurations
#    fail loudly on a missing DB rather than silently feeding back
#    fixture data.
just api-run
```

The bundled TOMLs and CSVs live under [`api/seed/`](./seed); see
[`api/seed/README.md`](./seed/README.md) for the file format and the
documented upstream sources (US Census ZCTA, StatsCan FSA, OpenPLZ
for PT) if you want to scale beyond the fixture-sized dataset.

Pass `--store=memory` (or set `URBANIST_STORE=memory`) to use the
fixture-backed in-memory store. Useful for the frontend devloop and
for offline `pkg/atlas` exploration.

## Integration tests

The Postgres store has its own integration test under
`internal/store/postgres/store_test.go`, gated by the `integration`
build tag so the default `go test ./...` stays fast and Docker-free.
The test spins up an ephemeral Postgres 17 container with
[testcontainers-go](https://github.com/testcontainers/testcontainers-go)
and runs the embedded migrations against it before exercising the
adapter through the same `atlas.Store` interface the production
server uses.

```
just api-test-integration
```

This requires a running Docker daemon. CI does not run it.
