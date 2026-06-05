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
	submissions  *prometheus.CounterVec
	rateLimit    *prometheus.CounterVec
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
