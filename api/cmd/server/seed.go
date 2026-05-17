package main

import "github.com/urfave/cli/v3"

// seedCommand is a placeholder. Once the Postgres store lands, this
// subcommand loads api/seed/orgs.yaml into the organizations and
// organization_regions tables.
func seedCommand() *cli.Command {
	return &cli.Command{
		Name:  "seed",
		Usage: "Load curated org seed data (api/seed/orgs.yaml) into the database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "src",
				Usage: "path to seed YAML",
				Value: "./seed/orgs.yaml",
			},
		},
		Action: notImplemented("seed"),
	}
}
