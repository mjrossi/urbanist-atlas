package linkcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/seed"
)

func TestCheck(t *testing.T) {
	t.Run("200 clean", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		got := Check(context.Background(), []seed.Org{{Slug: "ok", Name: "OK", WebsiteURL: srv.URL}}, Options{})
		if len(got) != 1 {
			t.Fatalf("want 1 result, got %d", len(got))
		}
		r := got[0]
		if r.Status != 200 {
			t.Errorf("status: want 200, got %d", r.Status)
		}
		if r.Err != "" {
			t.Errorf("err: want empty, got %q", r.Err)
		}
		if r.FinalURL != "" {
			t.Errorf("final_url: want empty (no redirect), got %q", r.FinalURL)
		}
	})

	t.Run("redirect populates FinalURL", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(target.Close)
		redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/landed", http.StatusMovedPermanently)
		}))
		t.Cleanup(redir.Close)

		got := Check(context.Background(), []seed.Org{{Slug: "r", Name: "R", WebsiteURL: redir.URL}}, Options{})
		r := got[0]
		if r.Status != 200 {
			t.Errorf("status: want 200, got %d", r.Status)
		}
		if r.Err != "" {
			t.Errorf("err: want empty, got %q", r.Err)
		}
		if !strings.HasSuffix(r.FinalURL, "/landed") {
			t.Errorf("final_url: want suffix /landed, got %q", r.FinalURL)
		}
	})

	t.Run("404 populates Err", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(srv.Close)

		got := Check(context.Background(), []seed.Org{{Slug: "gone", Name: "Gone", WebsiteURL: srv.URL}}, Options{})
		r := got[0]
		if r.Status != 404 {
			t.Errorf("status: want 404, got %d", r.Status)
		}
		if r.Err != "HTTP 404" {
			t.Errorf("err: want %q, got %q", "HTTP 404", r.Err)
		}
	})

	t.Run("HEAD 405 falls back to GET", func(t *testing.T) {
		var headCount, getCount int
		var mu = make(chan struct{}, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu <- struct{}{}
			defer func() { <-mu }()
			switch r.Method {
			case http.MethodHead:
				headCount++
				w.WriteHeader(http.StatusMethodNotAllowed)
			case http.MethodGet:
				getCount++
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusBadRequest)
			}
		}))
		t.Cleanup(srv.Close)

		got := Check(context.Background(), []seed.Org{{Slug: "fb", Name: "FB", WebsiteURL: srv.URL}}, Options{})
		r := got[0]
		if r.Status != 200 {
			t.Errorf("status: want 200 after GET fallback, got %d", r.Status)
		}
		if r.Err != "" {
			t.Errorf("err: want empty, got %q", r.Err)
		}
		if headCount != 1 || getCount != 1 {
			t.Errorf("expected 1 HEAD then 1 GET, got head=%d get=%d", headCount, getCount)
		}
	})

	t.Run("timeout yields zero status and non-empty err", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(2 * time.Second):
				w.WriteHeader(http.StatusOK)
			case <-r.Context().Done():
				return
			}
		}))
		t.Cleanup(srv.Close)

		got := Check(context.Background(), []seed.Org{{Slug: "slow", Name: "Slow", WebsiteURL: srv.URL}}, Options{Timeout: 50 * time.Millisecond})
		r := got[0]
		if r.Status != 0 {
			t.Errorf("status: want 0 on timeout, got %d", r.Status)
		}
		if r.Err == "" {
			t.Error("err: want non-empty on timeout, got empty")
		}
	})

	t.Run("results returned in input order", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		orgs := []seed.Org{
			{Slug: "a", Name: "A", WebsiteURL: srv.URL},
			{Slug: "b", Name: "B", WebsiteURL: srv.URL},
			{Slug: "c", Name: "C", WebsiteURL: srv.URL},
		}
		got := Check(context.Background(), orgs, Options{Concurrency: 3})
		for i, want := range []string{"a", "b", "c"} {
			if got[i].Slug != want {
				t.Errorf("results[%d].Slug: want %q, got %q", i, want, got[i].Slug)
			}
		}
	})
}
