// Command server is the urbanist-atlas API binary. It serves the HTTP
// API and carries the operator tooling:
//
//	urbanist-atlas-server serve
//	urbanist-atlas-server linkcheck
//	urbanist-atlas-server etl regenerate --country US
//	urbanist-atlas-server seed validate
//	urbanist-atlas-server submissions retry-pr --id <uuid>
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
	// os.Exit skips defers, so run's cleanup happens inside run.
	os.Exit(run())
}

func run() int {
	cmd := newRootCommand()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.Run(ctx, os.Args); err != nil {
		// A signal-driven cancellation is a clean exit.
		if errors.Is(err, context.Canceled) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
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
