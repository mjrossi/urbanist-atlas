package main

import "github.com/urfave/cli/v3"

// loadpostalCommand is a placeholder. Once the Postgres store lands,
// this subcommand ingests Census ZCTA / HUD ZIP-place crosswalks
// (US) and Statistics Canada FSA data (Canada) into the postal_codes
// and regions tables.
func loadpostalCommand() *cli.Command {
	return &cli.Command{
		Name:  "loadpostal",
		Usage: "Load postal-code → region data into the database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "src",
				Usage:    "path to CSV source file",
				Required: false, // becomes required once implemented
			},
			&cli.StringFlag{
				Name:  "country",
				Usage: "country of the source data: US or CA",
				Value: "US",
			},
		},
		Action: notImplemented("loadpostal"),
	}
}
