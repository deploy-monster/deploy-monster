package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// quotaMockStore implements core.Store with controllable app counts for quota tests.
type quotaMockStore struct {
	auditMockStore     // embed to satisfy the full interface
	appCount       int // total apps returned by ListAppsByTenant
}

func (s *quotaMockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, s.appCount, nil
}

func TestQuotaEnforcement_AllowsUnderLimit(t *testing.T) {
	store := &quotaMockStore{appCount: 5}
	handler := QuotaEnforcement(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	claims := &auth.Claims{UserID: "user-1", TenantID: "tenant-1", RoleID: "role_admin", Email: "a@b.com"}
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201 under quota, got %d", rr.Code)
	}
}

func TestQuotaEnforcement_RejectsAtLimit(t *testing.T) {
	store := &quotaMockStore{appCount: 100}
	handler := QuotaEnforcement(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called when quota exceeded")
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	claims := &auth.Claims{UserID: "user-1", TenantID: "tenant-1", RoleID: "role_admin", Email: "a@b.com"}
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 at quota limit, got %d", rr.Code)
	}
}

func TestQuotaEnforcement_SkipsGET(t *testing.T) {
	store := &quotaMockStore{appCount: 999}
	handlerCalled := false
	handler := QuotaEnforcement(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("handler should be called for GET")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for GET, got %d", rr.Code)
	}
}

func TestQuotaEnforcement_SkipsNonQuotaPath(t *testing.T) {
	store := &quotaMockStore{appCount: 999}
	handlerCalled := false
	handler := QuotaEnforcement(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings", nil)
	claims := &auth.Claims{UserID: "user-1", TenantID: "tenant-1", RoleID: "role_admin", Email: "a@b.com"}
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("handler should be called for non-quota path")
	}
}

func TestQuotaEnforcement_SkipsNoClaims(t *testing.T) {
	store := &quotaMockStore{appCount: 999}
	handlerCalled := false
	handler := QuotaEnforcement(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	// POST to quota path but no claims in context
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("handler should be called when no claims present")
	}
}

func TestIsQuotaPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/api/v1/apps", true},
		{"/api/v1/databases", true},
		{"/api/v1/domains", true},
		{"/api/v1/settings", false},
		{"/api/v1/users", false},
		{"/health", false},
		{"/api/v1/apps/abc/restart", false},
	}

	for _, tt := range tests {
		if got := isQuotaPath(tt.path); got != tt.want {
			t.Errorf("isQuotaPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
