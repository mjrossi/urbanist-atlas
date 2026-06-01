package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

// seedCommand groups operator-side checks over the embedded seed
// bundle. Its sole sub-subcommand today is `validate`, the HOST-01b
// pre-deploy gate:
//
//	urbanist-atlas-server seed validate
//
// validate runs the same BuildMemStore loader the server uses at boot
// (against the //go:embed bundle baked into the binary), so a malformed
// seed — a dangling org region_slug, a cross-file DAG cycle, an orphan
// leaf — fails CI with a non-zero exit BEFORE the Fly deploy proceeds,
// rather than surfacing at the post-deploy /healthz smoke test. It is
// distinct from `just seed-check` (which only regenerates the
// ETL-derived region files and diffs them): validate covers the
// hand-curated orgs.toml + city leaves that seed-check never loads.
//
// It is offline and mutates nothing (no network, no tree writes), so it
// is safe to run inside `just ci` and as a dedicated CI job.
func seedCommand() *cli.Command {
	logFlags := []cli.Flag{
		&cli.StringFlag{
			Name:    "log-format",
			Usage:   "log output format: json or text",
			Value:   "text",
			Sources: cli.EnvVars("URBANIST_LOG_FORMAT"),
		},
		&cli.StringFlag{
			Name:    "log-level",
			Usage:   "minimum log level: debug, info, warn, or error",
			Value:   "info",
			Sources: cli.EnvVars("URBANIST_LOG_LEVEL"),
		},
	}

	return &cli.Command{
		Name:  "seed",
		Usage: "Operator-side checks over the embedded seed bundle",
		Commands: []*cli.Command{
			{
				Name:   "validate",
				Usage:  "Load the embedded seed via BuildMemStore and exit non-zero on any error (pre-deploy gate)",
				Flags:  logFlags,
				Action: runSeedValidate,
			},
		},
	}
}

func runSeedValidate(_ context.Context, c *cli.Command) error {
	logger := buildLogger(c.String("log-format"), c.String("log-level"))

	// Validate the EMBEDDED bundle (same source the server boots from
	// when URBANIST_SEED_DIR is unset), not an on-disk directory: this
	// is exactly what ships in the image.
	s, err := seedfiles.BuildMemStore(logger, seedfs.FS)
	if err != nil {
		return fmt.Errorf("seed validate: %w", err)
	}

	logger.Info("seed validate: embedded seed loaded cleanly",
		"regions", len(s.Slugs()),
	)
	return nil
}
