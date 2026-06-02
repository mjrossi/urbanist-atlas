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

	// Metrics, when non-nil, enables the Prometheus request middleware
	// and product counters. The /metrics endpoint itself is served on a
	// separate private listener (see cmd/server), not on this mux.
	Metrics *Metrics
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
	// recoverer, handlers) sees a consistent rid; timeoutMiddleware
	// next so every handler runs with a per-request deadline ctx;
	// recoverer next so panics in business logic don't escape;
	// logger last so the access log records the final status
	// (including 500s from recoverer).
	r.Use(requestIDMiddleware)
	r.Use(timeoutMiddleware(requestTimeout))
	r.Use(recovererMiddleware(logger))
	r.Use(loggingMiddleware(logger))
	if len(cfg.CORSOrigins) > 0 {
		r.Use(corsMiddleware(cfg.CORSOrigins))
	}
	// metricsMiddleware sits inside corsMiddleware so the preflight
	// OPTIONS that CORS short-circuits with a 204 aren't recorded as
	// "unmatched" request noise. It reuses loggingMiddleware's status
	// recorder rather than wrapping the writer a second time.
	if cfg.Metrics != nil {
		r.Use(metricsMiddleware(cfg.Metrics))
	}

	// chi's defaults answer an unmatched route with a stdlib text/plain
	// 404 and an unmatched method with an empty 405 — both violate the
	// spec's "all error responses use application/problem+json" contract
	// (openapi.yaml ProblemDetails / RFC 9457). Route them through the
	// same writeProblem emitter every handled error uses. The handlers
	// stay thin (pull rid from ctx → writeProblem); writeProblem owns the
	// RFC-9457 body, so there is no second encoder.
	//
	// Both reuse an EXISTING catalog type URI (problemNotFound) — no new
	// problemMethodNotAllowed const, which would force an openapi.yaml
	// mirror edit (D-09 keeps the spec exclusive to plan 03-01). The 405's
	// status code + title carry the method-not-allowed semantics; the type
	// URI is a stable, already-published catalog entry.
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusNotFound, problemNotFound,
			"Not Found", "The requested resource does not exist.",
			requestIDFromContext(r.Context()))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, r, http.StatusMethodNotAllowed, problemNotFound,
			"Method Not Allowed", "This method is not supported for the requested resource.",
			requestIDFromContext(r.Context()))
	})

	getHead(r, healthzPath, healthHandler())
	getHead(r, readyzPath, readyHandler(cfg.Submissions, logger))

	r.Route("/api/"+apiVersion, func(r chi.Router) {
		r.Use(odblHeadersMiddleware)
		// /openapi.yaml stays public so consumers can discover the
		// wire contract before any auth. Registered BEFORE the
		// gated group so the client-secret middleware doesn't reach it.
		getHead(r, "/openapi.yaml", openapiHandler())
		r.Group(func(r chi.Router) {
			r.Use(clientSecretMiddleware(cfg.ClientSecret))
			getHead(r, "/lookup", lookupHandler(cfg.Store, logger, cfg.Metrics))
			getHead(r, "/regions", listRegionsHandler(cfg.Store, logger))
			getHead(r, "/regions/{slug}", getRegionHandler(cfg.Store, logger))
			getHead(r, "/orgs/{slug}", getOrgHandler(cfg.Store, logger))
			getHead(r, "/recent", recentHandler(cfg.Store, logger))

			if cfg.Submissions != nil {
				limiter := newIPRateLimiter(cfg.SubmissionsRatePerHour, rateLimitWindow)
				r.Post("/submissions", createSubmissionHandler(cfg.Submissions, cfg.Store, limiter, logger, cfg.Metrics))
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

// getHead registers `h` to handle both GET and HEAD for `pattern`.
// chi.Get alone returns 405 to HEAD requests, which breaks
// link-preview tools (Slack, Discord, etc.) that HEAD a URL before
// unfurling, and uptime probes that prefer HEAD.
//
// Per RFC 9110 §9.3.2, a HEAD response must be identical to the GET
// response except for the absence of a body. Go's net/http already
// honors that: when the request method is HEAD, the server's response
// writer suppresses body bytes automatically. So reusing the GET
// handler is correct and produces a compliant Content-Length without
// any per-handler bookkeeping.
func getHead(r chi.Router, pattern string, h http.HandlerFunc) {
	r.Get(pattern, h)
	r.Head(pattern, h)
}
