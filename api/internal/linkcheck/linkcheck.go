// Package linkcheck probes the website_url of each seed org and
// reports timeouts, transport errors, and non-2xx responses so the
// editorial pass can catch link rot before it ships.
package linkcheck

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/internal/seedfiles"
)

// Result is the per-org outcome of a single Check pass.
type Result struct {
	Slug      string
	Name      string
	URL       string
	Status    int
	FinalURL  string
	Err       string
	ElapsedMs int64
}

// Options tunes Check. Zero values fall back to documented defaults.
type Options struct {
	Timeout     time.Duration
	Concurrency int
}

const (
	defaultTimeout     = 15 * time.Second
	defaultConcurrency = 8
	userAgent          = "urbanist-atlas-linkcheck/0.1 (+https://urbanistatlas.com)"
)

// sharedTransport is reused across every probe so connections (and TLS
// sessions) are pooled instead of rebuilt per request (issue #32). A
// fresh http.Client per do() call is fine — the client is a thin handle
// — as long as they all share this one Transport, which owns the
// connection pool. Cloned from DefaultTransport so we get the stdlib
// proxy/dialer defaults, then sized for a small concurrent crawl.
var sharedTransport = func() *http.Transport {
	// DefaultTransport is always *http.Transport in the stdlib; the
	// comma-ok guards the assertion so a non-default runtime override
	// degrades to a plain transport instead of panicking.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	t := base.Clone()
	// Probes hit many distinct hosts; keep a modest idle pool so
	// back-to-back checks against the same host reuse a connection
	// without holding sockets open indefinitely.
	t.MaxIdleConns = 64
	t.MaxIdleConnsPerHost = 4
	t.IdleConnTimeout = 30 * time.Second
	return t
}()

// Check probes each org's website_url and returns results in input
// order so the report diffs cleanly against the source TOML.
func Check(ctx context.Context, orgs []seedfiles.OrgEntry, opts Options) []Result {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	results := make([]Result, len(orgs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, o := range orgs {
		// Acquire a slot, but bail out promptly if the caller cancels
		// while every worker is busy (issue #31): without the ctx arm,
		// a full semaphore would block the dispatch loop indefinitely
		// past cancellation. On cancel, mark this and every remaining
		// org with the cancellation cause so each input still has a
		// non-misleading result row (input-order contract preserved),
		// then stop dispatching.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			for j := i; j < len(orgs); j++ {
				results[j] = Result{
					Slug: orgs[j].Slug,
					Name: orgs[j].Name,
					URL:  orgs[j].WebsiteURL,
					Err:  ctx.Err().Error(),
				}
			}
			wg.Wait()
			return results
		}
		wg.Add(1)
		go func(i int, o seedfiles.OrgEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = probe(ctx, o, timeout)
		}(i, o)
	}
	wg.Wait()
	return results
}

func probe(ctx context.Context, o seedfiles.OrgEntry, timeout time.Duration) Result {
	r := Result{Slug: o.Slug, Name: o.Name, URL: o.WebsiteURL}
	start := time.Now()
	defer func() { r.ElapsedMs = time.Since(start).Milliseconds() }()

	status, finalURL, err := do(ctx, http.MethodHead, o.WebsiteURL, timeout)
	if err == nil && (status == http.StatusMethodNotAllowed || status == http.StatusForbidden) {
		status, finalURL, err = do(ctx, http.MethodGet, o.WebsiteURL, timeout)
	}
	r.Status = status
	r.FinalURL = finalURL
	if err != nil {
		r.Err = err.Error()
		return r
	}
	if status < 200 || status >= 300 {
		r.Err = fmt.Sprintf("HTTP %d", status)
	}
	return r
}

func do(ctx context.Context, method, url string, timeout time.Duration) (int, string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", userAgent)

	// finalURL is captured progressively during redirects so it stays
	// populated even if Do() ultimately errors mid-chain. The successful
	// path overwrites it with resp.Request.URL below — that value is the
	// authoritative post-redirect URL (or the original when no redirects
	// happened), and was previously left empty for direct 200 hits.
	var finalURL string
	// A per-call client is still needed because CheckRedirect closes over
	// this call's finalURL, but it reuses the package-level sharedTransport
	// so the connection pool is not rebuilt each call (issue #32).
	client := &http.Client{
		Transport: sharedTransport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			finalURL = req.URL.String()
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, finalURL, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	finalURL = resp.Request.URL.String()
	return resp.StatusCode, finalURL, nil
}
