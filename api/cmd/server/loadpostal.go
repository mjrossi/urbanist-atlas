package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/loadpostal"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// loadpostalCommand ingests a postal-code crosswalk CSV into regions +
// postal_codes. See internal/loadpostal for the format and write
// behavior; the cli action stays thin (parse flags, open pool, defer).
func loadpostalCommand() *cli.Command {
	return &cli.Command{
		Name:  "loadpostal",
		Usage: "Load postal-code → region data into the database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "src",
				Usage:    "path to CSV source file",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "country",
				Usage: "country of the source data: US or CA",
				Value: "US",
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
		Action: runLoadpostal,
	}
}

func runLoadpostal(ctx context.Context, c *cli.Command) error {
	dbURL := c.String("db-url")
	if dbURL == "" {
		return errors.New("loadpostal: --db-url or URBANIST_DB_URL is required")
	}
	country := atlas.Country(c.String("country"))
	if country != atlas.CountryUS && country != atlas.CountryCA {
		return fmt.Errorf("loadpostal: --country must be US or CA (got %q)", country)
	}
	src := c.String("src")
	logger := buildLogger(c.String("log-format"))

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("loadpostal: open pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("loadpostal: ping: %w", err)
	}

	summary, err := loadpostal.LoadFile(ctx, pool, logger, src, country)
	if err != nil {
		return err
	}
	logger.Info("loadpostal complete",
		"src", src,
		"country", string(country),
		"rows", summary.RowsParsed,
		"postal_codes", summary.PostalCodes,
		"regions_touched", summary.RegionsTouched,
	)
	return nil
}
