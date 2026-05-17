package main

import (
	"context"
	"errors"

	"github.com/urfave/cli/v3"
)

// notImplemented returns a cli.ActionFunc that errors out with a
// human-readable "not yet implemented" message. Used by the seed and
// loadpostal subcommands until their data pipelines land (roadmap
// slices #3 and #4).
func notImplemented(name string) cli.ActionFunc {
	return func(context.Context, *cli.Command) error {
		return errors.New(name + ": not yet implemented")
	}
}
