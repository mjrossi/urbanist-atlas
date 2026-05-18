package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/loaddata"
)

// loaddataCommand runs the full bundled-seed import in one shot:
// regions → postal codes → orgs, for every country in the bundle.
// Mirrors the `just loaddata` recipe in the root justfile, but as a
// single Go subcommand so a Fly deploy can run it via
//
//	flyctl ssh console -C "urbanist-atlas-server loaddata"
//
// without depending on `just` or shell scripting inside the image.
// The real logic lives in internal/loaddata; this is glue.
func loaddataCommand() *cli.Command {
	return &cli.Command{
		Name:  "loaddata",
		Usage: "Load every bundled seed file (regions → postal codes → orgs) in dependency order",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "seed-dir",
				Usage:   "directory containing regions_<cc>.toml, postal_codes_<cc>.csv, orgs.toml",
				Value:   "./seed",
				Sources: cli.EnvVars("URBANIST_SEED_DIR"),
			},
			&cli.StringFlag{
				Name:    "db-url",
				Usage:   "Postgres connection string",
				Sources: cli.EnvVars("URBANIST_DB_URL"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "log output format: json or text",
				Value:   "text",
				Sources: cli.EnvVars("URBANIST_LOG_FORMAT"),
			},
		},
		Action: runLoaddata,
	}
}

func runLoaddata(ctx context.Context, c *cli.Command) error {
	dbURL := c.String("db-url")
	if dbURL == "" {
		return errors.New("loaddata: --db-url or URBANIST_DB_URL is required")
	}
	seedDir := c.String("seed-dir")
	logger := buildLogger(c.String("log-format"))

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("loaddata: open pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("loaddata: ping: %w", err)
	}

	if err := loaddata.LoadAll(ctx, pool, logger, seedDir); err != nil {
		return err
	}
	logger.Info("loaddata complete", "seed_dir", seedDir)
	return nil
}
