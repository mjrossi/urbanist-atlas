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

	// Country plans register with etl.Plans from init().
	_ "github.com/mjrossi/urbanist-atlas/api/internal/etl/ca"
	_ "github.com/mjrossi/urbanist-atlas/api/internal/etl/us"
)

// etlCommand runs the operator-side pipeline that turns upstream
// reference data into the seed files:
//
//	urbanist-atlas-server etl download   --country US
//	urbanist-atlas-server etl regenerate --country US
//
// download fetches each source of the country plan into etl/sources/<cc>/
// and checks its pinned SHA256 (mirrored in etl/SOURCES.md); regenerate
// writes the deterministic files under api/seed/. The US CBSA file ships
// as xlsx only, so a Python step in etl/SOURCES.md converts it to CSV
// between the two. Design: docs/superpowers/specs/2026-05-19-postal-coverage-design.md.
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
		Usage: "Reshape upstream postal/metro source data into the seed file shapes internal/seedfiles loads at API boot",
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

// resolvedPlan bundles the pieces both etl subcommands derive from the
// shared flags before doing their subcommand-specific work.
type resolvedPlan struct {
	country string
	logger  *slog.Logger
	plan    etl.Country
	srcDir  string
	target  etl.Target
}

// resolvePlan parses the flags both etl subcommands share. op prefixes
// its errors.
func resolvePlan(c *cli.Command, op string) (resolvedPlan, error) {
	country := c.String("country")
	if country == "" {
		return resolvedPlan{}, fmt.Errorf("%s: --country is required", op)
	}
	logger := buildLogger(c.String("log-format"), c.String("log-level"))

	plan, ok := etl.Plans[country]
	if !ok {
		return resolvedPlan{}, fmt.Errorf("%s: no plan registered for country %q (known: %s)", op, country, strings.Join(planCodes(), ", "))
	}

	srcDir := filepath.Join(c.String("src"), plan.SourcesDir)
	target, err := etl.ParseTarget(c.String("target"))
	if err != nil {
		return resolvedPlan{}, fmt.Errorf("%s: %w", op, err)
	}
	return resolvedPlan{country: country, logger: logger, plan: plan, srcDir: srcDir, target: target}, nil
}

func runEtlDownload(ctx context.Context, c *cli.Command) error {
	r, err := resolvePlan(c, "etl download")
	if err != nil {
		return err
	}
	country, logger, plan, srcDir, target := r.country, r.logger, r.plan, r.srcDir, r.target

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return fmt.Errorf("etl download: create %s: %w", srcDir, err)
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
	// etlDownloadMaxAttempts absorbs transient upstream failures (5xx,
	// 429, dropped connections) so one blip doesn't fail a download.
	etlDownloadMaxAttempts = 4
	// etlDownloadBaseDelay doubles on each retry.
	etlDownloadBaseDelay = 1 * time.Second
)

// etlSleep waits d or until ctx is canceled. It is a var so tests can
// skip the backoff.
var etlSleep = func(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// downloadSource fetches src.URL into dst and checks src.SHA256 when set.
// A dst that already matches is not re-fetched. Transient failures are
// retried with backoff; a checksum mismatch is not, and the file stays on
// disk so the operator can review the new upstream vintage.
func downloadSource(ctx context.Context, client *http.Client, src etl.SourceDescriptor, dst string, logger *slog.Logger) error {
	// A missing, partial or corrupt file fails the checksum and is re-fetched.
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
	// The last attempt is outside the loop because no backoff follows it.
	_, err := fetchOnce(ctx, client, src, dst, logger)
	return err
}

// fetchOnce GETs src.URL into dst and verifies the sha256. The bool
// reports whether a non-nil error is transient and worth retrying.
func fetchOnce(ctx context.Context, client *http.Client, src etl.SourceDescriptor, dst string, logger *slog.Logger) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// Transport errors (DNS, connection reset, timeout) are transient.
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

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	// The hash covers the streamed bytes, not what reached disk; a failed
	// Close is the signal that the two may differ.
	var closeErr error
	if cerr := f.Close(); cerr != nil {
		closeErr = fmt.Errorf("close %s: %w", dst, cerr)
	}
	if err != nil {
		// A read cut short mid-stream (dropped connection) is transient.
		return true, errors.Join(fmt.Errorf("write %s: %w", dst, err), closeErr)
	}
	if closeErr != nil {
		return false, closeErr
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
	r, err := resolvePlan(c, "etl regenerate")
	if err != nil {
		return err
	}
	country, logger, plan, srcDir, target := r.country, r.logger, r.plan, r.srcDir, r.target

	if plan.Regenerate == nil {
		// A plan may register before it implements Regenerate.
		logger.Info("etl regenerate: plan registered but Regenerate hook is nil (no-op)",
			"country", country,
		)
		return nil
	}

	outDir := c.String("out")

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

// planCodes returns the registered country codes sorted, so the error
// hint is stable across runs.
func planCodes() []string {
	out := make([]string, 0, len(etl.Plans))
	for k := range etl.Plans {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
