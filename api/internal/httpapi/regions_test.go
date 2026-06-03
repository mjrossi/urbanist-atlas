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

func TestSearchRegions_HappyPath_ReturnsEnvelope(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/regions/search?q=brooklyn")
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
	// ODbL attribution rides on every collection response.
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}

	var env oapi.RegionSearchResultsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Meta.License != "ODbL-1.0" {
		t.Errorf("meta.license: want %q, got %q", "ODbL-1.0", env.Meta.License)
	}
	if len(env.Data) == 0 {
		t.Fatal("want at least one result for 'brooklyn', got 0")
	}
	if env.Data[0].Region.Slug != "brooklyn-ny" {
		t.Errorf("top result: want slug %q, got %q", "brooklyn-ny", env.Data[0].Region.Slug)
	}
	// Disambiguation hint resolves the nearest state ancestor (NY).
	if env.Data[0].ContextLabel != "NY" {
		t.Errorf("context_label: want %q, got %q", "NY", env.Data[0].ContextLabel)
	}
}

func TestSearchRegions_EmptyQuery_ReturnsEmptyData(t *testing.T) {
	srv := newTestServer(t)

	// Blank q and an entirely absent q both yield a 200 with `data: []`
	// — no special-casing needed on the client.
	for _, url := range []string{
		srv.URL + "/api/v1/regions/search?q=",
		srv.URL + "/api/v1/regions/search",
	} {
		resp, err := http.Get(url)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status want 200, got %d", url, resp.StatusCode)
			}
			var env oapi.RegionSearchResultsEnvelope
			if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if env.Data == nil {
				t.Errorf("GET %s: data want non-nil empty array, got null", url)
			}
			if len(env.Data) != 0 {
				t.Errorf("GET %s: data want empty, got %d", url, len(env.Data))
			}
		}()
	}
}

func TestSearchRegions_BadLimit_Returns400(t *testing.T) {
	srv := newTestServer(t)

	for _, limit := range []string{"0", "21", "abc", "-3"} {
		resp, err := http.Get(srv.URL + "/api/v1/regions/search?q=brooklyn&limit=" + limit)
		if err != nil {
			t.Fatalf("GET limit=%s: %v", limit, err)
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("limit=%s: status want 400, got %d", limit, resp.StatusCode)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
				t.Errorf("limit=%s: Content-Type want problem+json, got %q", limit, ct)
			}
		}()
	}
}

// TestSearchRegions_DoesNotShadowGetRegionSlug pins that the static
// "/regions/search" route resolves to the search handler (200 envelope)
// rather than being captured by "/regions/{slug}" as slug="search"
// (which would 404), and that real slugs still resolve on the param
// route.
func TestSearchRegions_DoesNotShadowGetRegionSlug(t *testing.T) {
	srv := newTestServer(t)

	// "/regions/search" → search handler (200), not a 404 slug lookup.
	searchResp, err := http.Get(srv.URL + "/api/v1/regions/search?q=toronto")
	if err != nil {
		t.Fatalf("GET search: %v", err)
	}
	defer searchResp.Body.Close()
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("search route: want 200, got %d", searchResp.StatusCode)
	}
	var env oapi.RegionSearchResultsEnvelope
	if err := json.NewDecoder(searchResp.Body).Decode(&env); err != nil {
		t.Fatalf("decode search envelope: %v", err)
	}
	if len(env.Data) == 0 {
		t.Error("search route returned no results for 'toronto'")
	}

	// "/regions/{slug}" still resolves a real slug to a RegionDetail.
	slugResp, err := http.Get(srv.URL + "/api/v1/regions/toronto-on")
	if err != nil {
		t.Fatalf("GET slug: %v", err)
	}
	defer slugResp.Body.Close()
	if slugResp.StatusCode != http.StatusOK {
		t.Fatalf("slug route: want 200, got %d", slugResp.StatusCode)
	}
	var detail oapi.RegionDetail
	if err := json.NewDecoder(slugResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode region detail: %v", err)
	}
	if detail.Region.Slug != "toronto-on" {
		t.Errorf("slug route: want region toronto-on, got %q", detail.Region.Slug)
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
	// At least one bucket must populate (the dev fixture has orgs in
	// nyc-metro's scope). Both being empty would be a regression.
	if len(got.Local) == 0 && len(got.Regional) == 0 {
		t.Error("local + regional both empty; want >= 1 org in scope")
	}
	// Every returned LookupOrg has its regions populated and carries
	// matched_region_slugs (the slugs that caused it to surface for
	// this region's scope).
	for _, o := range append(append([]oapi.LookupOrg{}, got.Local...), got.Regional...) {
		if len(o.Regions) == 0 {
			t.Errorf("org %s has no regions populated", o.Slug)
		}
		if len(o.MatchedRegionSlugs) == 0 {
			t.Errorf("org %s has no matched_region_slugs", o.Slug)
		}
	}
	// Ancestry must be present (non-nil) on every successful detail
	// response — the SPA's breadcrumb depends on it being an array,
	// even an empty one. nyc-metro's parent in the dev fixture is
	// ny (us:state), so we expect at least one ancestor.
	if got.Ancestry == nil {
		t.Error("ancestry: want non-nil array, got nil")
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
	// Downward walk picks up nyc-metro + Brooklyn + their orgs.
	if len(got.Local) == 0 && len(got.Regional) == 0 {
		t.Error("local + regional both empty for /regions/ny")
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
	if prob.Title != "Region Not Found" {
		t.Errorf("title: want %q, got %q", "Region Not Found", prob.Title)
	}
	if prob.RequestId == nil || *prob.RequestId == "" {
		t.Errorf("request_id: want non-empty, got %v", prob.RequestId)
	}
}
