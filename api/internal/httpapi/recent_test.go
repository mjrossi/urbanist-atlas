package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/httpapi/oapi"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestListRecent_HappyPath_ReturnsOAPIShape(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/v1/recent")
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

	var env oapi.RecentEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Meta.License != "ODbL-1.0" {
		t.Errorf("meta.license: want %q, got %q", "ODbL-1.0", env.Meta.License)
	}
	if env.Meta.GeneratedAt.IsZero() {
		t.Errorf("meta.generated_at: want a real time, got zero value")
	}

	got := env.Data
	// LoadDevFixtures seeds plain orgs only (no national-tier). Empty
	// is technically legal; non-empty is what we ship with.
	if len(got) > 10 {
		t.Errorf("len: want <= 10, got %d", len(got))
	}
	for _, o := range got {
		if len(o.Regions) == 0 {
			t.Errorf("org %s missing Regions hydration", o.Slug)
		}
	}
}

// TestListRecent_ExcludesNationalTier seeds a custom MemStore with both
// a plain org and a national-only org and asserts the national one
// stays out of /recent. The default newTestServer fixtures don't
// include a national-tier org, so this test builds its own server.
func TestListRecent_ExcludesNationalTier(t *testing.T) {
	s := atlas.NewMemStore()
	// Plain region + plain org (must surface).
	s.AddRegion(atlas.Region{
		ID: 1, Kind: "us:city", Name: "Brooklyn", Slug: "brooklyn-ny",
		Country: atlas.CountryUS, ScopeTier: atlas.ScopeLocal,
	})
	// National region + national-only org (must NOT surface).
	s.AddRegion(atlas.Region{
		ID: 99, Kind: "pt:nacional", Name: "Portugal (national)", Slug: "pt-nacional",
		Country: atlas.Country("PT"), ScopeTier: atlas.ScopeNational,
	})
	t0 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	s.AddOrg(atlas.Org{
		ID: 1, Slug: "plain-org", Name: "Plain",
		ShortDesc: "x", WebsiteURL: "https://x",
		CreatedAt: t0,
	}, []int64{1})
	s.AddOrg(atlas.Org{
		ID: 2, Slug: "mubi-nacional", Name: "MUBi (national)",
		ShortDesc: "x", WebsiteURL: "https://x",
		// Newer than the plain org — a forgotten filter would surface
		// this one at the top.
		CreatedAt: t0.Add(48 * time.Hour),
	}, []int64{99})

	handler := New(Config{
		Store:      s,
		Logger:     slog.New(slog.DiscardHandler),
		APIVersion: "v1",
	})
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/recent")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d", resp.StatusCode)
	}

	var env oapi.RecentEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := env.Data
	if len(got) != 1 {
		t.Fatalf("want exactly 1 org (plain-org); got %d (%v)", len(got), oapiOrgSlugs(got))
	}
	if got[0].Slug != "plain-org" {
		t.Errorf("[0]: want plain-org, got %s", got[0].Slug)
	}
	for _, o := range got {
		if o.Slug == "mubi-nacional" {
			t.Errorf("national-only org leaked into /recent: %s", o.Slug)
		}
	}
}

func oapiOrgSlugs(orgs []oapi.Org) []string {
	out := make([]string, len(orgs))
	for i, o := range orgs {
		out[i] = o.Slug
	}
	return out
}
