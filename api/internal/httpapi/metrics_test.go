package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newMetricsTestServer mirrors newTestServer (lookup_test.go) but keeps
// a reference to the *Metrics so the test can scrape the registry
// directly. The dev fixtures back the store: 11217 is a known US hit,
// 00000 a known miss.
func newMetricsTestServer(t *testing.T) (*httptest.Server, *Metrics) {
	t.Helper()
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	m := NewMetrics()
	handler := New(Config{
		Store:      store,
		Logger:     slog.New(slog.DiscardHandler),
		APIVersion: "v1",
		Metrics:    m,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, m
}

func TestMetrics_LookupCounters(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	// One known hit, one known miss.
	for _, postal := range []string{"11217", "00000"} {
		resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=" + postal + "&country=US")
		if err != nil {
			t.Fatalf("GET %s: %v", postal, err)
		}
		resp.Body.Close()
	}

	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("US", "hit")); got != 1 {
		t.Errorf("atlas_lookup_total{country=US,result=hit} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.lookupTotal.WithLabelValues("US", "miss")); got != 1 {
		t.Errorf("atlas_lookup_total{country=US,result=miss} = %v, want 1", got)
	}
}

func TestMetrics_HTTPRequestsUseRoutePattern(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	// The route label must be the chi route pattern, not the raw path.
	if got := testutil.ToFloat64(m.httpRequests.WithLabelValues("/api/v1/lookup", "GET", "200")); got < 1 {
		t.Errorf("atlas_http_requests_total{route=/api/v1/lookup,method=GET,status=200} = %v, want >= 1", got)
	}
}

func TestMetrics_HandlerExposition(t *testing.T) {
	srv, m := newMetricsTestServer(t)

	// Populate at least one atlas_ series before scraping.
	resp, err := http.Get(srv.URL + "/api/v1/lookup?postal_code=11217&country=US")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("metrics content-type = %q, want to contain text/plain", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "atlas_lookup_total") {
		t.Fatalf("metrics body missing atlas_lookup_total; got:\n%s", body)
	}
}
