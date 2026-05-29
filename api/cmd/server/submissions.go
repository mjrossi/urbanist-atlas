package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/githubpr"
	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// submissionsCommand groups operator-facing actions on the submission
// queue. `serve` already exposes the same functionality via the admin
// HTTP endpoints; the CLI mirrors are for cases where shell access is
// faster — most importantly, retrying a PR whose worker run failed
// during a GitHub outage.
func submissionsCommand() *cli.Command {
	return &cli.Command{
		Name:  "submissions",
		Usage: "Inspect and operate on the submission queue",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "db-path",
				Usage:   "SQLite database path",
				Value:   "/data/atlas.db",
				Sources: cli.EnvVars("URBANIST_DB_PATH"),
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "retry-pr",
				Usage: "Re-run the GitHub PR worker for an approved submission whose first attempt failed",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "id", Required: true, Usage: "submission public id (UUIDv7)"},
					&cli.StringFlag{
						Name:    "github-token",
						Usage:   "GitHub PAT (required)",
						Sources: cli.EnvVars("URBANIST_GITHUB_TOKEN"),
					},
				},
				Action: runRetryPR,
			},
		},
	}
}

func runRetryPR(ctx context.Context, c *cli.Command) error {
	logger := buildLogger("text", "debug")

	token := c.String("github-token")
	if token == "" {
		return errors.New("submissions retry-pr: --github-token (or URBANIST_GITHUB_TOKEN) required")
	}

	// db-path may live on either the top-level submissions command
	// or be left at its default. urfave looks up the nearest
	// definition.
	dbPath := c.String("db-path")
	if dbPath == "" {
		dbPath = "/data/atlas.db"
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		return fmt.Errorf("submissions retry-pr: open db: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("submissions retry-pr: migrate: %w", err)
	}

	id := c.String("id")
	sub, err := store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("submissions retry-pr: load %s: %w", id, err)
	}
	if sub.Status != atlas.SubmissionApproved {
		return fmt.Errorf("submissions retry-pr: submission %s is %q, not approved", id, sub.Status)
	}

	worker := githubpr.New(githubpr.Config{
		Token:  token,
		Logger: logger,
	})

	prURL, runErr := worker.ProcessNow(ctx, sub)
	persistErr := ""
	if runErr != nil {
		persistErr = runErr.Error()
	}
	if err := store.AttachPromotionResult(ctx, sub.PublicID, prURL, persistErr); err != nil {
		logger.WarnContext(ctx, "submissions retry-pr: failed to persist outcome",
			"id", sub.PublicID, "err", err)
	}
	if runErr != nil {
		return fmt.Errorf("submissions retry-pr: %w", runErr)
	}
	fmt.Fprintf(c.Root().Writer, "PR opened: %s\n", prURL)
	return nil
}
