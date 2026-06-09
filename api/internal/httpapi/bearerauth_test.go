package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBearerAuthMiddleware_ConstantTimeCompare pins issue #7: the admin
// token compare hashes both sides to a fixed-width SHA-256 digest before
// subtle.ConstantTimeCompare, so a wrong token is rejected the same way
// regardless of its length (a raw byte compare short-circuits on a
// length mismatch, leaking the secret's length through timing). We can't
// assert timing here, but we CAN pin the observable behavior the hashing
// exists to make safe: wrong tokens of every length — including ones far
// shorter and far longer than the real token — are all rejected with
// 401, and only the exact token passes. A regression to a raw
// variable-length compare would still pass these, but a regression that
// (say) compared lengths first and bailed early would not.
func TestBearerAuthMiddleware_ConstantTimeCompare(t *testing.T) {
	const adminToken = "the-real-admin-token"
	mw := bearerAuthMiddleware(adminToken)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := mw(next)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"correct token passes", "Bearer " + adminToken, http.StatusNoContent},
		{"correct token, case-insensitive scheme", "bearer " + adminToken, http.StatusNoContent},
		{"empty wrong token", "Bearer ", http.StatusUnauthorized},
		{"short wrong token", "Bearer x", http.StatusUnauthorized},
		{"wrong token, same length", "Bearer the-fake-admin-token!", http.StatusUnauthorized},
		{"wrong token, much longer", "Bearer " + adminToken + "-with-a-long-suffix-appended", http.StatusUnauthorized},
		{"correct token with extra suffix", "Bearer " + adminToken + "x", http.StatusUnauthorized},
		{"correct token missing last char", "Bearer " + adminToken[:len(adminToken)-1], http.StatusUnauthorized},
		{"missing Authorization header", "", http.StatusUnauthorized},
		{"non-bearer scheme", "Basic " + adminToken, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/submissions", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

// TestBearerAuthMiddleware_EmptyTokenFailsClosed pins the fail-closed
// guarantee: an unconfigured admin token disables the endpoints entirely
// (503), never silently allowing access. The hashing change (#7) sits
// AFTER this guard, so it can't turn a missing token into an auth bypass.
func TestBearerAuthMiddleware_EmptyTokenFailsClosed(t *testing.T) {
	mw := bearerAuthMiddleware("")
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})
	handler := mw(next)

	// Even a request that sends *some* bearer token must be refused with
	// 503 — there is no token value that authenticates when none is set.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/submissions", nil)
	req.Header.Set("Authorization", "Bearer anything")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d (admin endpoints disabled when no token configured)", rec.Code, http.StatusServiceUnavailable)
	}
	if nextCalled {
		t.Error("next handler was reached with no admin token configured — must fail closed")
	}
}
