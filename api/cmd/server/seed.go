package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

// seedCommand groups checks over the embedded seed bundle. `seed
// validate` runs the boot loader, BuildMemStore, so a malformed seed
// fails before deploy instead of at boot. Unlike `just seed-check` it
// covers the hand-curated files, and it is offline and read-only.
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

	// The embedded bundle, because that is what ships in the image.
	s, err := seedfiles.BuildMemStore(logger, seedfs.FS)
	if err != nil {
		return fmt.Errorf("seed validate: %w", err)
	}

	logger.Info("seed validate: embedded seed loaded cleanly",
		"regions", len(s.Slugs()),
	)
	return nil
}
