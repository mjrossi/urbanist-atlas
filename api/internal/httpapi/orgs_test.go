package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestGetOrg_HappyPath(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/orgs/transalt-brooklyn")
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
	// ODbL attribution headers ride every /api/v1/** response.
	if got, want := resp.Header.Get("X-Data-License"), "ODbL-1.0"; got != want {
		t.Errorf("X-Data-License: want %q, got %q", want, got)
	}
	if got, want := resp.Header.Get("X-Data-Attribution"), "https://urbanistatlas.com"; got != want {
		t.Errorf("X-Data-Attribution: want %q, got %q", want, got)
	}

	var got oapi.Org
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Slug != "transalt-brooklyn" {
		t.Errorf("slug: want transalt-brooklyn, got %s", got.Slug)
	}
	if got.Name == "" {
		t.Errorf("name: want non-empty, got empty")
	}
	if len(got.Regions) == 0 {
		t.Errorf("regions: want >= 1, got 0")
	}
	for _, r := range got.Regions {
		if r.ParentSlugs == nil {
			t.Errorf("region %s has nil parent_slugs (must be at least [])", r.Slug)
		}
	}
}

func TestGetOrg_404_UnknownSlug(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/orgs/totally-bogus")
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
	if prob.Title != "Organization Not Found" {
		t.Errorf("title: want %q, got %q", "Organization Not Found", prob.Title)
	}
	// Detail is the consumer-facing message the web UI renders verbatim
	// (no frontend-side copy), so pin it.
	wantDetail := "We don't have this organization in the atlas yet. It may not be indexed, or the link you followed may be out of date."
	if prob.Detail == nil || *prob.Detail != wantDetail {
		t.Errorf("detail: want %q, got %v", wantDetail, prob.Detail)
	}
	if prob.RequestId == nil || *prob.RequestId == "" {
		t.Errorf("request_id: want non-empty, got %v", prob.RequestId)
	}
}

func TestGetOrg_401_MissingClientSecret(t *testing.T) {
	// A server configured with a non-empty ClientSecret rejects requests
	// that omit the X-Atlas-Client header — same gate that protects
	// /lookup and /regions. The exact problem-type URI is pinned so a
	// future drift surfaces here.
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	handler := New(Config{
		Store:        store,
		Logger:       slog.New(slog.DiscardHandler),
		APIVersion:   "v1",
		ClientSecret: "the-secret",
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/api/v1/orgs/transalt-brooklyn")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: want 401, got %d", resp.StatusCode)
	}
	var prob oapi.ProblemDetails
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if prob.Type != problemUnauthorized {
		t.Errorf("type: want %q, got %q", problemUnauthorized, prob.Type)
	}
}
