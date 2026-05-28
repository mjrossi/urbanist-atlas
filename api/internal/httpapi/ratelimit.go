package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ipRateLimiter is a tiny per-IP sliding-window limiter for the public
// submission endpoint. It's deliberately in-process and simple — the
// goal is "a handful per hour per IP" deterrence, not a hard
// guarantee. Cloudflare's WAF rate-limit rule provides the real
// throttling at the edge; this is the second line of defense the API
// retains when traffic ever bypasses the CDN.
type ipRateLimiter struct {
	mu         sync.Mutex
	hits       map[string][]time.Time
	window     time.Duration
	maxPerWin  int
	nowFunc    func() time.Time
	lastSwept  time.Time
	sweepEvery time.Duration
}

func newIPRateLimiter(maxPerWindow int, window time.Duration) *ipRateLimiter {
	if maxPerWindow <= 0 {
		maxPerWindow = 5
	}
	if window <= 0 {
		window = time.Hour
	}
	return &ipRateLimiter{
		hits:       make(map[string][]time.Time),
		window:     window,
		maxPerWin:  maxPerWindow,
		nowFunc:    func() time.Time { return time.Now() },
		sweepEvery: 5 * time.Minute,
	}
}

// allow reports whether ip is within budget. The boolean is true on
// success; on failure, retryAfter is the seconds until the oldest
// hit ages out of the window (>= 1).
func (l *ipRateLimiter) allow(ip string) (ok bool, retryAfter int) {
	now := l.nowFunc()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Periodic sweep so the map doesn't leak under churning IPs.
	if now.Sub(l.lastSwept) > l.sweepEvery {
		for k, ts := range l.hits {
			kept := keepRecent(ts, cutoff)
			if len(kept) == 0 {
				delete(l.hits, k)
			} else {
				l.hits[k] = kept
			}
		}
		l.lastSwept = now
	}

	recent := keepRecent(l.hits[ip], cutoff)
	if len(recent) >= l.maxPerWin {
		oldest := recent[0]
		retry := max(int(oldest.Add(l.window).Sub(now).Seconds())+1, 1)
		l.hits[ip] = recent
		return false, retry
	}
	l.hits[ip] = append(recent, now)
	return true, 0
}

func keepRecent(ts []time.Time, cutoff time.Time) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			out = append(out, t)
		}
	}
	return out
}

// clientIP picks the most plausible source IP from r. We trust Fly's
// `Fly-Client-IP` if present (Fly sets it on every request from its
// proxy), then a single-value X-Forwarded-For, then RemoteAddr.
// Multi-value XFF is intentionally ignored — picking the wrong entry
// is worse than over-limiting one upstream proxy.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("Fly-Client-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" && !strings.Contains(v, ",") {
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeRateLimited emits a 429 with Retry-After + the rate-limited
// problem type.
func writeRateLimited(w http.ResponseWriter, r *http.Request, retryAfterSec int, rid string) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	writeProblem(w, r, http.StatusTooManyRequests, problemRateLimited,
		"Too Many Requests",
		"submission rate limit exceeded for your source IP; retry later",
		rid)
}
