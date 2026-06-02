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

	"github.com/mjrossi/urbanist-atlas/api/internal/githubpr"
	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi"
	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	"github.com/mjrossi/urbanist-atlas/api/internal/store/sqlite"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	seedfs "github.com/mjrossi/urbanist-atlas/api/seed"
)

const (
	storeKindFile   = "file"
	storeKindMemory = "memory"
)

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
		},
		Action: runServe,
	}
}

func runServe(ctx context.Context, c *cli.Command) error {
	logger := buildLogger(c.String("log-format"), c.String("log-level"))

	store, closeStore, err := buildStore(ctx, c, logger)
	if err != nil {
		return err
	}
	defer closeStore()

	subs, closeSubs, err := buildSubmissionStore(ctx, c, logger)
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
			Token:         c.String("github-token"),
			Logger:        logger,
			PersistResult: subs.AttachPromotionResult,
		})
		go worker.Run(ctx)
		enqueuer = worker
	}

	metrics := httpapi.NewMetrics()

	origins := splitCSV(c.String("cors-origins"))

	// One consolidated boot line so an operator can confirm the effective
	// non-secret config from the first lines of the log. Secrets are
	// reported as presence booleans only — never their values.
	logger.Info("startup config",
		"port", c.String("port"),
		"store", c.String("store"),
		"seed_source", seedSourceLabel(c.String("seed-dir")),
		"cors_origins", origins,
		"log_format", c.String("log-format"),
		"log_level", c.String("log-level"),
		"metrics_port", c.String("metrics-port"),
		"submissions_enabled", subs != nil,
		"submissions_rate_per_hour", c.Int("submissions-rate-per-hour"),
		"client_secret_set", c.String("client-secret") != "",
		"admin_token_set", c.String("admin-token") != "",
		"github_token_set", c.String("github-token") != "",
	)

	handler := httpapi.New(httpapi.Config{
		Store:                  store,
		Logger:                 logger,
		CORSOrigins:            origins,
		APIVersion:             "v1",
		ClientSecret:           c.String("client-secret"),
		Submissions:            submissionsOrNil(subs),
		PromotionEnqueuer:      enqueuer,
		AdminToken:             c.String("admin-token"),
		SubmissionsRatePerHour: c.Int("submissions-rate-per-hour"),
		Metrics:                metrics,
	})

	addr := net.JoinHostPort("", c.String("port"))
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Private Prometheus listener. Bound to the Fly 6PN private address
	// (FLY_PRIVATE_IP) when present so /metrics is never internet-routable
	// — Fly's managed Prometheus scrapes it over that private network. Off
	// the main mux on purpose; nil when disabled. ListenAndServe errors on
	// this listener are logged, never fatal: losing metrics must not take
	// the request path down.
	metricsSrv := newMetricsServer(c.String("metrics-port"), metrics, logger)
	if metricsSrv != nil {
		go func() {
			logger.Info("metrics listening", "addr", metricsSrv.Addr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server error", "err", err)
			}
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

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		// Order: stop accepting new HTTP traffic first, then drain
		// any GitHub-PR jobs already queued by approvals that landed
		// in the last few seconds. Reversing this would let a fresh
		// approval squeeze in after the worker stopped accepting.
		// Both calls share the same shutdownCtx deadline; whichever
		// hits it first wins.
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		if metricsSrv != nil {
			if err := metricsSrv.Shutdown(shutdownCtx); err != nil {
				logger.Warn("metrics graceful shutdown incomplete", "err", err)
			}
		}
		if worker != nil {
			dropped, stopErr := worker.Stop(shutdownCtx)
			if stopErr != nil {
				logger.Warn("githubpr: drain incomplete on shutdown",
					"err", stopErr,
					"dropped_count", len(dropped),
					"dropped_ids", dropped)
			}
		}
		// Wait for ListenAndServe to return (it will, with ErrServerClosed).
		<-serverErr
		return nil
	case err := <-serverErr:
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
func buildStore(_ context.Context, c *cli.Command, logger *slog.Logger) (atlas.Store, func(), error) {
	kind := strings.ToLower(strings.TrimSpace(c.String("store")))
	switch kind {
	case storeKindMemory:
		s := atlas.NewMemStore()
		atlas.LoadDevFixtures(s)
		logger.Info("store initialized", "kind", storeKindMemory, "fixtures", "dev")
		return s, func() {}, nil
	case storeKindFile, "":
		seedDir := c.String("seed-dir")
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
func buildSubmissionStore(ctx context.Context, c *cli.Command, logger *slog.Logger) (*sqlite.Store, func(), error) {
	path := strings.TrimSpace(c.String("db-path"))
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
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// newMetricsServer builds the private Prometheus listener for the given
// port, or returns nil when metrics are disabled (empty port or "0").
// It binds to the Fly private IP when available so the endpoint stays
// off the public internet, falling back to loopback for local dev.
func newMetricsServer(port string, metrics *httpapi.Metrics, logger *slog.Logger) *http.Server {
	port = strings.TrimSpace(port)
	if port == "" || port == "0" {
		logger.Info("metrics server disabled")
		return nil
	}
	host := os.Getenv("FLY_PRIVATE_IP")
	if host == "" {
		host = "127.0.0.1"
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metrics.Handler())
	return &http.Server{
		Addr:              net.JoinHostPort(host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
