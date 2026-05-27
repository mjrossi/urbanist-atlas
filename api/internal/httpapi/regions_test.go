package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

func TestListRegions_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions")
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
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), "https://urbanistatlas.com"; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}

	var env oapi.RegionSummariesEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Meta.License != "ODbL-1.0" {
		t.Errorf("meta.license: want %q, got %q", "ODbL-1.0", env.Meta.License)
	}
	if env.Meta.AttributionUrl != "https://urbanistatlas.com" {
		t.Errorf("meta.attribution_url: want %q, got %q",
			"https://urbanistatlas.com", env.Meta.AttributionUrl)
	}
	if env.Meta.GeneratedAt.IsZero() {
		t.Errorf("meta.generated_at: want a real time, got zero value")
	}
	if d := time.Since(env.Meta.GeneratedAt); d < 0 || d > 5*time.Second {
		t.Errorf("meta.generated_at: want within 5s of now, got delta %s", d)
	}

	got := env.Data
	if len(got) == 0 {
		t.Fatal("want at least one region, got 0")
	}
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
	for _, p := range got {
		if p.Region.ParentSlugs == nil {
			t.Errorf("region %s has nil parent_slugs (must be at least [])", p.Region.Slug)
		}
	}
}

func TestGetRegion_HappyPath_Metro(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions/nyc-metro")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}
	var got oapi.RegionDetail
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

// TestGetRegion_NonBrowseableKindResolves pins the broadened-detail
// contract: a state slug (us:state — outside defaultBrowseKinds) now
// resolves, returning the descendant orgs. This replaces the old
// "non-place slug → 404" behavior.
func TestGetRegion_NonBrowseableKindResolves(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions/ny")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d (body should resolve, since ny is non-national)", resp.StatusCode)
	}
	var got oapi.RegionDetail
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Region.Slug != "ny" {
		t.Errorf("region.slug: want ny, got %s", got.Region.Slug)
	}
	if got.Region.Kind != "us:state" {
		t.Errorf("region.kind: want us:state, got %s", got.Region.Kind)
	}
	// Descendants include nyc-metro + Brooklyn + their tagged orgs.
	if len(got.Orgs) == 0 {
		t.Errorf("orgs: want >= 1 (descendant walk), got 0")
	}
}

func TestGetRegion_404_UnknownSlug(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions/totally-bogus")
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
