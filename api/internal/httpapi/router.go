// Package httpapi wires the chi router and HTTP handlers for the
// Urbanist Atlas JSON API. Handlers are deliberately thin — they parse
// the request, call into pkg/atlas, and encode the result. Business
// logic lives in pkg/atlas.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

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
	// ClientSecret enables the X-Atlas-Client shared-secret gate
	// (Phase 1 launch lockdown — slice #23 / CLAUDE.md § Launch
	// strategy). Empty disables the gate; non-empty requires the
	// matching header on every /api/v1/* data endpoint. /healthz and
	// /api/v1/openapi.yaml stay exempt either way.
	ClientSecret string

	// Submissions is the SubmissionStore behind POST /api/v1/submissions
	// and the /api/v1/admin/submissions/* endpoints. Nil disables those
	// endpoints (they return 503).
	Submissions atlas.SubmissionStore

	// PromotionEnqueuer queues approved submissions for the GitHub PR
	// worker. Nil is a recognized "no worker configured" state — the
	// approve handler still flips status but persists
	// `promotion_error="worker disabled (no token configured)"`.
	PromotionEnqueuer PromotionEnqueuer

	// AdminToken gates admin endpoints behind a bearer token. Empty
	// disables admin endpoints entirely (they return 503).
	AdminToken string

	// SubmissionsRatePerHour caps POST /api/v1/submissions per source
	// IP. Zero or negative falls back to a default (5/hour).
	SubmissionsRatePerHour int
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
	r.Get("/readyz", readyHandler(cfg.Store, logger))

	r.Route("/api/"+apiVersion, func(r chi.Router) {
		r.Use(odblHeadersMiddleware)
		// /openapi.yaml stays public so consumers can discover the
		// wire contract before any auth. Registered BEFORE the
		// gated group so the client-secret middleware doesn't reach it.
		r.Get("/openapi.yaml", openapiHandler())
		r.Group(func(r chi.Router) {
			r.Use(clientSecretMiddleware(cfg.ClientSecret))
			r.Get("/lookup", lookupHandler(cfg.Store, logger))
			r.Get("/regions", listRegionsHandler(cfg.Store, logger))
			r.Get("/regions/{slug}", getRegionHandler(cfg.Store, logger))
			r.Get("/orgs/{slug}", getOrgHandler(cfg.Store, logger))
			r.Get("/recent", recentHandler(cfg.Store, logger))

			if cfg.Submissions != nil {
				limiter := newIPRateLimiter(cfg.SubmissionsRatePerHour, time.Hour)
				r.Post("/submissions", createSubmissionHandler(cfg.Submissions, cfg.Store, limiter, logger))
				r.Route("/admin", func(r chi.Router) {
					r.Use(bearerAuthMiddleware(cfg.AdminToken))
					r.Get("/submissions", listSubmissionsHandler(cfg.Submissions, logger))
					r.Post("/submissions/{id}/approve", approveSubmissionHandler(cfg.Submissions, cfg.PromotionEnqueuer, logger))
					r.Post("/submissions/{id}/reject", rejectSubmissionHandler(cfg.Submissions, logger))
				})
			}
		})
	})

	return r
}
