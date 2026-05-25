// Command server is the urbanist-atlas API binary. It owns every
// operational entry point — running the HTTP API, applying database
// migrations, loading postal-code datasets, seeding orgs — as
// subcommands of a single urfave/cli command tree.
//
//	urbanist-atlas-server serve
//	urbanist-atlas-server migrate up
//	urbanist-atlas-server loadpostal --src ./data/postal_us.csv
//	urbanist-atlas-server seed
//	urbanist-atlas-server loaddata
//	urbanist-atlas-server etl regenerate --country US
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
)

func main() {
	cmd := newRootCommand()

	// Cancel the context on SIGINT/SIGTERM so subcommands (especially
	// `serve`) can shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		// urfave already prints CLI usage errors; we only handle the
		// post-Action error case here. Treating context cancellation
		// as a clean exit (Ctrl-C during `serve`).
		if errors.Is(err, context.Canceled) {
			return
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cli.Command {
	return &cli.Command{
		Name:  "urbanist-atlas-server",
		Usage: "API server and operational tools for urbanistatlas.com",
		Commands: []*cli.Command{
			serveCommand(),
			migrateCommand(),
			loadregionsCommand(),
			loadpostalCommand(),
			linkcheckCommand(),
			seedCommand(),
			loaddataCommand(),
			etlCommand(),
		},
	}
}
