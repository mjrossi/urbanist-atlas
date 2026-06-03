package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
)

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
