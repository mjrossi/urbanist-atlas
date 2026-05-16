package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/migrations"
)

// migrateCommand wires goose against the embedded migrations FS so the
// server binary can apply, roll back, and report on its own schema
// without any external goose CLI install.
//
// The migrations live in api/migrations/*.sql and are baked into the
// binary via the embed.FS in migrations/embed.go.
//
// Usage:
//
//	urbanist-atlas-server migrate up
//	urbanist-atlas-server migrate down
//	urbanist-atlas-server migrate status
func migrateCommand() *cli.Command {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:     "db-url",
			Usage:    "Postgres connection string",
			Sources:  cli.EnvVars("URBANIST_DB_URL"),
			Required: false, // checked at action time so --help works without env
		},
	}

	return &cli.Command{
		Name:  "migrate",
		Usage: "Apply or roll back database migrations",
		Commands: []*cli.Command{
			{
				Name:   "up",
				Usage:  "Apply all pending migrations",
				Flags:  flags,
				Action: runMigrate("up"),
			},
			{
				Name:   "down",
				Usage:  "Roll back the most recent migration",
				Flags:  flags,
				Action: runMigrate("down"),
			},
			{
				Name:   "status",
				Usage:  "Show migration status",
				Flags:  flags,
				Action: runMigrate("status"),
			},
		},
	}
}

func runMigrate(action string) cli.ActionFunc {
	return func(ctx context.Context, c *cli.Command) error {
		dbURL := c.String("db-url")
		if dbURL == "" {
			return errors.New("migrate: --db-url or URBANIST_DB_URL is required")
		}

		// Goose drives migrations through database/sql, so we wrap our
		// pgx connection in the stdlib bridge. We deliberately do not
		// reuse the application's pgxpool here — migrations should run
		// against a fresh, single connection to avoid surprises from
		// pool-level session state (e.g. statement timeouts).
		connCfg, err := pgx.ParseConfig(dbURL)
		if err != nil {
			return fmt.Errorf("migrate: parse db url: %w", err)
		}
		db := stdlib.OpenDB(*connCfg)
		defer func() { _ = db.Close() }()

		if err := goose.SetDialect("postgres"); err != nil {
			return fmt.Errorf("migrate: set dialect: %w", err)
		}
		goose.SetBaseFS(migrations.FS)

		return runGooseAction(ctx, db, action)
	}
}

func runGooseAction(ctx context.Context, db *sql.DB, action string) error {
	switch action {
	case "up":
		if err := goose.UpContext(ctx, db, "."); err != nil {
			return fmt.Errorf("migrate up: %w", err)
		}
	case "down":
		if err := goose.DownContext(ctx, db, "."); err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
	case "status":
		if err := goose.StatusContext(ctx, db, "."); err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
	default:
		return fmt.Errorf("migrate: unknown action %q", action)
	}
	return nil
}
