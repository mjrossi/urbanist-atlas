package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

func TestListMetros_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}

	var got []oapi.MetroSummary
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("want at least one metro, got 0")
	}
	// Ordering: org_count DESC, then name ASC.
	for i := 1; i < len(got); i++ {
		if got[i].OrgCount > got[i-1].OrgCount {
			t.Errorf("not descending by org_count at [%d]: %d > %d",
				i, got[i].OrgCount, got[i-1].OrgCount)
		}
		if got[i].OrgCount == got[i-1].OrgCount && got[i].Region.Name < got[i-1].Region.Name {
			t.Errorf("not alphabetical within count tie at [%d]: %q < %q",
				i, got[i].Region.Name, got[i-1].Region.Name)
		}
	}
	// Region must have parent_slugs as a non-null array (even if empty).
	for _, m := range got {
		if m.Region.ParentSlugs == nil {
			t.Errorf("metro %s has nil parent_slugs (must be at least [])", m.Region.Slug)
		}
	}
}

func TestGetMetro_HappyPath(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros/nyc-metro")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type: want application/json prefix, got %q", ct)
	}

	var got oapi.MetroDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Region.Slug != "nyc-metro" {
		t.Errorf("region.slug: want nyc-metro, got %s", got.Region.Slug)
	}
	if len(got.Orgs) == 0 {
		t.Errorf("orgs: want >= 1, got 0")
	}
	for _, o := range got.Orgs {
		if len(o.Regions) == 0 {
			t.Errorf("org %s has no regions populated", o.Slug)
		}
	}
}

func TestGetMetro_404(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros/totally-bogus")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type: want application/problem+json, got %q", ct)
	}

	var prob oapi.ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if prob.Type != problemNotFound {
		t.Errorf("type: want %q, got %q", problemNotFound, prob.Type)
	}
	if prob.Status != int32(http.StatusNotFound) {
		t.Errorf("status: want 404, got %d", prob.Status)
	}
	if prob.Title != "Not Found" {
		t.Errorf("title: want %q, got %q", "Not Found", prob.Title)
	}
	if prob.RequestId == nil || *prob.RequestId == "" {
		t.Errorf("request_id: want non-empty, got %v", prob.RequestId)
	}
}

func TestGetMetro_NonMetroSlug_404(t *testing.T) {
	// The dev fixtures contain "ny" (us:state) — it exists as a region
	// but is NOT a metro-equivalent kind. The handler must reject it
	// with 404, not return a half-formed MetroDetail.
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/metros/ny")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: want 404, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type: want application/problem+json, got %q", ct)
	}
}
