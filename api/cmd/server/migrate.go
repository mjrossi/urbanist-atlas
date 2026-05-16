package main

import (
	"context"
	"errors"

	"github.com/urfave/cli/v3"
)

// migrateCommand is a placeholder until the Postgres store + goose
// wiring lands. It declares the subcommand shape (so it shows up in
// --help and so docs/CI can reference the surface) but every action
// currently returns an "not yet implemented" error.
func migrateCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Apply or roll back database migrations",
		Commands: []*cli.Command{
			{
				Name:   "up",
				Usage:  "Apply all pending migrations",
				Action: notImplemented("migrate up"),
			},
			{
				Name:   "down",
				Usage:  "Roll back the most recent migration",
				Action: notImplemented("migrate down"),
			},
			{
				Name:   "status",
				Usage:  "Show migration status",
				Action: notImplemented("migrate status"),
			},
		},
	}
}

func notImplemented(name string) cli.ActionFunc {
	return func(context.Context, *cli.Command) error {
		return errors.New(name + ": not yet implemented (Postgres store lands in a later commit)")
	}
}
