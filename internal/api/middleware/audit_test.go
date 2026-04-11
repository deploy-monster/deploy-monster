package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// auditMockStore is a minimal core.Store implementation for audit middleware tests.
// Only CreateAuditLog and ListAuditLogs are actually exercised; the rest
// are no-op stubs satisfying the interface.
type auditMockStore struct {
	mu       sync.Mutex
	entries  []*core.AuditEntry
	errAudit error
}

func (s *auditMockStore) CreateAuditLog(_ context.Context, entry *core.AuditEntry) error {
	if s.errAudit != nil {
		return s.errAudit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

func (s *auditMockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}

// ── Stubs for the rest of core.Store ────────────────────────────────────────

func (s *auditMockStore) CreateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (s *auditMockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (s *auditMockStore) DeleteTenant(_ context.Context, _ string) error       { return nil }

func (s *auditMockStore) CreateUser(_ context.Context, _ *core.User) error { return nil }
func (s *auditMockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) UpdateUser(_ context.Context, _ *core.User) error    { return nil }
func (s *auditMockStore) UpdatePassword(_ context.Context, _, _ string) error { return nil }
func (s *auditMockStore) UpdateLastLogin(_ context.Context, _ string) error   { return nil }
func (s *auditMockStore) CountUsers(_ context.Context) (int, error)           { return 0, nil }
func (s *auditMockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}

func (s *auditMockStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (s *auditMockStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (s *auditMockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (s *auditMockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (s *auditMockStore) UpdateAppStatus(_ context.Context, _, _ string) error { return nil }
func (s *auditMockStore) DeleteApp(_ context.Context, _ string) error          { return nil }
func (s *auditMockStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
}

func (s *auditMockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (s *auditMockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (s *auditMockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}

func (s *auditMockStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (s *auditMockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (s *auditMockStore) DeleteDomain(_ context.Context, _ string) error              { return nil }
func (s *auditMockStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) { return 0, nil }
func (s *auditMockStore) ListAllDomains(_ context.Context) ([]core.Domain, error)     { return nil, nil }

func (s *auditMockStore) CreateProject(_ context.Context, _ *core.Project) error { return nil }
func (s *auditMockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (s *auditMockStore) DeleteProject(_ context.Context, _ string) error { return nil }
func (s *auditMockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s *auditMockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}

func (s *auditMockStore) CreateSecret(_ context.Context, secret *core.Secret) error {
	if secret.ID == "" {
		secret.ID = core.GenerateID()
	}
	return nil
}
func (s *auditMockStore) CreateSecretVersion(_ context.Context, version *core.SecretVersion) error {
	if version.ID == "" {
		version.ID = core.GenerateID()
	}
	return nil
}
func (s *auditMockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (s *auditMockStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return nil, nil
}
func (s *auditMockStore) UpdateSecretVersionValue(_ context.Context, _, _ string) error {
	return nil
}
func (s *auditMockStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, core.ErrNotFound
}
func (s *auditMockStore) CreateInvite(_ context.Context, invite *core.Invitation) error {
	if invite.ID == "" {
		invite.ID = core.GenerateID()
	}
	return nil
}
func (s *auditMockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (s *auditMockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}

func (s *auditMockStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (s *auditMockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) {
	return nil, 0, nil
}
func (s *auditMockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (s *auditMockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (s *auditMockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
	return nil
}

func (s *auditMockStore) Close() error                 { return nil }
func (s *auditMockStore) Ping(_ context.Context) error { return nil }

// ── Helper ──────────────────────────────────────────────────────────────────

// withTestClaims returns a request with auth.Claims injected into context.
func withTestClaims(r *http.Request, userID, tenantID string) *http.Request {
	claims := &auth.Claims{
		UserID:   userID,
		TenantID: tenantID,
		RoleID:   "role_admin",
		Email:    "admin@test.com",
	}
	ctx := auth.ContextWithClaims(r.Context(), claims)
	return r.WithContext(ctx)
}

// ── Tests ───────────────────────────────────────────────────────────────────

func TestAuditLog_LogsPOST(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req = withTestClaims(req, "user-1", "tenant-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(store.entries))
	}
	entry := store.entries[0]
	if entry.Action != "create" {
		t.Errorf("expected action 'create', got %q", entry.Action)
	}
	if entry.ResourceType != "apps" {
		t.Errorf("expected resource 'apps', got %q", entry.ResourceType)
	}
	if entry.UserID != "user-1" {
		t.Errorf("expected user-1, got %q", entry.UserID)
	}
	if entry.TenantID != "tenant-1" {
		t.Errorf("expected tenant-1, got %q", entry.TenantID)
	}
}

func TestAuditLog_LogsDELETE(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/abc123", nil)
	req = withTestClaims(req, "user-2", "tenant-2")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(store.entries))
	}
	entry := store.entries[0]
	if entry.Action != "delete" {
		t.Errorf("expected action 'delete', got %q", entry.Action)
	}
	if entry.ResourceID != "abc123" {
		t.Errorf("expected resource ID 'abc123', got %q", entry.ResourceID)
	}
}

func TestAuditLog_LogsPUT(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/v1/apps/xyz789", nil)
	req = withTestClaims(req, "user-3", "tenant-3")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(store.entries))
	}
	if store.entries[0].Action != "update" {
		t.Errorf("expected action 'update', got %q", store.entries[0].Action)
	}
}

func TestAuditLog_SkipsGET(t *testing.T) {
	store := &auditMockStore{}
	handlerCalled := false
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req = withTestClaims(req, "user-1", "tenant-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("handler should be called for GET")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 0 {
		t.Errorf("GET should not create audit entry, got %d", len(store.entries))
	}
}

func TestAuditLog_SkipsOPTIONS(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 0 {
		t.Errorf("OPTIONS should not create audit entry, got %d", len(store.entries))
	}
}

func TestAuditLog_SkipsHEAD(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 0 {
		t.Errorf("HEAD should not create audit entry, got %d", len(store.entries))
	}
}

func TestAuditLog_SkipsErrorResponses(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req = withTestClaims(req, "user-1", "tenant-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 0 {
		t.Errorf("error responses should not create audit entry, got %d", len(store.entries))
	}
}

func TestAuditLog_SkipsNoClaims(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No claims in context
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 0 {
		t.Errorf("missing claims should not create audit entry, got %d", len(store.entries))
	}
}

func TestAuditLog_StoreError(t *testing.T) {
	store := &auditMockStore{errAudit: core.ErrNotFound}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req = withTestClaims(req, "user-1", "tenant-1")
	rr := httptest.NewRecorder()

	// Should not panic even if store returns error
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 despite store error, got %d", rr.Code)
	}
}

func TestAuditLog_CapturesRealIP(t *testing.T) {
	store := &auditMockStore{}
	handler := AuditLog(store, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")
	req = withTestClaims(req, "user-1", "tenant-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(store.entries))
	}
	if store.entries[0].IPAddress != "10.0.0.1" {
		t.Errorf("expected IP 10.0.0.1, got %q", store.entries[0].IPAddress)
	}
}
