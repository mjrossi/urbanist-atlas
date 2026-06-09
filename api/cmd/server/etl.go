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
	"slices"
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
			Name:  "target",
			Usage: "which outputs to operate on: all, regions, or postal",
			Value: "all",
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
	target, err := etl.ParseTarget(c.String("target"))
	if err != nil {
		return fmt.Errorf("etl download: %w", err)
	}

	logger.Info("etl download: start",
		"country", country,
		"sources", len(plan.Sources),
		"target", target,
		"src_dir", srcDir,
	)

	client := &http.Client{Timeout: 5 * time.Minute}
	for _, src := range plan.Sources {
		if src.Optional {
			logger.Info("etl download: skipping account-gated source (fetch manually)",
				"filename", src.Filename, "url", src.URL)
			continue
		}
		if !src.FeedsTarget(target) {
			logger.Info("etl download: skipping source not needed for target",
				"filename", src.Filename, "target", target)
			continue
		}
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

const (
	// etlDownloadMaxAttempts bounds the retry loop for a single source.
	// Census/StatsCan serve 200s normally; the retries absorb transient
	// edge/CDN failures (5xx, 429, dropped connections) that would
	// otherwise flake the seed-determinism CI job on an isolated blip.
	etlDownloadMaxAttempts = 4
	// etlDownloadBaseDelay is the first backoff; it doubles on each retry
	// (1s, 2s, 4s).
	etlDownloadBaseDelay = 1 * time.Second
)

// etlSleep waits d or until ctx is canceled, whichever comes first,
// returning the context error if canceled. It is a package var so the
// download tests can stub out the real wait and not spend backoff seconds.
var etlSleep = func(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// downloadSource fetches src.URL into dst, verifying the result against
// src.SHA256 (if non-empty). It is idempotent: when dst already exists
// and already matches the pinned checksum, the network fetch is skipped
// entirely, so re-runs and CI cache hits are offline. Transient failures
// (transport errors, HTTP 5xx, 429) are retried with exponential backoff;
// a complete-but-mismatched download is deterministic and is NOT retried —
// the file is left on disk so the operator can re-review the upstream
// vintage rather than losing the download.
func downloadSource(ctx context.Context, client *http.Client, src etl.SourceDescriptor, dst string, logger *slog.Logger) error {
	// Idempotent skip: a previously downloaded file whose sha256 already
	// matches the pin needs no fetch. A missing, partial, or corrupt file
	// fails this check and falls through to a fresh download — self-healing.
	if src.SHA256 != "" {
		if got, err := fileSHA256(dst); err == nil && got == src.SHA256 {
			logger.Info("etl download: cached (sha256 match, skipping fetch)",
				"filename", src.Filename,
				"sha256", got,
				"dst", dst,
			)
			return nil
		}
	}

	logger.Info("etl download: fetching",
		"filename", src.Filename,
		"url", src.URL,
		"vintage", src.Vintage,
	)

	// Attempts 1..max-1 may retry; each transient failure backs off and
	// loops. The final attempt runs after the loop, so its return is the
	// genuine "every attempt failed" path rather than dead code following
	// an in-loop return. A non-retryable error short-circuits out directly.
	for attempt := 1; attempt < etlDownloadMaxAttempts; attempt++ {
		retryable, err := fetchOnce(ctx, client, src, dst, logger)
		if err == nil {
			return nil
		}
		if !retryable {
			return err
		}
		delay := etlDownloadBaseDelay << (attempt - 1) // 1s, 2s, 4s
		logger.Warn("etl download: transient failure, retrying",
			"filename", src.Filename,
			"attempt", attempt,
			"max", etlDownloadMaxAttempts,
			"delay", delay,
			"err", err,
		)
		if serr := etlSleep(ctx, delay); serr != nil {
			return serr
		}
	}
	// Final attempt: no backoff follows it, so its result is terminal —
	// nil on success, otherwise the error (retryable or not) is returned.
	_, err := fetchOnce(ctx, client, src, dst, logger)
	return err
}

// fetchOnce performs a single GET of src.URL into dst, computing and
// verifying the sha256. The bool reports whether a non-nil error is worth
// retrying: transport errors and HTTP 5xx/429 are transient; any other
// non-200 and a sha256 mismatch are deterministic.
func fetchOnce(ctx context.Context, client *http.Client, src etl.SourceDescriptor, dst string, logger *slog.Logger) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// Transport-level failures (DNS, connection reset, timeout) are transient.
		return true, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return transient, fmt.Errorf("http get: status %s", resp.Status)
	}

	f, err := os.Create(dst)
	if err != nil {
		return false, fmt.Errorf("create %s: %w", dst, err)
	}
	defer f.Close()

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	if err != nil {
		// A read cut short mid-stream (dropped connection) is transient.
		return true, fmt.Errorf("write %s: %w", dst, err)
	}
	got := hex.EncodeToString(h.Sum(nil))

	logger.Info("etl download: wrote",
		"filename", src.Filename,
		"bytes", n,
		"sha256", got,
	)

	if src.SHA256 != "" && got != src.SHA256 {
		return false, fmt.Errorf("sha256 mismatch for %s: got %s, want %s (upstream vintage may have changed — update SHA256 in the country plan + etl/SOURCES.md after reviewing the diff)",
			src.Filename, got, src.SHA256)
	}
	return false, nil
}

// fileSHA256 returns the hex-encoded sha256 of the file at path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
	target, err := etl.ParseTarget(c.String("target"))
	if err != nil {
		return fmt.Errorf("etl regenerate: %w", err)
	}

	logger.Info("etl regenerate: start",
		"country", country,
		"sources", len(plan.Sources),
		"targets", len(plan.Targets),
		"target", target,
		"src_dir", srcDir,
		"out_dir", outDir,
	)
	if err := plan.Regenerate(ctx, srcDir, outDir, target, logger); err != nil {
		return fmt.Errorf("etl regenerate %s: %w", country, err)
	}
	logger.Info("etl regenerate: complete", "country", country)
	return nil
}

// planCodes returns the registered country codes in sorted order so the
// "known: %s" hint in the no-plan error reads identically run-to-run —
// ranging etl.Plans directly would emit them in Go's randomized map
// order.
func planCodes() []string {
	out := make([]string, 0, len(etl.Plans))
	for k := range etl.Plans {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
