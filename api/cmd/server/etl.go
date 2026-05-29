package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/etl"

	// Blank-import per-country plans so their init() blocks register
	// with etl.Plans. Adding a new country = add a blank import here.
	_ "github.com/mjrossi/urbanist-atlas/api/internal/etl/ca"
	_ "github.com/mjrossi/urbanist-atlas/api/internal/etl/us"
)

// etlCommand wraps the operator-side data pipeline that reshapes
// upstream postal-code and metro reference data (Census ZCTA + CBSA,
// Statistics Canada PCCF + CMA) into the seed file shapes loadregions
// and loadpostal already consume.
//
// Two sub-subcommands:
//
//	urbanist-atlas-server etl download   --country US
//	urbanist-atlas-server etl regenerate --country US
//
// download fetches every SourceDescriptor in the country plan via HTTP,
// streams each file into etl/sources/<country>/, and verifies the
// SHA256 against the value pinned in the plan (mirrored in
// etl/SOURCES.md). regenerate parses the staged sources and writes the
// deterministic seed TOML/CSV files under api/seed/. Country plans
// live in api/internal/etl/<cc>/ and self-register in etl.Plans via
// init() blocks.
//
// US note: the Census CBSA file ships as xlsx only. download writes
// list1_2023.xlsx; an out-of-band Python step (etl/scripts/xlsx_to_csv.py)
// converts it to list1_2023.csv, which regenerate consumes. See
// etl/SOURCES.md for the recipe.
//
// Design rationale at docs/superpowers/specs/2026-05-19-postal-coverage-design.md.
func etlCommand() *cli.Command {
	commonFlags := []cli.Flag{
		&cli.StringFlag{
			Name:     "country",
			Usage:    "country code to operate on (US, CA, ...)",
			Required: true,
		},
		&cli.StringFlag{
			Name:  "src",
			Usage: "directory holding upstream source files (per-country subdir layout)",
			Value: "./etl/sources",
		},
		&cli.StringFlag{
			Name:  "out",
			Usage: "directory to write generated seed files into",
			Value: "./seed",
		},
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
		Name:  "etl",
		Usage: "Reshape upstream postal/metro source data into the seed file shapes loadregions/loadpostal consume",
		Commands: []*cli.Command{
			{
				Name:   "download",
				Usage:  "Fetch upstream source files into etl/sources/<country>/ and validate checksums",
				Flags:  commonFlags,
				Action: runEtlDownload,
			},
			{
				Name:   "regenerate",
				Usage:  "Parse staged source files and write deterministic seed TOML/CSV under api/seed/",
				Flags:  commonFlags,
				Action: runEtlRegenerate,
			},
		},
	}
}

func runEtlDownload(ctx context.Context, c *cli.Command) error {
	country := c.String("country")
	if country == "" {
		return errors.New("etl download: --country is required")
	}
	logger := buildLogger(c.String("log-format"), c.String("log-level"))

	plan, ok := etl.Plans[country]
	if !ok {
		return fmt.Errorf("etl download: no plan registered for country %q (known: %s)", country, strings.Join(planCodes(), ", "))
	}

	srcDir := filepath.Join(c.String("src"), plan.SourcesDir)
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("etl download: create %s: %w", srcDir, err)
	}

	logger.Info("etl download: start",
		"country", country,
		"sources", len(plan.Sources),
		"src_dir", srcDir,
	)

	client := &http.Client{Timeout: 5 * time.Minute}
	for _, src := range plan.Sources {
		dst := filepath.Join(srcDir, src.Filename)
		if err := downloadSource(ctx, client, src, dst, logger); err != nil {
			return fmt.Errorf("etl download %s: %w", src.Filename, err)
		}
	}

	if country == "US" {
		logger.Info("etl download: complete — US has a follow-up xlsx conversion step before regenerate",
			"country", country,
			"hint", "python3 etl/scripts/xlsx_to_csv.py etl/sources/us/list1_2023.xlsx etl/sources/us/list1_2023.csv",
		)
	} else {
		logger.Info("etl download: complete", "country", country)
	}
	return nil
}

// downloadSource fetches src.URL, streams the body into dst while
// computing sha256, and verifies the result against src.SHA256 (if
// non-empty). A mismatch leaves the file on disk and returns an
// error — the operator can re-review the upstream vintage rather
// than losing the download.
func downloadSource(ctx context.Context, client *http.Client, src etl.SourceDescriptor, dst string, logger *slog.Logger) error {
	logger.Info("etl download: fetching",
		"filename", src.Filename,
		"url", src.URL,
		"vintage", src.Vintage,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http get: status %s", resp.Status)
	}

	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	got := hex.EncodeToString(h.Sum(nil))

	logger.Info("etl download: wrote",
		"filename", src.Filename,
		"bytes", n,
		"sha256", got,
	)

	if src.SHA256 != "" && got != src.SHA256 {
		return fmt.Errorf("sha256 mismatch for %s: got %s, want %s (upstream vintage may have changed — update SHA256 in the country plan + etl/SOURCES.md after reviewing the diff)",
			src.Filename, got, src.SHA256)
	}
	return nil
}

func runEtlRegenerate(ctx context.Context, c *cli.Command) error {
	country := c.String("country")
	if country == "" {
		return errors.New("etl regenerate: --country is required")
	}
	logger := buildLogger(c.String("log-format"), c.String("log-level"))

	plan, ok := etl.Plans[country]
	if !ok {
		return fmt.Errorf("etl regenerate: no plan registered for country %q (known: %s)", country, strings.Join(planCodes(), ", "))
	}
	if plan.Regenerate == nil {
		// Defensive: a country plan may register itself but defer
		// implementing Regenerate to a follow-up slice. US and CA both
		// ship complete plans today.
		logger.Info("etl regenerate: plan registered but Regenerate hook is nil (no-op)",
			"country", country,
		)
		return nil
	}

	srcDir := filepath.Join(c.String("src"), plan.SourcesDir)
	outDir := c.String("out")

	logger.Info("etl regenerate: start",
		"country", country,
		"sources", len(plan.Sources),
		"targets", len(plan.Targets),
		"src_dir", srcDir,
		"out_dir", outDir,
	)
	if err := plan.Regenerate(ctx, srcDir, outDir, logger); err != nil {
		return fmt.Errorf("etl regenerate %s: %w", country, err)
	}
	logger.Info("etl regenerate: complete", "country", country)
	return nil
}

func planCodes() []string {
	out := make([]string, 0, len(etl.Plans))
	for k := range etl.Plans {
		out = append(out, k)
	}
	return out
}
