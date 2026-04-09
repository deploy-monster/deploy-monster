package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

func TestTenantRateLimiter_AllowsWithinLimit(t *testing.T) {
	store := newRLBoltStore()
	trl := NewTenantRateLimiter(store, 5, time.Minute)

	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "t1", UserID: "u1"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req.WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
}

func TestTenantRateLimiter_BlocksOverLimit(t *testing.T) {
	store := newRLBoltStore()
	trl := NewTenantRateLimiter(store, 3, time.Minute)

	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &auth.Claims{TenantID: "t2", UserID: "u1"}

	// Use up the limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := auth.ContextWithClaims(req.Context(), claims)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req.WithContext(ctx))
	}

	// 4th should be rejected
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.ContextWithClaims(req.Context(), claims)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header should be set")
	}
}

func TestTenantRateLimiter_DifferentTenants_Independent(t *testing.T) {
	store := newRLBoltStore()
	trl := NewTenantRateLimiter(store, 1, time.Minute)

	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Tenant A uses its limit
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx1 := auth.ContextWithClaims(req1.Context(), &auth.Claims{TenantID: "tA", UserID: "u1"})
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1.WithContext(ctx1))
	if rec1.Code != http.StatusOK {
		t.Errorf("tenant A: expected 200, got %d", rec1.Code)
	}

	// Tenant B should not be affected
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx2 := auth.ContextWithClaims(req2.Context(), &auth.Claims{TenantID: "tB", UserID: "u2"})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2.WithContext(ctx2))
	if rec2.Code != http.StatusOK {
		t.Errorf("tenant B: expected 200, got %d", rec2.Code)
	}
}

func TestTenantRateLimiter_RespectsPerTenantConfig(t *testing.T) {
	store := newRLBoltStore()
	// Set a custom limit for tenant "custom" — 2 req/min instead of default 100
	store.Set("tenant_ratelimit", "custom", tenantRateLimitConfig{
		RequestsPerMinute: 2,
		BurstSize:         5,
	}, 0)

	trl := NewTenantRateLimiter(store, 100, time.Minute)

	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	claims := &auth.Claims{TenantID: "custom", UserID: "u1"}

	// First 2 should pass
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		ctx := auth.ContextWithClaims(req.Context(), claims)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req.WithContext(ctx))
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// 3rd should be rate-limited
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.ContextWithClaims(req.Context(), claims)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

func TestTenantRateLimiter_NoClaims_PassesThrough(t *testing.T) {
	store := newRLBoltStore()
	trl := NewTenantRateLimiter(store, 1, time.Minute)

	called := false
	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called without claims")
	}
}

func TestTenantRateLimiter_NilBolt_PassesThrough(t *testing.T) {
	trl := NewTenantRateLimiter(nil, 1, time.Minute)

	called := false
	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "t1", UserID: "u1"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))

	if !called {
		t.Error("handler should pass through when bolt is nil")
	}
}
