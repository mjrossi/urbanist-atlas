package linkcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestCheck(t *testing.T) {
	t.Run("200 clean populates FinalURL", func(t *testing.T) {
		// Pins the slice that closed the direct-hit FinalURL coverage
		// gap: even when no redirect occurs, FinalURL must echo the
		// resolved URL so the TSV report has a value in every row.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		got := Check(context.Background(), []seedfiles.OrgEntry{{Org: atlas.Org{Slug: "ok", Name: "OK", WebsiteURL: srv.URL}}}, Options{})
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
		// HEAD probe is what the linkcheck issues; the httptest server
		// answers it directly so resp.Request.URL.String() ≡ srv.URL.
		if r.FinalURL != srv.URL {
			t.Errorf("final_url: want %q, got %q", srv.URL, r.FinalURL)
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

		got := Check(context.Background(), []seedfiles.OrgEntry{{Org: atlas.Org{Slug: "r", Name: "R", WebsiteURL: redir.URL}}}, Options{})
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

		got := Check(context.Background(), []seedfiles.OrgEntry{{Org: atlas.Org{Slug: "gone", Name: "Gone", WebsiteURL: srv.URL}}}, Options{})
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
		mu := make(chan struct{}, 1)
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

		got := Check(context.Background(), []seedfiles.OrgEntry{{Org: atlas.Org{Slug: "fb", Name: "FB", WebsiteURL: srv.URL}}}, Options{})
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

		got := Check(context.Background(), []seedfiles.OrgEntry{{Org: atlas.Org{Slug: "slow", Name: "Slow", WebsiteURL: srv.URL}}}, Options{Timeout: 50 * time.Millisecond})
		r := got[0]
		if r.Status != 0 {
			t.Errorf("status: want 0 on timeout, got %d", r.Status)
		}
		if r.Err == "" {
			t.Error("err: want non-empty on timeout, got empty")
		}
	})

	t.Run("context cancellation short-circuits dispatch", func(t *testing.T) {
		// A server that blocks until its request ctx fires, so with
		// Concurrency=1 the first probe pins the only slot and the
		// dispatch loop blocks trying to enqueue the second org. Issue
		// #31: canceling the parent ctx must unblock the dispatch loop's
		// semaphore send (not hang past cancellation) and mark every
		// not-yet-dispatched org with the cancellation cause.
		started := make(chan struct{}, 1)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)

		ctx, cancel := context.WithCancel(context.Background())
		orgs := []seedfiles.OrgEntry{
			{Org: atlas.Org{Slug: "a", Name: "A", WebsiteURL: srv.URL}},
			{Org: atlas.Org{Slug: "b", Name: "B", WebsiteURL: srv.URL}},
			{Org: atlas.Org{Slug: "c", Name: "C", WebsiteURL: srv.URL}},
		}

		done := make(chan []Result, 1)
		go func() { done <- Check(ctx, orgs, Options{Concurrency: 1}) }()

		// Wait until the first probe is actually in flight (holding the
		// single slot), then cancel so the dispatch loop is blocked on
		// the semaphore send when cancellation lands.
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("first probe never started")
		}
		cancel()

		var got []Result
		select {
		case got = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Check did not return after cancellation (dispatch loop pinned)")
		}

		if len(got) != 3 {
			t.Fatalf("want 3 results (input-order contract), got %d", len(got))
		}
		// Every row must carry an error and preserve input order/slug.
		for i, want := range []string{"a", "b", "c"} {
			if got[i].Slug != want {
				t.Errorf("results[%d].Slug: want %q, got %q", i, want, got[i].Slug)
			}
			if got[i].Err == "" {
				t.Errorf("results[%d] (%s): want non-empty Err after cancel, got empty", i, want)
			}
		}
		// The not-yet-dispatched orgs (b, c) must specifically carry the
		// context cancellation cause, not some unrelated transport error.
		for _, i := range []int{1, 2} {
			if !strings.Contains(got[i].Err, context.Canceled.Error()) {
				t.Errorf("results[%d].Err = %q, want it to mention %q", i, got[i].Err, context.Canceled.Error())
			}
		}
	})

	t.Run("results returned in input order", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		orgs := []seedfiles.OrgEntry{
			{Org: atlas.Org{Slug: "a", Name: "A", WebsiteURL: srv.URL}},
			{Org: atlas.Org{Slug: "b", Name: "B", WebsiteURL: srv.URL}},
			{Org: atlas.Org{Slug: "c", Name: "C", WebsiteURL: srv.URL}},
		}
		got := Check(context.Background(), orgs, Options{Concurrency: 3})
		for i, want := range []string{"a", "b", "c"} {
			if got[i].Slug != want {
				t.Errorf("results[%d].Slug: want %q, got %q", i, want, got[i].Slug)
			}
		}
	})
}
