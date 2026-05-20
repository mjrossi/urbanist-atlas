package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"
)

// etlCommand wraps the operator-side data pipeline that reshapes
// upstream postal-code and metro reference data (Census ZCTA + CBSA,
// Statistics Canada PCCF + CMA) into the seed file shapes loadregions
// and loadpostal already consume.
//
// Two sub-subcommands:
//
//	urbanist-atlas-server etl download   --country US
//	urbanist-atlas-server etl regenerate --country US
//
// The foundation slice (#7.5.1) ships these as no-op stubs that just
// log what they would do. Concrete country plans land in:
//
//   - #7.5.3 — US: Census CBSA + ZCTA-to-place + ZCTA-to-county →
//     regions_us_msas.toml + postal_codes_us.csv.
//   - #7.5.4 — CA: StatsCan PCCF + CMA reference →
//     regions_ca_cmas.toml + postal_codes_ca.csv.
//
// Design rationale at docs/superpowers/specs/2026-05-19-postal-coverage-design.md.
func etlCommand() *cli.Command {
	commonFlags := []cli.Flag{
		&cli.StringFlag{
			Name:     "country",
			Usage:    "country code to operate on (US, CA, ...)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "src",
			Usage: "directory holding upstream source files (per-country subdir layout)",
			Value: "./etl/sources",
		},
		&cli.StringFlag{
			Name:  "out",
			Usage: "directory to write generated seed files into",
			Value: "./seed",
		},
		&cli.StringFlag{
			Name:    "log-format",
			Usage:   "log output format: json or text",
			Value:   "text",
			Sources: cli.EnvVars("URBANIST_LOG_FORMAT"),
		},
	}

	return &cli.Command{
		Name:  "etl",
		Usage: "Reshape upstream postal/metro source data into the seed file shapes loadregions/loadpostal consume",
		Commands: []*cli.Command{
			{
				Name:   "download",
				Usage:  "Fetch upstream source files into etl/sources/<country>/ and validate checksums",
				Flags:  commonFlags,
				Action: runEtlDownload,
			},
			{
				Name:   "regenerate",
				Usage:  "Parse staged source files and write deterministic seed TOML/CSV under api/seed/",
				Flags:  commonFlags,
				Action: runEtlRegenerate,
			},
		},
	}
}

func runEtlDownload(ctx context.Context, c *cli.Command) error {
	country := c.String("country")
	if country == "" {
		return errors.New("etl download: --country is required")
	}
	logger := buildLogger(c.String("log-format"))

	plan, ok := etl.Plans[country]
	if !ok {
		// Foundation slice ships no plans; this is the expected path
		// until #7.5.3 / #7.5.4 register US and CA.
		logger.Info("etl download: no plan registered for country (no-op stub)",
			"country", country,
			"hint", "concrete plans land in slices #7.5.3 (US) and #7.5.4 (CA)",
		)
		return nil
	}

	logger.Info("etl download: plan found",
		"country", country,
		"sources", len(plan.Sources),
		"src_dir", c.String("src"),
	)
	return fmt.Errorf("etl download: country %q plan registered but download flow not yet implemented (lands in #7.5.3/#7.5.4)", country)
}

func runEtlRegenerate(ctx context.Context, c *cli.Command) error {
	country := c.String("country")
	if country == "" {
		return errors.New("etl regenerate: --country is required")
	}
	logger := buildLogger(c.String("log-format"))

	plan, ok := etl.Plans[country]
	if !ok {
		// Foundation slice ships no plans; this is the expected path
		// until #7.5.3 / #7.5.4 register US and CA.
		logger.Info("etl regenerate: no plan registered for country (no-op stub)",
			"country", country,
			"hint", "concrete plans land in slices #7.5.3 (US) and #7.5.4 (CA)",
		)
		return nil
	}

	logger.Info("etl regenerate: plan found",
		"country", country,
		"sources", len(plan.Sources),
		"targets", len(plan.Targets),
		"src_dir", c.String("src"),
		"out_dir", c.String("out"),
	)
	return fmt.Errorf("etl regenerate: country %q plan registered but regenerate flow not yet implemented (lands in #7.5.3/#7.5.4)", country)
}
