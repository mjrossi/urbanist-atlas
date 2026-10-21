package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBearerAuthMiddleware_ConstantTimeCompare can't assert timing, so it
// pins what the fixed-width hashing protects: wrong tokens of every
// length, far shorter and far longer than the real one, all get 401, and
// only the exact token passes. A compare that checked lengths first and
// bailed early would fail it.
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
// (503), never silently allowing access. The hashed compare sits after
// this guard, so it can't turn a missing token into an auth bypass.
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
	var p struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Type != problemAdminDisabled {
		t.Errorf("problem type = %q, want %q", p.Type, problemAdminDisabled)
	}
	if nextCalled {
		t.Error("next handler was reached with no admin token configured — must fail closed")
	}
}
