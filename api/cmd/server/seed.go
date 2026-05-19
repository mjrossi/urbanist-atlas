package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
)

// seedCommand loads api/seed/orgs.toml into organizations +
// organization_regions. Region linkage resolves each org's
// region_slugs through the already-populated regions table, so
// loadregions must run first.
func seedCommand() *cli.Command {
	return &cli.Command{
		Name:  "seed",
		Usage: "Load curated org seed data (api/seed/orgs.toml) into the database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "src",
				Usage: "path to seed TOML",
				Value: "./seed/orgs.toml",
			},
			&cli.StringFlag{
				Name:    "db-url",
				Usage:   "Postgres connection string",
				Sources: cli.EnvVars("DATABASE_URL"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "log output format: json or text",
				Value:   "text",
				Sources: cli.EnvVars("URBANIST_LOG_FORMAT"),
			},
		},
		Action: runSeed,
	}
}

func runSeed(ctx context.Context, c *cli.Command) error {
	dbURL := c.String("db-url")
	if dbURL == "" {
		return errors.New("seed: --db-url or DATABASE_URL is required")
	}
	src := c.String("src")
	logger := buildLogger(c.String("log-format"))

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("seed: open pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("seed: ping: %w", err)
	}

	summary, err := seed.LoadFile(ctx, pool, logger, src)
	if err != nil {
		return err
	}
	logger.Info("seed complete",
		"src", src,
		"orgs", summary.OrgsUpserted,
		"region_links", summary.RegionLinks,
	)
	return nil
}
