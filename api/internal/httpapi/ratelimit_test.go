package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/mjrossi/urbanist-atlas/api/pkg/atlas"
)

func TestRateLimitKey(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{name: "ipv4 per address", in: "203.0.113.7", want: "203.0.113.7"},
		{name: "ipv4-mapped ipv6 is ipv4", in: "::ffff:203.0.113.7", want: "203.0.113.7"},
		{name: "ipv6 masked to /64", in: "2001:db8:1:2:aaaa:bbbb:cccc:dddd", want: "2001:db8:1:2::/64"},
		{name: "ipv6 zone dropped", in: "fe80::1%eth0", want: "fe80::/64"},
		{name: "unparseable kept verbatim", in: "not-an-ip", want: "not-an-ip"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rateLimitKey(tc.in); got != tc.want {
				t.Errorf("rateLimitKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// One IPv6 /64 is routinely a single subscriber, so rotating the
// interface identifier must not buy a fresh budget.
func TestIPRateLimiter_IPv6Slash64SharesBudget(t *testing.T) {
	l := newIPRateLimiter(2, time.Hour)
	for _, ip := range []string{"2001:db8:1:2::1", "2001:db8:1:2::2"} {
		if ok, _ := l.allow(ip); !ok {
			t.Fatalf("allow(%s) = false, want true (within budget)", ip)
		}
	}
	if ok, _ := l.allow("2001:db8:1:2:ffff::3"); ok {
		t.Error("third address in the same /64 allowed; want it to share the exhausted budget")
	}
	if ok, _ := l.allow("2001:db8:1:3::1"); !ok {
		t.Error("address in a different /64 denied; want its own budget")
	}
	// IPv4 stays per-address.
	for _, ip := range []string{"203.0.113.1", "203.0.113.1", "203.0.113.2"} {
		if ok, _ := l.allow(ip); !ok {
			t.Errorf("allow(%s) = false, want IPv4 budgets per address", ip)
		}
	}
}

func TestIPRateLimiter_GlobalCap(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newIPRateLimiter(5, time.Hour)
	l.nowFunc = func() time.Time { return now }
	l.globalMax = 3

	for i := range 3 {
		ip := "203.0.113." + strconv.Itoa(i+1)
		if ok, _ := l.allow(ip); !ok {
			t.Fatalf("allow(%s) = false, want true (under global cap)", ip)
		}
		now = now.Add(time.Minute)
	}
	ok, retry := l.allow("198.51.100.1")
	if ok {
		t.Fatal("fresh IP allowed past the global cap")
	}
	// The first hit was at t0 and it is now t0+3m, so it ages out in 57m.
	if want := int((57 * time.Minute).Seconds()) + 1; retry != want {
		t.Errorf("retryAfter = %d, want %d", retry, want)
	}

	now = now.Add(57*time.Minute + time.Second)
	if ok, _ := l.allow("198.51.100.1"); !ok {
		t.Error("allow after the oldest global hit aged out = false, want true")
	}
}

// Requests the global cap rejects from new addresses must not grow the
// per-client map, or a flood of fresh addresses during a full window
// would hold memory until the next sweep.
func TestIPRateLimiter_GlobalDenialStoresNoEntry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l := newIPRateLimiter(5, time.Hour)
	l.nowFunc = func() time.Time { return now }
	l.globalMax = 2

	for _, ip := range []string{"203.0.113.1", "203.0.113.2"} {
		if ok, _ := l.allow(ip); !ok {
			t.Fatalf("allow(%s) = false, want true (under global cap)", ip)
		}
	}
	before := len(l.hits)
	for i := range 50 {
		ip := "198.51.100." + strconv.Itoa(i+1)
		if ok, _ := l.allow(ip); ok {
			t.Fatalf("allow(%s) = true past the global cap", ip)
		}
	}
	if got := len(l.hits); got != before {
		t.Errorf("hit map holds %d keys after global denials, want %d", got, before)
	}
}

// A request the per-IP limit rejects must not also spend the shared
// global budget, or one noisy client could exhaust it for everyone.
func TestIPRateLimiter_PerIPDenialSparesGlobalBudget(t *testing.T) {
	l := newIPRateLimiter(1, time.Hour)
	l.globalMax = 2
	if ok, _ := l.allow("203.0.113.1"); !ok {
		t.Fatal("first request denied")
	}
	for range 5 {
		if ok, _ := l.allow("203.0.113.1"); ok {
			t.Fatal("per-IP limit not enforced")
		}
	}
	if ok, _ := l.allow("203.0.113.2"); !ok {
		t.Error("second IP denied; per-IP rejections consumed the global budget")
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{name: "Fly-Client-IP", headers: map[string]string{"Fly-Client-IP": " 203.0.113.9 "}, want: "203.0.113.9"},
		// Nothing in front of a non-Fly deployment rewrites X-Forwarded-For,
		// so trusting it would let any client pick its own bucket.
		{name: "X-Forwarded-For ignored", headers: map[string]string{"X-Forwarded-For": "198.51.100.1"}, want: "192.0.2.1"},
		{name: "RemoteAddr", want: "192.0.2.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.RemoteAddr = "192.0.2.1:4321"
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if got := clientIP(req); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

// pendingListStore answers List with `pending` rows (capped at the
// query limit, like the real store) and counts the calls.
type pendingListStore struct {
	atlas.SubmissionStore
	mu      sync.Mutex
	pending int
	err     error
	calls   int
}

func (s *pendingListStore) List(ctx context.Context, q atlas.ListSubmissionsQuery) ([]atlas.Submission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.err != nil {
		return nil, s.err
	}
	if q.Status != atlas.SubmissionPending {
		return nil, errors.New("unexpected status filter " + string(q.Status))
	}
	return make([]atlas.Submission, min(s.pending, q.Limit)), nil
}

func TestPendingQueueGuard(t *testing.T) {
	const maxPending = 3
	cases := []struct {
		name     string
		pending  int
		err      error
		wantNext bool
	}{
		{name: "below cap", pending: maxPending - 1, wantNext: true},
		{name: "at cap", pending: maxPending, wantNext: false},
		{name: "store error fails open", err: errors.New("db down"), wantNext: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &pendingListStore{pending: tc.pending, err: tc.err}
			m := NewMetrics()
			mw := pendingQueueGuard(store, maxPending, slog.New(slog.DiscardHandler), m)
			nextCalls := 0
			h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalls++
				w.WriteHeader(http.StatusCreated)
			}))

			for range 3 {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/submissions", nil))
				if tc.wantNext {
					if rec.Code != http.StatusCreated {
						t.Fatalf("status = %d, want 201", rec.Code)
					}
					continue
				}
				if rec.Code != http.StatusTooManyRequests {
					t.Fatalf("status = %d, want 429", rec.Code)
				}
				if rec.Header().Get("Retry-After") == "" {
					t.Error("429 without Retry-After")
				}
			}
			if gotNext := nextCalls > 0; gotNext != tc.wantNext {
				t.Errorf("next reached = %v, want %v", gotNext, tc.wantNext)
			}
			// The check is cached, so a flood of POSTs costs one query
			// per interval rather than one per request.
			if store.calls != 1 {
				t.Errorf("store List calls = %d, want 1 (cached between requests)", store.calls)
			}
		})
	}
}

func TestNew_SubmissionsRefusedWhenQueueFull(t *testing.T) {
	store := atlas.NewMemStore()
	atlas.LoadDevFixtures(store)
	srv := httptest.NewServer(New(Config{
		Store:       store,
		Logger:      slog.New(slog.DiscardHandler),
		Submissions: &pendingListStore{pending: maxPendingSubmissions},
	}))
	t.Cleanup(srv.Close)

	resp := postJSON(t, srv.URL+"/api/v1/submissions", goodSubmissionBody(), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 with a full moderation queue", resp.StatusCode)
	}
}

func TestPendingQueueGuard_RechecksAfterInterval(t *testing.T) {
	store := &pendingListStore{pending: 5}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := newPendingQueue(store, 5, slog.New(slog.DiscardHandler))
	g.nowFunc = func() time.Time { return now }

	if !g.full(context.Background()) {
		t.Fatal("full = false at cap")
	}
	store.mu.Lock()
	store.pending = 0
	store.mu.Unlock()
	if !g.full(context.Background()) {
		t.Error("full re-queried inside the cache interval")
	}
	now = now.Add(pendingQueueCheckInterval)
	if g.full(context.Background()) {
		t.Error("full = true after the interval and the queue drained")
	}
}

func TestPendingQueue_RefreshErrorKeepsLastAnswer(t *testing.T) {
	store := &pendingListStore{pending: 5}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	g := newPendingQueue(store, 5, slog.New(slog.DiscardHandler))
	g.nowFunc = func() time.Time { return now }

	if !g.full(context.Background()) {
		t.Fatal("full = false at cap")
	}
	store.mu.Lock()
	store.err = errors.New("db down")
	store.mu.Unlock()
	now = now.Add(pendingQueueCheckInterval)
	if !g.full(context.Background()) {
		t.Error("a failed refresh reset a full queue to not-full")
	}
	calls := store.calls
	if !g.full(context.Background()) || store.calls != calls {
		t.Error("failed refresh was not held off for the retry interval")
	}
	now = now.Add(pendingQueueRetryInterval)
	g.full(context.Background())
	if store.calls != calls+1 {
		t.Errorf("store List calls = %d, want a retry after the retry interval", store.calls-calls)
	}
}

func TestPendingQueue_CanceledCallerDoesNotPoisonCache(t *testing.T) {
	store := &pendingListStore{pending: 5}
	g := newPendingQueue(store, 5, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !g.full(ctx) {
		t.Error("a canceled caller context made the guard report not-full")
	}
}
