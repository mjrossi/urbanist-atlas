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
				Usage: "ISO-style country code of the source data (e.g. US, CA, PT)",
				Value: "US",
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
		Action: runLoadpostal,
	}
}

func runLoadpostal(ctx context.Context, c *cli.Command) error {
	dbURL := c.String("db-url")
	if dbURL == "" {
		return errors.New("loadpostal: --db-url or DATABASE_URL is required")
	}
	country := atlas.Country(c.String("country"))
	if country == "" {
		return errors.New("loadpostal: --country is required")
	}
	src := c.String("src")
	logger := buildLogger(c.String("log-format"))

	// Country is an opaque string per pkg/atlas/atlas.go; the loader is
	// country-agnostic, but per-country postal validation lives in
	// atlas.ValidatePostalCode. Warn (don't fail) if the country isn't
	// in the recognized validator set — loading still proceeds, but
	// operators see they're in uncharted territory. To add validation
	// for a new country, extend the switch in api/pkg/atlas/postal.go.
	switch country {
	case atlas.CountryUS, atlas.CountryCA, "DE", "FR", "MX", "UK", "AU", "PT":
		// recognized; no warning.
	default:
		logger.Warn("loadpostal: country has no postal-code validator; loading without per-row format checks",
			"country", string(country),
			"hint", "add a case to api/pkg/atlas/postal.go to validate this country's codes",
		)
	}

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
		"postal_codes", summary.PostalCodes,
	)
	return nil
}
