package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/coverage"
	"github.com/mjrossi/urbanist-atlas/api/internal/githubpr"
	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi"
	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

const (
	storeKindFile   = "file"
	storeKindMemory = "memory"
)

// serveConfig is the fully-resolved serve configuration, read once from
// the CLI flags by parseServeConfig. Holding every value in one struct
// keeps each flag (and its env fallback) read exactly once and lets
// runServe and its build helpers read as orchestration over a plain
// value rather than re-querying *cli.Command at every use site.
type serveConfig struct {
	port                   string
	metricsPort            string
	logFormat              string
	logLevel               string
	corsOrigins            string // raw CSV; split via splitCSV at use
	store                  string
	seedDir                string
	clientSecret           string
	dbPath                 string
	adminToken             string
	githubToken            string
	submissionsRatePerHour int
	coverageSampleRate     float64
	coverageMaxRows        int
	usageFlushInterval     time.Duration
	usageKeepDays          int
}

// parseServeConfig reads every serve flag exactly once. Flag names,
// env-var fallbacks, and defaults all live on the cli.Command flag set
// (serveCommand); this only snapshots the resolved values.
func parseServeConfig(c *cli.Command) serveConfig {
	return serveConfig{
		port:                   c.String("port"),
		metricsPort:            c.String("metrics-port"),
		logFormat:              c.String("log-format"),
		logLevel:               c.String("log-level"),
		corsOrigins:            c.String("cors-origins"),
		store:                  c.String("store"),
		seedDir:                c.String("seed-dir"),
		clientSecret:           c.String("client-secret"),
		dbPath:                 c.String("db-path"),
		adminToken:             c.String("admin-token"),
		githubToken:            c.String("github-token"),
		submissionsRatePerHour: c.Int("submissions-rate-per-hour"),
		coverageSampleRate:     c.Float("coverage-sample-rate"),
		coverageMaxRows:        c.Int("coverage-max-rows"),
		usageFlushInterval:     c.Duration("usage-flush-interval"),
		usageKeepDays:          c.Int("usage-keep-days"),
	}
}

func serveCommand() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Run the HTTP API server",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "port",
				Usage:   "TCP port to listen on",
				Value:   "8080",
				Sources: cli.EnvVars("URBANIST_PORT"),
			},
			&cli.StringFlag{
				Name:    "metrics-port",
				Usage:   "TCP port for the private Prometheus /metrics listener; empty or 0 disables it",
				Value:   "9091",
				Sources: cli.EnvVars("URBANIST_METRICS_PORT"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "log output format: json or text",
				Value:   "json",
				Sources: cli.EnvVars("URBANIST_LOG_FORMAT"),
			},
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "minimum log level: debug, info, warn, or error",
				Value:   "info",
				Sources: cli.EnvVars("URBANIST_LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:    "cors-origins",
				Usage:   "comma-separated allowed CORS origins (exact match)",
				Value:   "http://localhost:5173",
				Sources: cli.EnvVars("URBANIST_CORS_ORIGINS"),
			},
			&cli.StringFlag{
				Name:    "store",
				Usage:   "store backing: file (default; reads from --seed-dir) or memory (built-in dev fixtures)",
				Value:   storeKindFile,
				Sources: cli.EnvVars("URBANIST_STORE"),
			},
			&cli.StringFlag{
				Name:    "seed-dir",
				Usage:   "directory containing regions_<cc>.toml, postal_codes_<cc>.csv, orgs.toml; empty (default) uses the bundle embedded in the binary",
				Sources: cli.EnvVars("URBANIST_SEED_DIR"),
			},
			&cli.StringFlag{
				Name:    "client-secret",
				Usage:   "shared secret expected in the X-Atlas-Client header; empty disables the gate",
				Sources: cli.EnvVars("URBANIST_CLIENT_SECRET"),
			},
			&cli.StringFlag{
				Name:    "db-path",
				Usage:   "SQLite database path for the submission queue; empty disables /submissions endpoints",
				Value:   "/data/atlas.db",
				Sources: cli.EnvVars("URBANIST_DB_PATH"),
			},
			&cli.StringFlag{
				Name:    "admin-token",
				Usage:   "bearer token guarding /api/v1/admin/*; empty disables admin endpoints (they 503)",
				Sources: cli.EnvVars("URBANIST_ADMIN_TOKEN"),
			},
			&cli.StringFlag{
				Name:    "github-token",
				Usage:   "fine-grained GitHub PAT for the promotion PR worker; empty disables the worker",
				Sources: cli.EnvVars("URBANIST_GITHUB_TOKEN"),
			},
			&cli.IntFlag{
				Name:    "submissions-rate-per-hour",
				Usage:   "per-IP cap on POST /api/v1/submissions",
				Value:   5,
				Sources: cli.EnvVars("URBANIST_SUBMISSIONS_RATE_PER_HOUR"),
			},
			&cli.FloatFlag{
				Name:    "coverage-sample-rate",
				Usage:   "probability (0..1) of persisting an empty-result lookup/search for coverage-gap analysis; 0 disables capture",
				Value:   0,
				Sources: cli.EnvVars("URBANIST_COVERAGE_SAMPLE_RATE"),
			},
			&cli.IntFlag{
				Name:    "coverage-max-rows",
				Usage:   "cap on retained coverage_gaps rows (pruned after each write); 0 leaves it unbounded",
				Value:   5000,
				Sources: cli.EnvVars("URBANIST_COVERAGE_MAX_ROWS"),
			},
			&cli.DurationFlag{
				Name:    "usage-flush-interval",
				Usage:   "how often buffered usage counts are written to SQLite",
				Value:   time.Minute,
				Sources: cli.EnvVars("URBANIST_USAGE_FLUSH_INTERVAL"),
			},
			&cli.IntFlag{
				Name: "usage-keep-days",
				// 400 days keeps a full year plus a month of margin, so
				// a year-over-year comparison is always available.
				Usage:   "days of daily usage rollups to retain; 0 disables pruning",
				Value:   400,
				Sources: cli.EnvVars("URBANIST_USAGE_KEEP_DAYS"),
			},
		},
		Action: runServe,
	}
}

func runServe(ctx context.Context, c *cli.Command) error {
	cfg := parseServeConfig(c)
	logger := buildLogger(cfg.logFormat, cfg.logLevel)

	store, closeStore, err := buildStore(cfg, logger)
	if err != nil {
		return err
	}
	defer closeStore()

	subs, closeSubs, err := buildSubmissionStore(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer closeSubs()

	var (
		enqueuer httpapi.PromotionEnqueuer
		worker   *githubpr.Worker
	)
	if subs != nil {
		worker = githubpr.New(githubpr.Config{
			Token:         cfg.githubToken,
			Logger:        logger,
			PersistResult: subs.AttachPromotionResult,
		})
		go worker.Run(ctx)
		enqueuer = worker
	}

	metrics := httpapi.NewMetrics()

	// Coverage-gap recorder shares the SQLite store with submissions, so
	// it only exists when that store does. Disabled by default
	// (sample-rate 0) — capturing raw empty-result input is opt-in.
	var recorder *coverage.Recorder
	if subs != nil {
		recorder = coverage.New(subs, cfg.coverageSampleRate, cfg.coverageMaxRows, logger)
	}

	// Usage rollups share the SQLite store with submissions, so like the
	// coverage recorder they exist only when that store does. Unlike
	// coverage, this is on by default: the buckets hold public content
	// identifiers only, and recording them is the point of the slice.
	var usageRec *usage.Recorder
	if subs != nil {
		usageRec = usage.New(subs, cfg.usageFlushInterval, cfg.usageKeepDays, logger)
		// Start, not `go Run`: it registers with the recorder's
		// WaitGroup synchronously, so the shutdown Wait below is
		// guaranteed to join this goroutine.
		usageRec.Start(ctx)
	}

	origins := splitCSV(cfg.corsOrigins)
	logServeConfig(logger, cfg, origins, subs != nil)

	handler := httpapi.New(httpapi.Config{
		Store:                  store,
		Logger:                 logger,
		CORSOrigins:            origins,
		APIVersion:             "v1",
		ClientSecret:           cfg.clientSecret,
		Submissions:            submissionsOrNil(subs),
		PromotionEnqueuer:      enqueuer,
		AdminToken:             cfg.adminToken,
		SubmissionsRatePerHour: cfg.submissionsRatePerHour,
		Metrics:                metrics,
		Coverage:               recorder,
		CoverageGaps:           coverageReaderOrNil(subs),
		Usage:                  usageRec,
		UsageCounts:            usageReaderOrNil(subs),
	})

	addr := net.JoinHostPort("", cfg.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Private Prometheus listener. Binds all interfaces on Fly (see
	// newMetricsServer) so Fly's managed Prometheus can scrape it; the
	// port is never internet-routable because it isn't declared in
	// [http_service]/[[services]]. Off the main mux on purpose; nil when
	// disabled. ListenAndServe errors on this listener are logged, never
	// fatal: losing metrics must not take the request path down.
	metricsSrv := newMetricsServer(cfg.metricsPort, metrics, logger)
	metricsDone := make(chan struct{}, 1)
	if metricsSrv != nil {
		go func() {
			logger.Info("metrics listening", "addr", metricsSrv.Addr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server error", "err", err)
			}
			metricsDone <- struct{}{}
		}()
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	return awaitShutdown(ctx, logger, shutdownDeps{
		srv:         srv,
		serverErr:   serverErr,
		metricsSrv:  metricsSrv,
		metricsDone: metricsDone,
		worker:      worker,
		recorder:    recorder,
		usageRec:    usageRec,
	})
}

// logServeConfig emits the single consolidated boot line so an operator
// can confirm the effective non-secret config from the first lines of
// the log. Secrets are reported as presence booleans only — never their
// values.
func logServeConfig(logger *slog.Logger, cfg serveConfig, origins []string, submissionsEnabled bool) {
	logger.Info("startup config",
		"port", cfg.port,
		"store", cfg.store,
		"seed_source", seedSourceLabel(cfg.seedDir),
		"cors_origins", origins,
		"log_format", cfg.logFormat,
		"log_level", cfg.logLevel,
		"metrics_port", cfg.metricsPort,
		"submissions_enabled", submissionsEnabled,
		"submissions_rate_per_hour", cfg.submissionsRatePerHour,
		"coverage_sample_rate", cfg.coverageSampleRate,
		"coverage_max_rows", cfg.coverageMaxRows,
		"usage_flush_interval", cfg.usageFlushInterval,
		"usage_keep_days", cfg.usageKeepDays,
		"client_secret_set", cfg.clientSecret != "",
		"admin_token_set", cfg.adminToken != "",
		"github_token_set", cfg.githubToken != "",
	)
}

// shutdownDeps groups the running servers and background workers
// awaitShutdown tears down, so the orchestration in runServe passes one
// value instead of a long parameter list.
type shutdownDeps struct {
	srv         *http.Server
	serverErr   <-chan error
	metricsSrv  *http.Server
	metricsDone <-chan struct{}
	worker      *githubpr.Worker
	recorder    *coverage.Recorder
	usageRec    *usage.Recorder
}

// awaitShutdown blocks until either the OS signal (ctx cancellation) or
// the main HTTP listener errors, then performs the dependency-ordered
// teardown. The order is load-bearing: stop accepting new HTTP traffic
// first, then drain GitHub-PR jobs queued by approvals that landed in the
// last few seconds (reversing it would let a fresh approval squeeze in
// after the worker stopped), then flush sampled coverage-gap writes
// before the deferred store Close runs. Every teardown step shares one
// shutdownCtx deadline; whichever step hits it first wins.
func awaitShutdown(ctx context.Context, logger *slog.Logger, d shutdownDeps) error {
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := d.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if d.metricsSrv != nil {
			if err := d.metricsSrv.Shutdown(shutdownCtx); err != nil {
				logger.Warn("metrics graceful shutdown incomplete", "err", err)
			}
			// Join the metrics goroutine so a slow drain can't race
			// process exit. Shutdown already closed the listener, so
			// ListenAndServe has returned (or is about to); this is the
			// symmetric counterpart to the <-serverErr join below.
			<-d.metricsDone
		}
		if d.worker != nil {
			dropped, stopErr := d.worker.Stop(shutdownCtx)
			if stopErr != nil {
				logger.Warn("githubpr: drain incomplete on shutdown",
					"err", stopErr,
					"dropped_count", len(dropped),
					"dropped_ids", dropped)
			}
		}
		// Drain in-flight coverage-gap writes before the deferred store
		// Close runs, so sampled rows aren't lost on shutdown. Bounded by
		// the shared shutdownCtx so a wedged write can't overrun the
		// shutdown budget; stragglers (sampled, best-effort) are dropped.
		// Nil-safe.
		if err := d.recorder.Wait(shutdownCtx); err != nil {
			logger.Warn("coverage: drain incomplete on shutdown", "err", err)
		}
		// Flush buffered usage counts and join the recorder's ticker
		// goroutine before the deferred store Close runs, so the last
		// interval isn't lost and no flush is still in flight when the
		// database closes. The goroutine does its own final flush on ctx
		// cancellation using a detached context, so the join — not just
		// the flush — is what makes closing the store safe here.
		// Nil-safe.
		if err := d.usageRec.Wait(shutdownCtx); err != nil {
			logger.Warn("usage: final flush incomplete on shutdown", "err", err)
		}
		// Wait for ListenAndServe to return (it will, with ErrServerClosed).
		<-d.serverErr
		return nil
	case err := <-d.serverErr:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	}
}

// buildStore returns an atlas.Store backed by the file-loaded
// MemStore (default) or the built-in dev fixtures (--store=memory).
// The dev-fixture path stays available so the binary can boot and
// serve a few sample orgs without a seed bundle — useful for demos.
//
// When --seed-dir is empty (production default) the loader reads
// from the seedfs.FS embed baked into the binary; when non-empty
// it reads from os.DirFS(seedDir). The latter is what mise.development
// activates (URBANIST_SEED_DIR=api/seed) so dev iterates against
// the on-disk files without rebuilds.
func buildStore(cfg serveConfig, logger *slog.Logger) (atlas.Store, func(), error) {
	kind := strings.ToLower(strings.TrimSpace(cfg.store))
	switch kind {
	case storeKindMemory:
		s := atlas.NewMemStore()
		atlas.LoadDevFixtures(s)
		logger.Info("store initialized", "kind", storeKindMemory, "fixtures", "dev")
		return s, func() {}, nil
	case storeKindFile, "":
		seedDir := cfg.seedDir
		var (
			source string
			seedFS fs.FS
		)
		if seedDir == "" {
			source = "embed"
			seedFS = seedfs.FS
		} else {
			source = seedDir
			seedFS = os.DirFS(seedDir)
		}
		s, err := seedfiles.BuildMemStore(logger, seedFS)
		if err != nil {
			return nil, nil, err
		}
		logger.Info("store initialized", "kind", storeKindFile, "source", source)
		return s, func() {}, nil
	default:
		return nil, nil, fmt.Errorf("serve: unknown --store value %q (want %q or %q)", kind, storeKindFile, storeKindMemory)
	}
}

// seedSourceLabel describes where the file store reads its seed bundle
// from for the startup-config log line: "embed" (the binary-baked
// seedfs.FS, the production default) when --seed-dir is empty, otherwise
// the directory path. Mirrors the source resolution in buildStore.
func seedSourceLabel(seedDir string) string {
	if strings.TrimSpace(seedDir) == "" {
		return "embed"
	}
	return seedDir
}

// buildSubmissionStore opens the SQLite submission database and runs
// migrations. An empty --db-path is treated as "no submissions" — the
// returned store is nil and the /submissions endpoints stay
// unregistered.
func buildSubmissionStore(ctx context.Context, cfg serveConfig, logger *slog.Logger) (*sqlite.Store, func(), error) {
	path := strings.TrimSpace(cfg.dbPath)
	if path == "" {
		logger.Info("submission store disabled (empty --db-path)")
		return nil, func() {}, nil
	}
	// SQLite won't create missing parent dirs (it'd surface as
	// SQLITE_CANTOPEN). Skip the mkdir for the in-memory URIs.
	if !strings.HasPrefix(path, ":memory:") && !strings.HasPrefix(path, "file::memory:") {
		if dir := filepath.Dir(path); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, func() {}, fmt.Errorf("submission store: mkdir %s: %w", dir, err)
			}
		}
	}
	s, err := sqlite.Open(path)
	if err != nil {
		return nil, func() {}, fmt.Errorf("submission store: %w", err)
	}
	if err := s.Migrate(ctx); err != nil {
		_ = s.Close()
		return nil, func() {}, fmt.Errorf("submission store migrate: %w", err)
	}
	logger.Info("submission store initialized", "path", path)
	return s, func() { _ = s.Close() }, nil
}

// submissionsOrNil converts the concrete *sqlite.Store into the
// atlas.SubmissionStore interface, preserving nil-ness so the router
// can detect the "disabled" case (a typed nil would route to handlers
// that then segfault on store calls).
func submissionsOrNil(s *sqlite.Store) atlas.SubmissionStore {
	if s == nil {
		return nil
	}
	return s
}

// usageReaderOrNil converts the concrete *sqlite.Store into the
// atlas.UsageReader interface, preserving nil-ness (same typed-nil guard
// as submissionsOrNil) so the router skips registering the admin usage
// route when there is no store.
func usageReaderOrNil(s *sqlite.Store) atlas.UsageReader {
	if s == nil {
		return nil
	}
	return s
}

// coverageReaderOrNil mirrors usageReaderOrNil for the coverage-gap seam.
func coverageReaderOrNil(s *sqlite.Store) atlas.CoverageGapReader {
	if s == nil {
		return nil
	}
	return s
}

// buildLogger returns an slog.Logger writing JSON or text to stderr at the
// given minimum level.
func buildLogger(format, level string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var h slog.Handler
	if strings.EqualFold(format, "text") {
		h = slog.NewTextHandler(os.Stderr, opts)
	} else {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(h)
}

// parseLevel maps a log-level string to an slog.Level. Matching is
// case-insensitive and tolerant of surrounding whitespace; empty or
// unrecognized values fall back to Info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default: // "info" and anything unrecognized
		return slog.LevelInfo
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// newMetricsServer builds the private Prometheus listener for the given
// port, or returns nil when metrics are disabled (empty port or "0").
//
// On Fly it binds ALL interfaces (":port"), NOT FLY_PRIVATE_IP. Fly's
// managed-Prometheus scraper reaches the instance on a different
// interface than the 6PN address, so binding only to FLY_PRIVATE_IP
// leaves /metrics reachable via `fly proxy` (6PN) yet never scraped — no
// atlas_* series ever land in Fly's Prometheus. The endpoint stays
// private regardless of bind address: Fly's edge only routes ports
// declared in [http_service]/[[services]], and the metrics port
// deliberately isn't one. Locally (no FLY_PRIVATE_IP) it binds loopback
// so a dev machine doesn't expose it on the LAN.
func newMetricsServer(port string, metrics *httpapi.Metrics, logger *slog.Logger) *http.Server {
	port = strings.TrimSpace(port)
	if port == "" || port == "0" {
		logger.Info("metrics server disabled")
		return nil
	}
	host := "127.0.0.1"
	if os.Getenv("FLY_PRIVATE_IP") != "" {
		host = "" // all interfaces, so Fly's metrics scraper can reach it
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	return &http.Server{
		Addr:              net.JoinHostPort(host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
