package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi"
	"github.com/mjrossi/urbanist-atlas/api/internal/loaddata"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
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
				Usage:   "directory containing regions_<cc>.toml, postal_codes_<cc>.csv, orgs.toml",
				Value:   "./seed",
				Sources: cli.EnvVars("URBANIST_SEED_DIR"),
			},
			&cli.StringFlag{
				Name:    "client-secret",
				Usage:   "shared secret expected in the X-Atlas-Client header; empty disables the gate",
				Sources: cli.EnvVars("URBANIST_CLIENT_SECRET"),
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

	origins := splitCSV(c.String("cors-origins"))
	handler := httpapi.New(httpapi.Config{
		Store:        store,
		Logger:       logger,
		CORSOrigins:  origins,
		APIVersion:   "v1",
		ClientSecret: c.String("client-secret"),
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
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
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
// serve a few sample orgs without a seed directory on disk — useful
// for demos and ad-hoc CLI testing.
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
		if seedDir == "" {
			return nil, nil, errors.New("serve: --seed-dir or URBANIST_SEED_DIR is required when --store=file")
		}
		s, err := loaddata.BuildMemStore(logger, seedDir)
		if err != nil {
			return nil, nil, err
		}
		logger.Info("store initialized", "kind", storeKindFile, "seed_dir", seedDir)
		return s, func() {}, nil
	default:
		return nil, nil, fmt.Errorf("serve: unknown --store value %q (want %q or %q)", kind, storeKindFile, storeKindMemory)
	}
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
