package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// Metrics owns the Prometheus registry and the application counters/histograms
// exposed on the private /metrics endpoint. It deliberately holds its own
// registry rather than the global default so multiple New()/test routers can
// coexist without duplicate-registration panics.
type Metrics struct {
	reg *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
	lookupTotal  *prometheus.CounterVec
	// lookupResults partitions resolved (hit) lookups by the most-specific
	// result tier so we can see, e.g., the empty-result rate — the editorial
	// coverage-gap signal. See incLookupTier / lookupTier.
	lookupResults        *prometheus.CounterVec
	regionViews          *prometheus.CounterVec
	orgViews             *prometheus.CounterVec
	regionSearch         *prometheus.CounterVec
	regionSearchResults  prometheus.Histogram
	regionSearchQueryLen prometheus.Histogram
	submissions          *prometheus.CounterVec
	// submissionValidationFailures breaks down rejected submissions by the
	// offending field (bounded via metricSubmissionField).
	submissionValidationFailures *prometheus.CounterVec
	adminActions                 *prometheus.CounterVec
	rateLimit                    *prometheus.CounterVec
	storePingFailures            prometheus.Counter
}

// NewMetrics builds a Metrics with a fresh registry, the standard Go runtime
// and process collectors, and the atlas-namespaced application vectors.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	factory := promauto.With(reg)
	return &Metrics{
		reg: reg,
		httpRequests: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests by route pattern, method, and status.",
		}, []string{"route", "method", "status"}),
		httpDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "atlas",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds by route pattern and method.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"route", "method"}),
		lookupTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "lookup_total",
			Help:      "Total postal-code lookups by country and result (hit|miss).",
		}, []string{"country", "result"}),
		submissions: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "submissions_total",
			Help:      "Total submission attempts by outcome status.",
		}, []string{"status"}),
		rateLimit: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "rate_limit_hits_total",
			Help:      "Total requests rejected by the per-IP rate limiter, by endpoint.",
		}, []string{"endpoint"}),
		lookupResults: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "lookup_results_total",
			Help:      "Resolved lookups by country and most-specific result tier (local|regional|statewide|empty). Sums to lookup_total{result=hit}.",
		}, []string{"country", "tier"}),
		regionViews: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "region_views_total",
			Help:      "Region detail fetches (GET /regions/{slug}) by whether the slug resolved.",
		}, []string{"found"}),
		orgViews: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "org_views_total",
			Help:      "Org detail fetches (GET /orgs/{slug}) by whether the slug resolved.",
		}, []string{"found"}),
		regionSearch: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "region_search_total",
			Help:      "Region type-ahead searches by whether any result was returned (nonempty|empty).",
		}, []string{"result"}),
		regionSearchResults: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: "atlas",
			Name:      "region_search_results",
			Help:      "Number of results returned per region type-ahead search.",
			Buckets:   []float64{0, 1, 2, 5, 10, 20, 50},
		}),
		regionSearchQueryLen: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: "atlas",
			Name:      "region_search_query_length",
			Help:      "Length in characters of the region type-ahead query. The query text itself is never recorded.",
			Buckets:   []float64{1, 2, 3, 5, 8, 13, 21, 34},
		}),
		submissionValidationFailures: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "submission_validation_failures_total",
			Help:      "Per-field submission validation failures, by field (bounded set; unknown keys collapse to other).",
		}, []string{"field"}),
		adminActions: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "admin_actions_total",
			Help:      "Admin submission state transitions by action (approve|reject) and outcome (ok|not_found|conflict|error).",
		}, []string{"action", "outcome"}),
		storePingFailures: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "atlas",
			Name:      "store_ping_failures_total",
			Help:      "Total /readyz store ping failures.",
		}),
	}
}

// incLookup records a lookup outcome. result is "hit" or "miss".
//
// country is caller-supplied query input (the handler does not gate on a
// known-country list), so it is bucketed to a bounded set here — only the
// shipping countries get their own series; everything else collapses to
// "other". Without this, a client looping unknown country codes would
// create an unbounded number of label series in the registry.
func (m *Metrics) incLookup(country, result string) {
	if m == nil {
		return
	}
	m.lookupTotal.WithLabelValues(metricCountry(country), result).Inc()
}

// metricCountry clamps a raw country string to a bounded label set so the
// lookup_total cardinality can't be inflated by arbitrary input.
func metricCountry(country string) string {
	switch atlas.Country(country) {
	case atlas.CountryUS, atlas.CountryCA:
		return country
	default:
		return "other"
	}
}

// incSubmissions records a submission outcome by status.
func (m *Metrics) incSubmissions(status string) {
	if m == nil {
		return
	}
	m.submissions.WithLabelValues(status).Inc()
}

// incRateLimit records a rate-limit rejection for the named endpoint.
func (m *Metrics) incRateLimit(endpoint string) {
	if m == nil {
		return
	}
	m.rateLimit.WithLabelValues(endpoint).Inc()
}

// incLookupTier records the result tier of a resolved (hit) lookup.
// Pair with incLookup(country, "hit") — there is exactly one tier
// increment per hit, so lookup_results_total sums to
// lookup_total{result="hit"}.
func (m *Metrics) incLookupTier(country, tier string) {
	if m == nil {
		return
	}
	m.lookupResults.WithLabelValues(metricCountry(country), tier).Inc()
}

// lookupTier picks the most-specific bucket that carried at least one
// org (local ≻ regional ≻ statewide), or "empty" when a resolved region
// surfaced no orgs in any tier. The "empty" bucket is the high-value
// coverage-gap signal that also drives the sampled-empty capture.
func lookupTier(local, regional, statewide int) string {
	switch {
	case local > 0:
		return "local"
	case regional > 0:
		return "regional"
	case statewide > 0:
		return "statewide"
	default:
		return "empty"
	}
}

// incRegionView records a GET /regions/{slug} fetch, labeled by whether
// the slug resolved.
func (m *Metrics) incRegionView(found bool) {
	if m == nil {
		return
	}
	m.regionViews.WithLabelValues(strconv.FormatBool(found)).Inc()
}

// incOrgView records a GET /orgs/{slug} fetch, labeled by whether the
// slug resolved.
func (m *Metrics) incOrgView(found bool) {
	if m == nil {
		return
	}
	m.orgViews.WithLabelValues(strconv.FormatBool(found)).Inc()
}

// incRegionSearch records a region type-ahead query: the empty/nonempty
// counter, the result-count histogram, and the query-length histogram.
// The query text is never recorded — only its length.
func (m *Metrics) incRegionSearch(queryLen, resultCount int) {
	if m == nil {
		return
	}
	result := "nonempty"
	if resultCount == 0 {
		result = "empty"
	}
	m.regionSearch.WithLabelValues(result).Inc()
	m.regionSearchResults.Observe(float64(resultCount))
	m.regionSearchQueryLen.Observe(float64(queryLen))
}

// incSubmissionValidationFailure records one rejected field, bounded to
// the known SubmissionPayload field set via metricSubmissionField.
func (m *Metrics) incSubmissionValidationFailure(field string) {
	if m == nil {
		return
	}
	m.submissionValidationFailures.WithLabelValues(metricSubmissionField(field)).Inc()
}

// metricSubmissionField clamps an arbitrary field-error key to the known
// SubmissionPayload field set so the validation-failures cardinality
// can't be inflated by unexpected keys. Mirrors metricCountry.
func metricSubmissionField(field string) string {
	switch field {
	case "name", "short_desc", "website_url", "contact_url",
		"region_slugs", "submitter_name", "submitter_email", "submitter_note":
		return field
	default:
		return "other"
	}
}

// incAdminAction records an admin submission state transition.
func (m *Metrics) incAdminAction(action, outcome string) {
	if m == nil {
		return
	}
	m.adminActions.WithLabelValues(action, outcome).Inc()
}

// incStorePingFailure records a /readyz store ping failure.
func (m *Metrics) incStorePingFailure() {
	if m == nil {
		return
	}
	m.storePingFailures.Inc()
}

// Handler returns the Prometheus exposition handler bound to this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// metricsMiddleware records per-request count and duration, mirroring
// loggingMiddleware. When m is nil it is a pass-through. Cardinality is bounded
// by always labeling with the chi route pattern, never the raw URL path.
func metricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// Reuse the access-log middleware's status recorder when it is
			// already in the chain (loggingMiddleware runs just outside this
			// one) so a request carries at most one wrapper; only allocate
			// our own when this middleware is used standalone.
			rec, ok := w.(*statusRecorder)
			if !ok {
				rec = &statusRecorder{ResponseWriter: w, status: http.StatusOK}
				w = rec
			}
			next.ServeHTTP(w, r)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}
			m.httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
			m.httpDuration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
		})
	}
}
