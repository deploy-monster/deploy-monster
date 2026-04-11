package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

func withClaimsCtx(r *http.Request, userID, tenantID, roleID, email string) *http.Request {
	claims := &auth.Claims{
		UserID:   userID,
		TenantID: tenantID,
		RoleID:   roleID,
		Email:    email,
	}
	return r.WithContext(auth.ContextWithClaims(r.Context(), claims))
}

func TestRequireAdmin_SuperAdmin(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for super_admin, got %d", rr.Code)
	}
}

func TestRequireAdmin_Admin(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", rr.Code)
	}
}

func TestRequireAdmin_Developer_Rejected(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_developer", "dev@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for developer, got %d", rr.Code)
	}
}

func TestRequireAdmin_NoClaims(t *testing.T) {
	handler := RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no claims, got %d", rr.Code)
	}
}

func TestRequireSuperAdmin_SuperAdmin(t *testing.T) {
	handler := RequireSuperAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_super_admin", "sa@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for super_admin, got %d", rr.Code)
	}
}

func TestRequireSuperAdmin_Admin_Rejected(t *testing.T) {
	handler := RequireSuperAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for admin on super-admin route, got %d", rr.Code)
	}
}

func TestRequireSuperAdmin_Viewer_Rejected(t *testing.T) {
	handler := RequireSuperAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_viewer", "viewer@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for viewer, got %d", rr.Code)
	}
}

func TestRequireSuperAdmin_NoClaims(t *testing.T) {
	handler := RequireSuperAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- RequireOwnerOrAbove ---

func TestRequireOwnerOrAbove_SuperAdmin(t *testing.T) {
	handler := RequireOwnerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/servers/provision", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_super_admin", "sa@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for super_admin, got %d", rr.Code)
	}
}

func TestRequireOwnerOrAbove_Owner(t *testing.T) {
	handler := RequireOwnerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/servers/provision", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_owner", "owner@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for owner, got %d", rr.Code)
	}
}

func TestRequireOwnerOrAbove_Admin(t *testing.T) {
	handler := RequireOwnerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/servers/provision", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", rr.Code)
	}
}

func TestRequireOwnerOrAbove_Developer_Rejected(t *testing.T) {
	handler := RequireOwnerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/servers/provision", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_developer", "dev@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for developer, got %d", rr.Code)
	}
}

func TestRequireOwnerOrAbove_Viewer_Rejected(t *testing.T) {
	handler := RequireOwnerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/servers/provision", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_viewer", "viewer@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for viewer, got %d", rr.Code)
	}
}

func TestRequireOwnerOrAbove_NoClaims(t *testing.T) {
	handler := RequireOwnerOrAbove(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/servers/provision", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequireSuperAdmin_ErrorResponse_JSON(t *testing.T) {
	handler := RequireSuperAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_developer", "dev@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("expected JSON response: %v", err)
	}
	if resp["success"] != false {
		t.Errorf("expected success=false, got %v", resp["success"])
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error object in response")
	}
	if errObj["code"] != "forbidden" {
		t.Errorf("expected code=forbidden, got %v", errObj["code"])
	}
}
