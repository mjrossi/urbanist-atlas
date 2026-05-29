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
				Name:    "log-format",
				Usage:   "log output format: json or text",
				Value:   "json",
				Sources: cli.EnvVars("URBANIST_LOG_FORMAT"),
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
	logger := buildLogger(c.String("log-format"))

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

	origins := splitCSV(c.String("cors-origins"))
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

// buildLogger returns an slog.Logger writing JSON or text to stderr.
func buildLogger(format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var h slog.Handler
	if strings.EqualFold(format, "text") {
		h = slog.NewTextHandler(os.Stderr, opts)
	} else {
		h = slog.NewJSONHandler(os.Stderr, opts)
	}
	return slog.New(h)
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
