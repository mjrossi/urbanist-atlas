// Command server is the urbanist-atlas API binary. It owns the
// runtime HTTP API and the operator-side ETL tooling that regenerates
// the bundled seed data:
//
//	urbanist-atlas-server serve
//	urbanist-atlas-server linkcheck
//	urbanist-atlas-server etl regenerate --country US
//
// There is no longer a database-seeding workflow: the TOML/CSV files
// under api/seed/ are the runtime source of truth (loaded into an
// in-memory FileStore at boot), so the loaddata / loadregions /
// loadpostal / seed / migrate subcommands have been retired.
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
			linkcheckCommand(),
			etlCommand(),
			seedCommand(),
			submissionsCommand(),
		},
	}
}
