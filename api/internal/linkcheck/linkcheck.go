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
		wg.Add(1)
		sem <- struct{}{}
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
	client := &http.Client{
		Timeout: timeout,
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
