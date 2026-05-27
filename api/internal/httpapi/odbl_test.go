package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

// TestODbLHeaders_PresentOnAPISuccessResponse asserts the middleware
// puts both attribution headers on every /api/v1/** 200.
func TestODbLHeaders_PresentOnAPISuccessResponse(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if got, want := resp.Header.Get("X-Data-License"), dataLicense; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), dataAttributionURL; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}
}

// TestODbLHeaders_AbsentOnHealthz pins the path-scoping decision:
// /healthz isn't a data endpoint and must NOT carry the attribution
// headers. If someone reroutes the middleware to the router root,
// this test fails.
func TestODbLHeaders_AbsentOnHealthz(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-Data-License"); got != "" {
		t.Errorf("X-Data-License on /healthz: want empty, got %q", got)
	}
	if got := resp.Header.Get("X-Data-Attribution"); got != "" {
		t.Errorf("X-Data-Attribution on /healthz: want empty, got %q", got)
	}
}

// TestODbLHeaders_PresentOnAPIErrorResponse documents the
// headers-on-every-response decision: even a 404 problem document
// under /api/v1 carries the attribution headers. Cheaper than
// status-sniffing and arguably more honest.
func TestODbLHeaders_PresentOnAPIErrorResponse(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions/totally-bogus")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}
	if got, want := resp.Header.Get("X-Data-License"), dataLicense; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), dataAttributionURL; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}
}

// TestRespondCollection_WrapsItemsAndSetsHeaders covers the helper's
// shape contract against a httptest.ResponseRecorder so we don't have
// to spin up a server. Asserts:
//   - status 200, Content-Type application/json
//   - meta.license / meta.attribution_url / meta.generated_at present
//   - data is the items passed in, in order
func TestRespondCollection_WrapsItemsAndSetsHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	items := []oapi.RegionSummary{
		{Region: oapi.Region{Slug: "a"}, OrgCount: 3},
		{Region: oapi.Region{Slug: "b"}, OrgCount: 1},
	}
	respondCollection(w, items)

	resp := w.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}
	var env oapi.RegionSummariesEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Meta.License != dataLicense {
		t.Errorf("meta.license: want %q, got %q", dataLicense, env.Meta.License)
	}
	if env.Meta.AttributionUrl != dataAttributionURL {
		t.Errorf("meta.attribution_url: want %q, got %q", dataAttributionURL, env.Meta.AttributionUrl)
	}
	// oapi-codegen typed `generated_at` as time.Time (format: date-time).
	// JSON round-trips through RFC3339; the decoded time should be
	// well-formed and recent.
	if env.Meta.GeneratedAt.IsZero() {
		t.Errorf("meta.generated_at: want a real time, got zero value")
	}
	if d := time.Since(env.Meta.GeneratedAt); d < 0 || d > 5*time.Second {
		t.Errorf("meta.generated_at: want within 5s of now, got delta %s", d)
	}
	if len(env.Data) != 2 {
		t.Fatalf("data length: want 2, got %d", len(env.Data))
	}
	if env.Data[0].Region.Slug != "a" || env.Data[1].Region.Slug != "b" {
		t.Errorf("data order: want [a, b], got [%s, %s]",
			env.Data[0].Region.Slug, env.Data[1].Region.Slug)
	}
}

// TestRespondCollection_NilSlice_EncodesEmptyArray guards against a
// future regression where a handler hands `nil` to respondCollection
// and the JSON encoder writes `"data": null`. Downstream clients can
// assume `data` is always an array.
func TestRespondCollection_NilSlice_EncodesEmptyArray(t *testing.T) {
	w := httptest.NewRecorder()
	respondCollection[oapi.Org](w, nil)

	body := w.Body.String()
	if !strings.Contains(body, `"data":[]`) {
		t.Errorf("body should contain \"data\":[], got %s", body)
	}
}

// TestRespondCollection_EmitsRFC3339UTCInBody asserts the JSON wire
// format of meta.generated_at is RFC3339 with a UTC zone ('Z'
// suffix) AND second precision (no fractional seconds). The decoded
// time.Time has lost the original string, so we extract the raw
// value from the body bytes.
func TestRespondCollection_EmitsRFC3339UTCInBody(t *testing.T) {
	w := httptest.NewRecorder()
	respondCollection[oapi.Org](w, nil)

	body := w.Body.String()
	const marker = `"generated_at":"`
	start := strings.Index(body, marker)
	if start == -1 {
		t.Fatalf("generated_at field missing: %s", body)
	}
	start += len(marker)
	end := strings.Index(body[start:], `"`)
	if end == -1 {
		t.Fatalf("generated_at not terminated: %s", body)
	}
	val := body[start : start+end]

	if !strings.HasSuffix(val, "Z") {
		t.Errorf("generated_at: want UTC ('Z' suffix), got %q", val)
	}
	// Second precision: RFC3339Nano renders nanos-zero times as
	// "...HH:MM:SSZ" with no '.'. If newMeta stops truncating, a
	// fractional component reappears and this fails.
	if strings.Contains(val, ".") {
		t.Errorf("generated_at: want second precision (no fractional digits), got %q", val)
	}
}

// TestNewMeta_EmitsRecentUTCTime asserts the helper produces a UTC
// timestamp matching wall-clock now (within a small window).
func TestNewMeta_EmitsRecentUTCTime(t *testing.T) {
	m := newMeta()

	if m.License != dataLicense {
		t.Errorf("license: want %q, got %q", dataLicense, m.License)
	}
	if m.AttributionUrl != dataAttributionURL {
		t.Errorf("attribution_url: want %q, got %q", dataAttributionURL, m.AttributionUrl)
	}
	if m.GeneratedAt.Location() != time.UTC {
		t.Errorf("generated_at location: want UTC, got %s", m.GeneratedAt.Location())
	}
	if d := time.Since(m.GeneratedAt); d < 0 || d > 5*time.Second {
		t.Errorf("generated_at: want within 5s of now, got delta %s", d)
	}
}
