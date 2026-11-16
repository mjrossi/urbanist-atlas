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

// serveConfig is the resolved serve configuration, read once from the
// flags by parseServeConfig.
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

// parseServeConfig snapshots the resolved flag values; names, env vars
// and defaults live on serveCommand's flags.
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
				Usage:   "hourly cap on POST /api/v1/submissions per source address (IPv4 address or IPv6 /64); a separate service-wide cap also applies",
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
		worker = startPromotionWorker(ctx, githubpr.Config{
			Token:         cfg.githubToken,
			Logger:        logger,
			PersistResult: subs.AttachPromotionResult,
		})
		enqueuer = worker
	}

	metrics := httpapi.NewMetrics()

	// The recorders share the submissions SQLite store. Coverage capture
	// is opt-in (sample rate 0 by default) because it keeps raw input.
	var recorder *coverage.Recorder
	if subs != nil {
		recorder = coverage.New(subs, cfg.coverageSampleRate, cfg.coverageMaxRows, logger)
	}

	// Usage is on by default: its buckets hold only canonical slugs and
	// fixed enum values, never raw input.
	var usageRec *usage.Recorder
	if subs != nil {
		usageRec = usage.New(subs, cfg.usageFlushInterval, cfg.usageKeepDays, logger)
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

	// The metrics listener's errors are logged, never fatal: losing
	// metrics must not take the request path down.
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
		srv:             srv,
		serverErr:       serverErr,
		metricsSrv:      metricsSrv,
		metricsDone:     metricsDone,
		worker:          worker,
		recorder:        recorder,
		usageRec:        usageRec,
		httpTimeout:     shutdownHTTPTimeout,
		teardownTimeout: shutdownTeardownTimeout,
	})
}

// startPromotionWorker launches the PR worker detached from ctx's
// cancellation. Approvals keep arriving while srv.Shutdown drains, so
// the worker must keep reading until awaitShutdown calls Stop.
func startPromotionWorker(ctx context.Context, cfg githubpr.Config) *githubpr.Worker {
	w := githubpr.New(cfg)
	go w.Run(context.WithoutCancel(ctx))
	return w
}

// logServeConfig logs the effective config in one boot line. Secrets
// appear only as presence booleans.
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

// shutdownDeps is everything awaitShutdown tears down.
type shutdownDeps struct {
	srv         *http.Server
	serverErr   <-chan error
	metricsSrv  *http.Server
	metricsDone <-chan struct{}
	worker      *githubpr.Worker
	recorder    *coverage.Recorder
	usageRec    *usage.Recorder

	httpTimeout     time.Duration
	teardownTimeout time.Duration
}

// The two shutdown phases run back to back, so their sum (13s) must stay
// under fly.toml's kill_timeout (15s) or Fly SIGKILLs mid-teardown.
// Teardown gets its own budget so a slow HTTP drain can't starve it.
const (
	shutdownHTTPTimeout     = 6 * time.Second
	shutdownTeardownTimeout = 7 * time.Second
)

// awaitShutdown waits for ctx cancellation or a listener error, then tears
// down in order: HTTP first, so no approval can enqueue after the PR
// worker stops; then the worker; then the recorders, before the deferred
// store Close. Every step runs even if an earlier one fails.
func awaitShutdown(ctx context.Context, logger *slog.Logger, d shutdownDeps) error {
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		var errs []error

		httpCtx, cancelHTTP := context.WithTimeout(context.Background(), d.httpTimeout)
		defer cancelHTTP()
		if err := d.srv.Shutdown(httpCtx); err != nil {
			errs = append(errs, fmt.Errorf("graceful shutdown: %w", err))
		}
		if d.metricsSrv != nil {
			if err := d.metricsSrv.Shutdown(httpCtx); err != nil {
				logger.Warn("metrics graceful shutdown incomplete", "err", err)
			}
			<-d.metricsDone
		}

		teardownCtx, cancelTeardown := context.WithTimeout(context.Background(), d.teardownTimeout)
		defer cancelTeardown()
		if d.worker != nil {
			dropped, stopErr := d.worker.Stop(teardownCtx)
			if stopErr != nil {
				logger.Warn("githubpr: drain incomplete on shutdown",
					"err", stopErr,
					"dropped_count", len(dropped),
					"dropped_ids", dropped)
				errs = append(errs, fmt.Errorf("githubpr drain: %w", stopErr))
			}
		}
		// Both recorders are nil-safe. Waiting on them before the
		// deferred store Close keeps their last writes from racing it;
		// coverage stragglers are sampled and best-effort.
		if err := d.recorder.Wait(teardownCtx); err != nil {
			errs = append(errs, fmt.Errorf("coverage drain: %w", err))
		}
		if err := d.usageRec.Wait(teardownCtx); err != nil {
			errs = append(errs, fmt.Errorf("usage final flush: %w", err))
		}
		// Shutdown closes the listener before draining, so this returns
		// promptly even when the drain timed out.
		if err := <-d.serverErr; err != nil {
			errs = append(errs, fmt.Errorf("listen: %w", err))
		}
		return errors.Join(errs...)
	case err := <-d.serverErr:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	}
}

// buildStore loads the seed bundle into a MemStore, or the dev fixtures
// for --store=memory. An empty --seed-dir reads the bundle embedded in
// the binary; a set one reads that directory, so dev needs no rebuild.
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

// seedSourceLabel names buildStore's seed source for the boot log line.
func seedSourceLabel(seedDir string) string {
	if strings.TrimSpace(seedDir) == "" {
		return "embed"
	}
	return seedDir
}

// buildSubmissionStore opens and migrates the SQLite store. An empty
// --db-path returns nil, which leaves the submission routes unregistered.
func buildSubmissionStore(ctx context.Context, cfg serveConfig, logger *slog.Logger) (*sqlite.Store, func(), error) {
	path := strings.TrimSpace(cfg.dbPath)
	if path == "" {
		logger.Info("submission store disabled (empty --db-path)")
		return nil, func() {}, nil
	}
	// SQLite won't create missing parent dirs (SQLITE_CANTOPEN).
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

// submissionsOrNil keeps a nil store a nil interface, so the router
// sees the disabled case rather than a typed nil.
func submissionsOrNil(s *sqlite.Store) atlas.SubmissionStore {
	if s == nil {
		return nil
	}
	return s
}

// usageReaderOrNil is submissionsOrNil for atlas.UsageReader.
func usageReaderOrNil(s *sqlite.Store) atlas.UsageReader {
	if s == nil {
		return nil
	}
	return s
}

// coverageReaderOrNil is submissionsOrNil for atlas.CoverageGapReader.
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

// parseLevel maps a log-level name to an slog.Level, defaulting to Info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
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

// newMetricsServer builds the private Prometheus listener, or nil when
// the port is empty or "0". It binds loopback locally and all interfaces
// on Fly, where binding FLY_PRIVATE_IP alone was never scraped; see
// docs/architecture/http-api.md.
func newMetricsServer(port string, metrics *httpapi.Metrics, logger *slog.Logger) *http.Server {
	port = strings.TrimSpace(port)
	if port == "" || port == "0" {
		logger.Info("metrics server disabled")
		return nil
	}
	host := "127.0.0.1"
	if os.Getenv("FLY_PRIVATE_IP") != "" {
		host = ""
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	return &http.Server{
		Addr:              net.JoinHostPort(host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
