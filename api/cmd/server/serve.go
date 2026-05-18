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
	"github.com/mjrossi/urbanist-atlas/api/internal/store/postgres"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

const (
	storeKindPostgres = "postgres"
	storeKindMemory   = "memory"
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
				Usage:   "store backing: postgres or memory",
				Value:   storeKindPostgres,
				Sources: cli.EnvVars("URBANIST_STORE"),
			},
			&cli.StringFlag{
				Name:    "db-url",
				Usage:   "Postgres connection string (required when --store=postgres)",
				Sources: cli.EnvVars("URBANIST_DB_URL"),
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

// buildStore returns an atlas.Store backed by either Postgres or the
// in-memory MemStore, depending on --store. Memory is opt-in (for
// tests and offline use); the default is Postgres so production
// configurations are accidentally-correct rather than accidentally-
// fixture-backed.
func buildStore(ctx context.Context, c *cli.Command, logger *slog.Logger) (atlas.Store, func(), error) {
	kind := strings.ToLower(strings.TrimSpace(c.String("store")))
	switch kind {
	case storeKindMemory:
		s := atlas.NewMemStore()
		atlas.LoadDevFixtures(s)
		logger.Info("store initialized", "kind", storeKindMemory, "fixtures", "dev")
		return s, func() {}, nil
	case storeKindPostgres, "":
		dbURL := c.String("db-url")
		if dbURL == "" {
			return nil, nil, errors.New("serve: --db-url or URBANIST_DB_URL is required when --store=postgres")
		}
		s, closeFn, err := postgres.Open(ctx, dbURL)
		if err != nil {
			return nil, nil, err
		}
		logger.Info("store initialized", "kind", storeKindPostgres)
		return s, closeFn, nil
	default:
		return nil, nil, fmt.Errorf("serve: unknown --store value %q (want %q or %q)", kind, storeKindPostgres, storeKindMemory)
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
