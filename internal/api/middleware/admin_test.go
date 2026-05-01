package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
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

// --- RequirePermission ---

func TestRequirePermission_Allowed(t *testing.T) {
	store := &mockPermStore{}
	handler := RequirePermission(store, "app.delete")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/apps/1", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_owner", "owner@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequirePermission_Missing(t *testing.T) {
	store := &mockPermStore{}
	handler := RequirePermission(store, "app.delete")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/apps/1", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_viewer", "viewer@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestRequirePermission_NoClaims(t *testing.T) {
	store := &mockPermStore{}
	handler := RequirePermission(store, "app.delete")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/apps/1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestRequirePermission_RoleNotFound(t *testing.T) {
	store := &mockPermStore{}
	handler := RequirePermission(store, "app.delete")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/apps/1", nil)
	req = withClaimsCtx(req, "u1", "t1", "role_unknown", "unknown@test.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// mockPermStore implements core.Store just enough for RequirePermission tests.
type mockPermStore struct{}

func (m *mockPermStore) GetRole(_ context.Context, roleID string) (*core.Role, error) {
	var perms string
	switch roleID {
	case "role_owner":
		perms = `["app.*","domain.*"]`
	case "role_viewer":
		perms = `["app.view"]`
	default:
		return nil, core.ErrNotFound
	}
	return &core.Role{ID: roleID, PermissionsJSON: perms}, nil
}

// Stub implementations for core.Store
func (m *mockPermStore) Close() error                                            { return nil }
func (m *mockPermStore) Ping(_ context.Context) error                            { return nil }
func (m *mockPermStore) CreateUser(_ context.Context, _ *core.User) error        { return nil }
func (m *mockPermStore) GetUser(_ context.Context, _ string) (*core.User, error) { return nil, nil }
func (m *mockPermStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateUser(_ context.Context, _ *core.User) error            { return nil }
func (m *mockPermStore) UpdatePassword(_ context.Context, _, _ string) error         { return nil }
func (m *mockPermStore) UpdateLastLogin(_ context.Context, _ string) error           { return nil }
func (m *mockPermStore) CountUsers(_ context.Context) (int, error)                   { return 0, nil }
func (m *mockPermStore) CreateTenant(_ context.Context, _ *core.Tenant) error        { return nil }
func (m *mockPermStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) { return nil, nil }
func (m *mockPermStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateTenant(_ context.Context, _ *core.Tenant) error   { return nil }
func (m *mockPermStore) DeleteTenant(_ context.Context, _ string) error         { return nil }
func (m *mockPermStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *mockPermStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *mockPermStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (m *mockPermStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateAppStatus(_ context.Context, _, _ string) error         { return nil }
func (m *mockPermStore) DeleteApp(_ context.Context, _ string) error                  { return nil }
func (m *mockPermStore) CreateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (m *mockPermStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}
func (m *mockPermStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (m *mockPermStore) ListDeploymentsByStatus(_ context.Context, _ string) ([]core.Deployment, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateDeployment(_ context.Context, _ *core.Deployment) error  { return nil }
func (m *mockPermStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockPermStore) AtomicNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *mockPermStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (m *mockPermStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, nil
}
func (m *mockPermStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (m *mockPermStore) DeleteDomain(_ context.Context, _ string) error          { return nil }
func (m *mockPermStore) ListAllDomains(_ context.Context) ([]core.Domain, error) { return nil, nil }
func (m *mockPermStore) CreateProject(_ context.Context, _ *core.Project) error  { return nil }
func (m *mockPermStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, nil
}
func (m *mockPermStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (m *mockPermStore) DeleteProject(_ context.Context, _ string) error { return nil }
func (m *mockPermStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *mockPermStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockPermStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, nil
}
func (m *mockPermStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) { return nil, nil }
func (m *mockPermStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (m *mockPermStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockPermStore) CreateSecret(_ context.Context, _ *core.Secret) error { return nil }
func (m *mockPermStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}
func (m *mockPermStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (m *mockPermStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateSecretVersionValue(_ context.Context, _, _ string) error { return nil }
func (m *mockPermStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, nil
}
func (m *mockPermStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, nil
}
func (m *mockPermStore) CreateInvite(_ context.Context, _ *core.Invitation) error { return nil }
func (m *mockPermStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (m *mockPermStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (m *mockPermStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) {
	return nil, 0, nil
}
func (m *mockPermStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *mockPermStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (m *mockPermStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error { return nil }
func (m *mockPermStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}
func (m *mockPermStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockPermStore) GetDomain(_ context.Context, _ string) (*core.Domain, error) { return nil, nil }
func (m *mockPermStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, nil
}
func (m *mockPermStore) UpdateTOTPEnabled(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

// --- RequireOwnerOrAbove ---

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
