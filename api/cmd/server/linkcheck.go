package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/linkcheck"
	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
)

func linkcheckCommand() *cli.Command {
	return &cli.Command{
		Name:  "linkcheck",
		Usage: "Probe website_url for every seed org and emit a TSV report",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "src",
				Usage: "path to seed TOML",
				Value: "./seed/orgs.toml",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "per-request timeout",
				Value: 15 * time.Second,
			},
			&cli.IntFlag{
				Name:  "concurrency",
				Usage: "max parallel requests",
				Value: 8,
			},
			&cli.StringFlag{
				Name:  "out",
				Usage: "output path for the TSV report; - means stdout",
				Value: "-",
			},
		},
		Action: runLinkcheck,
	}
}

func runLinkcheck(ctx context.Context, c *cli.Command) error {
	src := c.String("src")
	out := c.String("out")

	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("linkcheck: open %s: %w", src, err)
	}
	defer func() { _ = f.Close() }()

	file, err := seed.Parse(f)
	if err != nil {
		return err
	}

	results := linkcheck.Check(ctx, file.Orgs, linkcheck.Options{
		Timeout:     c.Duration("timeout"),
		Concurrency: c.Int("concurrency"),
	})

	var w io.Writer
	if out == "-" {
		w = os.Stdout
	} else {
		of, err := os.Create(out)
		if err != nil {
			return fmt.Errorf("linkcheck: create %s: %w", out, err)
		}
		defer func() { _ = of.Close() }()
		w = of
	}

	flagged := 0
	for _, r := range results {
		if r.Err != "" {
			flagged++
		}
	}

	if err := writeTSV(w, results); err != nil {
		return fmt.Errorf("linkcheck: write: %w", err)
	}

	fmt.Fprintf(os.Stderr, "linkcheck: %d orgs checked, %d flagged\n", len(results), flagged)

	if out == "-" && flagged > 0 {
		return fmt.Errorf("linkcheck: %d flagged result(s)", flagged)
	}
	return nil
}

func writeTSV(w io.Writer, results []linkcheck.Result) error {
	sorted := make([]linkcheck.Result, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		return linkcheckPriority(sorted[i]) < linkcheckPriority(sorted[j])
	})

	bw := bufio.NewWriter(w)
	if _, err := fmt.Fprintln(bw, "slug\tname\tstatus\tfinal_url\telapsed_ms\terror"); err != nil {
		return err
	}
	for _, r := range sorted {
		if _, err := fmt.Fprintf(bw, "%s\t%s\t%d\t%s\t%d\t%s\n",
			tsvEscape(r.Slug),
			tsvEscape(r.Name),
			r.Status,
			tsvEscape(r.FinalURL),
			r.ElapsedMs,
			tsvEscape(r.Err),
		); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// linkcheckPriority returns a sort key: 0 for actionable rows
// (transport error or HTTP >= 400), 1 for healthy 2xx/3xx. Within
// a tier the rows tie-break by slug to keep diffs stable.
func linkcheckPriority(r linkcheck.Result) string {
	if r.Status == 0 || r.Status >= 400 {
		return "0\t" + r.Slug
	}
	return "1\t" + r.Slug
}

func tsvEscape(s string) string {
	r := strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")
	return r.Replace(s)
}
