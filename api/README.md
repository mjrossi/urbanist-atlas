# Urbanist Atlas — API

Go service powering [urbanistatlas.com](https://urbanistatlas.com). Public JSON API at `/api/v1`, served from Fly.io.

## Layout

```
api/
├── cmd/server/         # urfave/cli entry point. Subcommands:
│                       #   serve, linkcheck, etl
├── pkg/atlas/          # Public, importable library. Pure Go.
│                       # Org/Region/LookupResult types, the lookup
│                       # algorithm, the Store interface, and the
│                       # in-memory MemStore that backs the runtime.
├── internal/seedfiles/ # Parses the bundled TOML/CSV files into
│                       # atlas.Region / atlas.Org (with toml tags)
│                       # and builds the in-memory MemStore at boot.
│                       # Works against either an fs.FS embed or a
│                       # disk path via os.DirFS.
├── internal/httpapi/   # chi handlers. Thin wrappers over pkg/atlas.
└── seed/               # Human-reviewed seed data (the runtime source of truth):
                        #   regions_<cc>*.toml (taxonomy + DAG edges)
                        #   postal_codes_<cc>.csv (postal → leaf)
                        #   orgs.toml (curated org directory)
```

## Conventions

See the root `CLAUDE.md`. In short:

- Standard library first; deliberate exceptions are listed in CLAUDE.md.
- Router: `go-chi/chi` v5.
- CLI / startup: `urfave/cli` v3.
- Logging: `log/slog` (stdlib).
- Errors: stdlib `errors` + `fmt.Errorf("...: %w", err)`.
- No database: the API is stateless and reads `api/seed/` into memory at boot.

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
just api-gen   # `go generate ./...` — refreshes both the embedded copy AND the oapi-codegen types
```

`TestEmbeddedOpenAPISpecMatchesCanonical` (in
`internal/httpapi/openapi_handler_test.go`) fails fast if the two
files drift, and `just api-check` re-runs `go generate` and asks git
whether anything changed, so a forgotten `go generate` won't slip
through review.

## Local dev

```
# 1. (one time) install Go, Node, oapi-codegen via mise.
mise install

# 2. run the server. Defaults to --store=file --seed-dir=./seed,
#    so this is the entire setup.
just api-run
```

The bundled TOMLs and CSVs live under [`api/seed/`](./seed); see
[`api/seed/README.md`](./seed/README.md) for the file format and the
documented upstream sources (US Census ZCTA, StatsCan FSA, OpenPLZ
for PT).

Pass `--store=memory` (or set `URBANIST_STORE=memory`) to use the
tiny dev-fixture set hard-coded in `pkg/atlas/devfixtures.go` instead
of the full bundle. Useful for offline `pkg/atlas` exploration and
demos where you don't want to ship `api/seed/` alongside the binary.

## Tests

```
just api-test            # unit + the BuildMemStore test that loads the real api/seed/ bundle
just api-check           # vet + tests + oapi gen-no-diff (what CI runs)
```

There is no separate Postgres integration suite — the FileStore is
exercised against the real bundled seed inside `just api-test`, and
the `pkg/atlas/storetest` contract harness validates every Store
behavioral guarantee against MemStore.
