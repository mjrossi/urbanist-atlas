package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/loadregions"
)

// loadregionsCommand ingests a regions TOML file into the regions +
// region_parents tables. See internal/loadregions for the format and write
// behavior; the cli action stays thin (parse flags, open pool, defer).
func loadregionsCommand() *cli.Command {
	return &cli.Command{
		Name:  "loadregions",
		Usage: "Load a regions_<cc>.toml file into the regions + region_parents tables",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "src",
				Usage:    "path to the regions TOML file",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "country",
				Usage:    "country code stamped on every region row (US, CA, DE, …)",
				Required: true,
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
		Action: runLoadregions,
	}
}

func runLoadregions(ctx context.Context, c *cli.Command) error {
	dbURL := c.String("db-url")
	if dbURL == "" {
		return errors.New("loadregions: --db-url or DATABASE_URL is required")
	}
	src := c.String("src")
	country := c.String("country")
	logger := buildLogger(c.String("log-format"))

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("loadregions: open pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("loadregions: ping: %w", err)
	}

	summary, err := loadregions.LoadFile(ctx, pool, logger, src, country)
	if err != nil {
		return err
	}
	logger.Info("loadregions complete",
		"src", src,
		"country", country,
		"regions", summary.Regions,
		"parent_edges", summary.ParentEdges,
	)
	return nil
}
