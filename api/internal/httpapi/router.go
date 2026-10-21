// Package httpapi wires the chi router and HTTP handlers for the
// Urbanist Atlas JSON API. Handlers parse the request, call into
// pkg/atlas, and encode the result; business logic lives in pkg/atlas.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/mjrossi/urbanist-atlas/api/internal/coverage"
	"github.com/mjrossi/urbanist-atlas/api/internal/usage"
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
	// ClientSecret enables the X-Atlas-Client gate on the /api/v1 data
	// endpoints. Empty disables it. /healthz, /readyz, the spec and
	// the admin subtree are never gated.
	ClientSecret string

	// Submissions is the SubmissionStore behind POST /api/v1/submissions
	// and the /api/v1/admin/submissions/* endpoints. Nil disables those
	// endpoints (they return 503).
	Submissions atlas.SubmissionStore

	// PromotionEnqueuer queues approved submissions for the GitHub PR
	// worker. With nil, the approve handler still flips status and
	// persists atlas.ErrPromotionDisabled as the promotion_error.
	PromotionEnqueuer PromotionEnqueuer

	// AdminToken gates admin endpoints behind a bearer token. Empty
	// disables admin endpoints entirely (they return 503).
	AdminToken string

	// SubmissionsRatePerHour caps POST /api/v1/submissions per client
	// (IPv4 address or IPv6 /64). Zero or negative falls back to 5/hour.
	SubmissionsRatePerHour int

	// Metrics, when non-nil, enables the Prometheus request middleware
	// and product counters. The /metrics endpoint itself is served on a
	// separate private listener (see cmd/server), not on this mux.
	Metrics *Metrics

	// Coverage, when non-nil, captures sampled empty-result lookups and
	// searches for coverage-gap analysis (see internal/coverage). Nil
	// disables capture; handlers call it unconditionally (it is nil-safe).
	Coverage *coverage.Recorder

	// Usage, when non-nil, accumulates daily aggregate usage counts
	// (content popularity + lookup outcomes) for the monthly digest.
	// Nil disables recording; handlers call it unconditionally (it is
	// nil-safe). See internal/usage.
	Usage *usage.Recorder

	// UsageCounts, when non-nil, backs the admin GET /api/v1/admin/usage
	// read endpoint. Satisfied by the same SQLite store as Submissions.
	UsageCounts atlas.UsageReader

	// CoverageGaps, when non-nil, backs the admin GET
	// /api/v1/admin/coverage-gaps read endpoint. Satisfied by the same
	// SQLite store as Submissions.
	CoverageGaps atlas.CoverageGapReader
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

	// Order matters. securityHeaders is outermost so every response,
	// probes and errors included, carries it (secheaders_test.go). The
	// recoverer is innermost so a panic becomes a 500 that logging and
	// metrics still record (TestNew_PanicIsLoggedAndCounted).
	r.Use(securityHeadersMiddleware)
	r.Use(requestIDMiddleware)
	r.Use(timeoutMiddleware(requestTimeout))
	r.Use(loggingMiddleware(logger))
	if len(cfg.CORSOrigins) > 0 {
		r.Use(corsMiddleware(cfg.CORSOrigins))
	}
	if cfg.Metrics != nil {
		r.Use(metricsMiddleware(cfg.Metrics))
	}
	r.Use(recovererMiddleware(logger))

	// chi's default 404 is text/plain and its 405 is empty, but the spec
	// requires problem+json for every error. The 405 reuses the not-found
	// type URI so the spec's problem catalog needs no new entry; its
	// status and title carry the meaning.
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
	getHead(r, readyzPath, readyHandler(cfg.Submissions, logger, cfg.Metrics))

	r.Route("/api/"+apiVersion, func(r chi.Router) {
		r.Use(odblHeadersMiddleware)
		// The spec is public so consumers can discover the contract
		// before any auth.
		getHead(r, "/openapi.yaml", openapiHandler())
		// Admin sits outside the client-secret group. The bearer token is
		// the stronger credential, and requiring the browser secret too
		// would only burden server-to-server callers.
		if cfg.Submissions != nil {
			r.Route("/admin", func(r chi.Router) {
				r.Use(bearerAuthMiddleware(cfg.AdminToken))
				r.Get("/submissions", listSubmissionsHandler(cfg.Submissions, logger))
				r.Post("/submissions/{id}/approve", approveSubmissionHandler(cfg.Submissions, cfg.PromotionEnqueuer, logger, cfg.Metrics))
				r.Post("/submissions/{id}/reject", rejectSubmissionHandler(cfg.Submissions, logger, cfg.Metrics))
				if cfg.CoverageGaps != nil {
					r.Get("/coverage-gaps", listCoverageGapsHandler(cfg.CoverageGaps, logger))
				}
				if cfg.UsageCounts != nil {
					r.Get("/usage", listUsageHandler(cfg.UsageCounts, logger))
				}
			})
		}

		r.Group(func(r chi.Router) {
			r.Use(clientSecretMiddleware(cfg.ClientSecret))
			getHead(r, "/lookup", lookupHandler(cfg.Store, logger, cfg.Metrics, cfg.Coverage, cfg.Usage))
			getHead(r, "/regions", listRegionsHandler(cfg.Store, logger))
			// chi prefers static segments over params, so "search" never
			// resolves as a slug (TestSearchRegions_DoesNotShadowGetRegionSlug).
			getHead(r, "/regions/search", searchRegionsHandler(cfg.Store, logger, cfg.Metrics, cfg.Coverage))
			getHead(r, "/regions/{slug}", getRegionHandler(cfg.Store, logger, cfg.Metrics, cfg.Usage))
			getHead(r, "/orgs/{slug}", getOrgHandler(cfg.Store, logger, cfg.Metrics, cfg.Usage))
			getHead(r, "/recent", recentHandler(cfg.Store, logger))
			getHead(r, "/stats", statsHandler(cfg.Store, logger))

			// The submission POST is browser-facing, so the deterrent applies.
			if cfg.Submissions != nil {
				limiter := newIPRateLimiter(cfg.SubmissionsRatePerHour, rateLimitWindow)
				r.With(
					pendingQueueGuard(cfg.Submissions, maxPendingSubmissions, logger, cfg.Metrics),
					submissionRateLimit(limiter, cfg.Metrics),
				).Post("/submissions", createSubmissionHandler(cfg.Submissions, cfg.Store, logger, cfg.Metrics))
			}
		})
	})

	return r
}

// getHead registers h for both GET and HEAD, since chi.Get alone
// answers HEAD with 405. net/http drops the body on HEAD, so reusing the
// GET handler meets RFC 9110 §9.3.2.
func getHead(r chi.Router, pattern string, h http.HandlerFunc) {
	r.Get(pattern, h)
	r.Head(pattern, h)
}
