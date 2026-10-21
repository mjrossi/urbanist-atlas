package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

// ipRateLimiter is an in-process sliding-window limiter for submissions:
// a per-client budget (see rateLimitKey) plus one shared by every
// client. State resets on restart.
type ipRateLimiter struct {
	mu         sync.Mutex
	hits       map[string][]time.Time
	global     []time.Time
	window     time.Duration
	maxPerWin  int
	globalMax  int
	nowFunc    func() time.Time
	lastSwept  time.Time
	sweepEvery time.Duration
}

func newIPRateLimiter(maxPerWindow int, window time.Duration) *ipRateLimiter {
	if maxPerWindow <= 0 {
		maxPerWindow = 5
	}
	if window <= 0 {
		window = rateLimitWindow
	}
	return &ipRateLimiter{
		hits:       make(map[string][]time.Time),
		window:     window,
		maxPerWin:  maxPerWindow,
		globalMax:  submissionsGlobalPerWindow,
		nowFunc:    time.Now,
		sweepEvery: rateLimitSweepInterval,
	}
}

// allow reports whether ip is within both its own budget and the shared
// one. On failure, retryAfter is the seconds until the blocking window's
// oldest hit ages out (>= 1). A rejected request spends neither budget.
func (l *ipRateLimiter) allow(ip string) (ok bool, retryAfter int) {
	now := l.nowFunc()
	cutoff := now.Add(-l.window)
	key := rateLimitKey(ip)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Sweep so the map doesn't grow without bound under churning IPs.
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

	recent := keepRecent(l.hits[key], cutoff)
	if len(recent) >= l.maxPerWin {
		l.hits[key] = recent
		return false, l.retryAfter(recent[0], now)
	}
	l.global = keepRecent(l.global, cutoff)
	if len(l.global) >= l.globalMax {
		if len(recent) == 0 {
			delete(l.hits, key)
		} else {
			l.hits[key] = recent
		}
		return false, l.retryAfter(l.global[0], now)
	}
	l.hits[key] = append(recent, now)
	l.global = append(l.global, now)
	return true, 0
}

func (l *ipRateLimiter) retryAfter(oldest, now time.Time) int {
	return max(int(oldest.Add(l.window).Sub(now).Seconds())+1, 1)
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

// rateLimitKey buckets a client address for the limiter. IPv4 is keyed
// per address. IPv6 is keyed by its /64, because a single subscriber is
// routinely handed a whole /64 and could otherwise rotate addresses
// within it for a fresh budget each time. Unparseable input is kept
// verbatim.
func rateLimitKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	addr = addr.Unmap().WithZone("")
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}

// clientIP picks the source IP the limiter keys on. Trusting
// Fly-Client-IP assumes the app is reachable only through Fly's proxy and
// that the proxy overwrites the header; served any other way, the header
// is client-controlled and this must change. X-Forwarded-For is never
// trusted, or a client could choose its own bucket.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("Fly-Client-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// submissionRateLimit refuses a request with a 429 when limiter.allow
// rejects its source address.
func submissionRateLimit(limiter *ipRateLimiter, m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ok, retry := limiter.allow(clientIP(r)); !ok {
				m.incRateLimit("submissions")
				m.incSubmissions("rate_limited")
				writeRateLimited(w, r, retry, requestIDFromContext(r.Context()))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeRateLimited emits a 429 with Retry-After + the rate-limited
// problem type.
func writeRateLimited(w http.ResponseWriter, r *http.Request, retryAfterSec int, rid string) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSec))
	writeProblem(w, r, http.StatusTooManyRequests, problemRateLimited,
		"Too Many Requests",
		"Too many submissions have been received. Try again after the Retry-After interval.",
		rid)
}

// pendingQueue reports whether the moderation queue holds maxPending or
// more submissions. The answer is cached for pendingQueueCheckInterval
// so a flood of POSTs costs one store query per interval.
type pendingQueue struct {
	subs       atlas.SubmissionStore
	maxPending int
	logger     *slog.Logger
	nowFunc    func() time.Time

	mu        sync.Mutex
	checkedAt time.Time
	isFull    bool
}

func newPendingQueue(subs atlas.SubmissionStore, maxPending int, logger *slog.Logger) *pendingQueue {
	return &pendingQueue{subs: subs, maxPending: maxPending, logger: logger, nowFunc: time.Now}
}

// full fails open on a store error until the first successful check,
// since the create would hit the same store anyway; after that a failed
// refresh keeps the last answer and retries sooner. The query is detached
// from the caller's context so one disconnect can't poison the cache.
func (q *pendingQueue) full(ctx context.Context) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.nowFunc()
	if !q.checkedAt.IsZero() && now.Sub(q.checkedAt) < pendingQueueCheckInterval {
		return q.isFull
	}
	qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pendingQueueQueryTimeout)
	defer cancel()
	items, err := q.subs.List(qctx, atlas.ListSubmissionsQuery{Status: atlas.SubmissionPending, Limit: q.maxPending})
	if err != nil {
		q.logger.ErrorContext(ctx, "pending queue check failed", "err", err)
		q.checkedAt = now.Add(pendingQueueRetryInterval - pendingQueueCheckInterval)
		return q.isFull
	}
	q.isFull = len(items) >= q.maxPending
	q.checkedAt = now
	return q.isFull
}

// pendingQueueGuard refuses new submissions with a 429 while the
// moderation queue is at maxPending, bounding what unreviewed
// submissions can occupy on the shared volume.
func pendingQueueGuard(subs atlas.SubmissionStore, maxPending int, logger *slog.Logger, m *Metrics) func(http.Handler) http.Handler {
	q := newPendingQueue(subs, maxPending, logger)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !q.full(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			rid := requestIDFromContext(r.Context())
			logger.WarnContext(r.Context(), "submission refused: pending queue full", "max_pending", maxPending, "rid", rid)
			m.incRateLimit("submissions")
			m.incSubmissions("rate_limited")
			w.Header().Set("Retry-After", strconv.Itoa(int(rateLimitWindow.Seconds())))
			writeProblem(w, r, http.StatusTooManyRequests, problemRateLimited,
				"Too Many Requests",
				"The moderation queue is full. Try again after the Retry-After interval.",
				rid)
		})
	}
}
