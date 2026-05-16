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
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
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
				Value:   "http://localhost:5173,http://localhost:3000",
				Sources: cli.EnvVars("URBANIST_CORS_ORIGINS"),
			},
			// Postgres store wiring lands in a later commit. For now the
			// server always uses the in-memory store seeded with
			// LoadDevFixtures so /api/v1/lookup returns something useful.
		},
		Action: runServe,
	}
}

func runServe(ctx context.Context, c *cli.Command) error {
	logger := buildLogger(c.String("log-format"))

	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	logger.Info("store initialized", "kind", "memory", "fixtures", "dev")

	origins := splitCSV(c.String("cors-origins"))
	handler := httpapi.New(httpapi.Config{
		Store:        store,
		Logger:       logger,
		CORSOrigins:  origins,
		APIVersion:   "v1",
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
