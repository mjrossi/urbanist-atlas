// Package httpapi wires the chi router and HTTP handlers for the
// Urbanist Atlas JSON API. Handlers are deliberately thin — they parse
// the request, call into pkg/atlas, and encode the result. Business
// logic lives in pkg/atlas.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Config bundles the dependencies a router needs to be built. New
// returns an http.Handler, not a *chi.Mux, so callers can compose
// without depending on chi.
type Config struct {
	Store       atlas.Store
	Logger      *slog.Logger
	CORSOrigins []string
	APIVersion  string // typically "v1"
}

// New builds the full middleware stack and route table.
func New(cfg Config) http.Handler {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = "v1"
	}

	r := chi.NewRouter()

	// Order matters: requestID first so every later layer (logger,
	// recoverer, handlers) sees a consistent rid; recoverer next so
	// panics in business logic don't escape; logger last so the access
	// log records the final status (including 500s from recoverer).
	r.Use(requestIDMiddleware)
	r.Use(recovererMiddleware(logger))
	r.Use(loggingMiddleware(logger))
	if len(cfg.CORSOrigins) > 0 {
		r.Use(corsMiddleware(cfg.CORSOrigins))
	}

	r.Get("/healthz", healthHandler())

	r.Route("/api/"+apiVersion, func(r chi.Router) {
		r.Use(odblHeadersMiddleware)
		r.Get("/openapi.yaml", openapiHandler())
		r.Get("/lookup", lookupHandler(cfg.Store, logger))
		r.Get("/metros", listMetrosHandler(cfg.Store, logger))
		r.Get("/metros/{slug}", getMetroHandler(cfg.Store, logger))
		r.Get("/recent", recentHandler(cfg.Store, logger))
	})

	return r
}
