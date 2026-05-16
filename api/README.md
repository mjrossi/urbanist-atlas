# Urbanist Atlas — API

Go service powering [urbanistatlas.com](https://urbanistatlas.com). Public JSON API at `/api/v1`, served by Fly.io.

## Layout

```
api/
├── cmd/server/         # urfave/cli entry point. Subcommands:
│                       #   serve, migrate {up|down|status},
│                       #   loadpostal, seed
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
└── seed/               # Human-reviewed seed data (orgs.yaml).
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

## Local dev (once scaffolded)

```
go run ./cmd/server migrate up
go run ./cmd/server loadpostal --src ./data/postal_us.csv
go run ./cmd/server seed
go run ./cmd/server serve
```
