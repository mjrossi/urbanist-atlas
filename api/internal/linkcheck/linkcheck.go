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

// sharedTransport pools connections and TLS sessions across every probe.
// Each do() builds its own cheap http.Client, but they all share this
// Transport, which owns the pool.
var sharedTransport = func() *http.Transport {
	// The comma-ok keeps a replaced DefaultTransport from panicking.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	t := base.Clone()
	// Many distinct hosts: a modest idle pool still reuses a connection
	// for back-to-back checks of one host.
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
		// Bail out on cancel even when every worker is busy, marking this and
		// every remaining org with the cause so each input keeps a result row.
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

	// CheckRedirect records each hop, so finalURL survives an error
	// mid-chain.
	var finalURL string
	// A per-call client, because CheckRedirect closes over finalURL.
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
