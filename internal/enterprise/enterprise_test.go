package enterprise

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Mock Store
// ---------------------------------------------------------------------------

type mockStore struct {
	users      map[string]*core.User
	members    map[string]*core.TeamMember
	apps       []core.Application
	appCount   int
	auditLogs  []core.AuditEntry
	auditCount int
	getUserErr error
	updateErr  error
}

func newMockStore() *mockStore {
	return &mockStore{
		users:   make(map[string]*core.User),
		members: make(map[string]*core.TeamMember),
	}
}

func (s *mockStore) CreateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (s *mockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}
func (s *mockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}
func (s *mockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (s *mockStore) DeleteTenant(_ context.Context, _ string) error       { return nil }

func (s *mockStore) CreateUser(_ context.Context, user *core.User) error {
	s.users[user.ID] = user
	return nil
}
func (s *mockStore) GetUser(_ context.Context, id string) (*core.User, error) {
	if s.getUserErr != nil {
		return nil, s.getUserErr
	}
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found: %s", id)
	}
	return u, nil
}
func (s *mockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, nil
}
func (s *mockStore) UpdateUser(_ context.Context, user *core.User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.users[user.ID] = user
	return nil
}
func (s *mockStore) UpdatePassword(_ context.Context, _, _ string) error { return nil }
func (s *mockStore) UpdateLastLogin(_ context.Context, _ string) error   { return nil }
func (s *mockStore) CountUsers(_ context.Context) (int, error)           { return len(s.users), nil }
func (s *mockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}

func (s *mockStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (s *mockStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	return nil, nil
}
func (s *mockStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (s *mockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return s.apps, s.appCount, nil
}
func (s *mockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (s *mockStore) UpdateAppStatus(_ context.Context, _, _ string) error { return nil }
func (s *mockStore) DeleteApp(_ context.Context, _ string) error          { return nil }

func (s *mockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (s *mockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}
func (s *mockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (s *mockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}

func (s *mockStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (s *mockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, nil
}
func (s *mockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (s *mockStore) DeleteDomain(_ context.Context, _ string) error { return nil }
func (s *mockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	return nil, nil
}

func (s *mockStore) CreateProject(_ context.Context, _ *core.Project) error { return nil }
func (s *mockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, nil
}
func (s *mockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (s *mockStore) DeleteProject(_ context.Context, _ string) error { return nil }
func (s *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

func (s *mockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, nil
}
func (s *mockStore) GetUserMembership(_ context.Context, userID string) (*core.TeamMember, error) {
	m, ok := s.members[userID]
	if !ok {
		return nil, fmt.Errorf("membership not found")
	}
	return m, nil
}
func (s *mockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}

func (s *mockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (s *mockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return s.auditLogs, s.auditCount, nil
}

func (s *mockStore) CreateSecret(_ context.Context, _ *core.Secret) error { return nil }
func (s *mockStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}
func (s *mockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (s *mockStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, core.ErrNotFound
}
func (s *mockStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, core.ErrNotFound
}
func (s *mockStore) CreateInvite(_ context.Context, _ *core.Invitation) error { return nil }
func (s *mockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (s *mockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}
func (s *mockStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (s *mockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) { return nil, 0, nil }
func (s *mockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (s *mockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) { return nil, 0, nil }
func (s *mockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error { return nil }

func (s *mockStore) Close() error                 { return nil }
func (s *mockStore) Ping(_ context.Context) error { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testCore(enterpriseEnabled bool) *core.Core {
	cfg := &core.Config{}
	cfg.Enterprise.Enabled = enterpriseEnabled
	return &core.Core{
		Config:   cfg,
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
		Store:    newMockStore(),
	}
}

// ===========================================================================
// Module tests
// ===========================================================================

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModule_Identity(t *testing.T) {
	m := New()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ID", m.ID(), "enterprise"},
		{"Name", m.Name(), "Enterprise Engine"},
		{"Version", m.Version(), "1.0.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	if deps[0] != "core.db" || deps[1] != "billing" {
		t.Errorf("Dependencies() = %v, want [core.db billing]", deps)
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", h)
	}
}

func TestModule_Init(t *testing.T) {
	c := testCore(false)
	m := New()

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if m.core != c {
		t.Error("Init() should set core reference")
	}
	if m.logger == nil {
		t.Error("Init() should set logger")
	}
}

func TestModule_Start_Enabled(t *testing.T) {
	c := testCore(true)
	m := New()

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
}

func TestModule_Start_Disabled(t *testing.T) {
	c := testCore(false)
	m := New()

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
}

func TestModule_Stop(t *testing.T) {
	m := New()
	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

// ===========================================================================
// GDPRExporter tests
// ===========================================================================

func TestNewGDPRExporter(t *testing.T) {
	store := newMockStore()
	g := NewGDPRExporter(store)
	if g == nil {
		t.Fatal("NewGDPRExporter returned nil")
	}
}

func TestGDPRExporter_ExportData_Success(t *testing.T) {
	store := newMockStore()
	now := time.Now()
	store.users["user-1"] = &core.User{
		ID:        "user-1",
		Email:     "test@example.com",
		Name:      "Test User",
		AvatarURL: "https://img.example.com/avatar.png",
		Status:    "active",
		CreatedAt: now,
	}
	store.members["user-1"] = &core.TeamMember{
		TenantID: "tenant-1",
		UserID:   "user-1",
		RoleID:   "role-admin",
	}
	store.apps = []core.Application{
		{ID: "app-1", Name: "App 1"},
		{ID: "app-2", Name: "App 2"},
	}
	store.appCount = 2
	store.auditLogs = []core.AuditEntry{
		{Action: "login"},
		{Action: "deploy"},
		{Action: "update"},
	}
	store.auditCount = 3

	g := NewGDPRExporter(store)
	export, err := g.ExportData(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ExportData() error: %v", err)
	}

	if export.User.ID != "user-1" {
		t.Errorf("User.ID = %q, want %q", export.User.ID, "user-1")
	}
	if export.User.Email != "test@example.com" {
		t.Errorf("User.Email = %q, want %q", export.User.Email, "test@example.com")
	}
	if export.User.Name != "Test User" {
		t.Errorf("User.Name = %q, want %q", export.User.Name, "Test User")
	}
	if export.User.AvatarURL != "https://img.example.com/avatar.png" {
		t.Errorf("User.AvatarURL = %q", export.User.AvatarURL)
	}
	if export.User.Status != "active" {
		t.Errorf("User.Status = %q, want %q", export.User.Status, "active")
	}
	if export.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want %q", export.TenantID, "tenant-1")
	}
	if export.RoleID != "role-admin" {
		t.Errorf("RoleID = %q, want %q", export.RoleID, "role-admin")
	}
	if export.Applications != 2 {
		t.Errorf("Applications = %d, want 2", export.Applications)
	}
	if export.AuditEntries != 3 {
		t.Errorf("AuditEntries = %d, want 3", export.AuditEntries)
	}
	if export.ExportedAt.IsZero() {
		t.Error("ExportedAt should not be zero")
	}
}

func TestGDPRExporter_ExportData_UserNotFound(t *testing.T) {
	store := newMockStore()
	g := NewGDPRExporter(store)

	_, err := g.ExportData(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
	if got := err.Error(); got != "get user: user not found: nonexistent" {
		t.Errorf("error = %q", got)
	}
}

func TestGDPRExporter_ExportData_NoMembership(t *testing.T) {
	store := newMockStore()
	store.users["user-2"] = &core.User{
		ID:    "user-2",
		Email: "solo@example.com",
		Name:  "Solo User",
	}
	// No membership

	g := NewGDPRExporter(store)
	export, err := g.ExportData(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("ExportData() error: %v", err)
	}

	if export.TenantID != "" {
		t.Errorf("TenantID should be empty for user without membership, got %q", export.TenantID)
	}
	if export.Applications != 0 {
		t.Errorf("Applications should be 0, got %d", export.Applications)
	}
	if export.AuditEntries != 0 {
		t.Errorf("AuditEntries should be 0, got %d", export.AuditEntries)
	}
}

func TestGDPRExporter_EraseData_Success(t *testing.T) {
	store := newMockStore()
	userID := "user-erase-full-id-here"
	store.users[userID] = &core.User{
		ID:           userID,
		Email:        "erase@example.com",
		Name:         "Erase Me",
		AvatarURL:    "https://img.example.com/me.png",
		PasswordHash: "hashed-password",
		Status:       "active",
	}

	g := NewGDPRExporter(store)
	err := g.EraseData(context.Background(), userID)
	if err != nil {
		t.Fatalf("EraseData() error: %v", err)
	}

	// Verify user was anonymized
	u := store.users[userID]
	if u.Name != "Deleted User" {
		t.Errorf("Name should be 'Deleted User', got %q", u.Name)
	}
	if u.AvatarURL != "" {
		t.Errorf("AvatarURL should be empty, got %q", u.AvatarURL)
	}
	if u.PasswordHash != "" {
		t.Errorf("PasswordHash should be empty, got %q", u.PasswordHash)
	}
	if u.Status != "deleted" {
		t.Errorf("Status should be 'deleted', got %q", u.Status)
	}
	// Email: "deleted-" + user.ID[:8] + "@anonymized.local"
	expectedEmail := fmt.Sprintf("deleted-%s@anonymized.local", userID[:8])
	if u.Email != expectedEmail {
		t.Errorf("Email = %q, want %q", u.Email, expectedEmail)
	}
}

func TestGDPRExporter_EraseData_UserNotFound(t *testing.T) {
	store := newMockStore()
	g := NewGDPRExporter(store)

	err := g.EraseData(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestGDPRExporter_EraseData_UpdateError(t *testing.T) {
	store := newMockStore()
	userID := "user-fail-long-enough"
	store.users[userID] = &core.User{
		ID:    userID,
		Email: "fail@example.com",
		Name:  "Fail User",
	}
	store.updateErr = fmt.Errorf("database error")

	g := NewGDPRExporter(store)
	err := g.EraseData(context.Background(), userID)
	if err == nil {
		t.Fatal("expected error from UpdateUser failure")
	}
	if err.Error() != "database error" {
		t.Errorf("error = %q, want 'database error'", err.Error())
	}
}

func TestDataExport_ToJSON(t *testing.T) {
	export := &DataExport{
		ExportedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		User: UserData{
			ID:    "user-1",
			Email: "test@example.com",
			Name:  "Test User",
		},
		TenantID:     "tenant-1",
		RoleID:       "role-admin",
		Applications: 5,
		AuditEntries: 10,
	}

	data, err := export.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	// Verify valid JSON
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}

	if parsed["tenant_id"] != "tenant-1" {
		t.Errorf("tenant_id = %v, want 'tenant-1'", parsed["tenant_id"])
	}
	if parsed["applications_count"].(float64) != 5 {
		t.Errorf("applications_count = %v, want 5", parsed["applications_count"])
	}
}

// ===========================================================================
// Branding tests
// ===========================================================================

func TestDefaultBranding(t *testing.T) {
	b := DefaultBranding()
	if b == nil {
		t.Fatal("DefaultBranding() returned nil")
	}
	if b.AppName != "DeployMonster" {
		t.Errorf("AppName = %q, want %q", b.AppName, "DeployMonster")
	}
	if b.PrimaryColor != "#10b981" {
		t.Errorf("PrimaryColor = %q, want %q", b.PrimaryColor, "#10b981")
	}
	if b.AccentColor != "#8b5cf6" {
		t.Errorf("AccentColor = %q, want %q", b.AccentColor, "#8b5cf6")
	}
	if b.Copyright != "DeployMonster by ECOSTACK TECHNOLOGY" {
		t.Errorf("Copyright = %q", b.Copyright)
	}
	if b.SupportEmail != "support@deploy.monster" {
		t.Errorf("SupportEmail = %q", b.SupportEmail)
	}
}

func TestBranding_ToJSON(t *testing.T) {
	b := &Branding{
		AppName:       "MyPaaS",
		PrimaryColor:  "#ff0000",
		HidePoweredBy: true,
		CustomCSS:     "body { color: red; }",
	}

	jsonStr := b.ToJSON()
	if jsonStr == "" {
		t.Fatal("ToJSON() returned empty string")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("ToJSON() produced invalid JSON: %v", err)
	}
	if parsed["app_name"] != "MyPaaS" {
		t.Errorf("app_name = %v, want 'MyPaaS'", parsed["app_name"])
	}
	if parsed["hide_powered_by"] != true {
		t.Errorf("hide_powered_by = %v, want true", parsed["hide_powered_by"])
	}
	if parsed["custom_css"] != "body { color: red; }" {
		t.Errorf("custom_css = %v", parsed["custom_css"])
	}
}

func TestBranding_AllFields(t *testing.T) {
	b := &Branding{
		LogoURL:       "https://cdn.example.com/logo.png",
		LogoDarkURL:   "https://cdn.example.com/logo-dark.png",
		FaviconURL:    "https://cdn.example.com/favicon.ico",
		AppName:       "CloudPaaS",
		Domain:        "cloud.example.com",
		PrimaryColor:  "#00ff00",
		AccentColor:   "#0000ff",
		Copyright:     "CloudPaaS Inc.",
		SupportEmail:  "help@cloud.example.com",
		SupportURL:    "https://help.cloud.example.com",
		HidePoweredBy: true,
		CustomCSS:     ".header { display: none; }",
	}

	jsonStr := b.ToJSON()
	var parsed Branding
	json.Unmarshal([]byte(jsonStr), &parsed)

	if parsed.LogoURL != b.LogoURL {
		t.Errorf("LogoURL mismatch")
	}
	if parsed.LogoDarkURL != b.LogoDarkURL {
		t.Errorf("LogoDarkURL mismatch")
	}
	if parsed.FaviconURL != b.FaviconURL {
		t.Errorf("FaviconURL mismatch")
	}
	if parsed.Domain != b.Domain {
		t.Errorf("Domain mismatch")
	}
	if parsed.SupportURL != b.SupportURL {
		t.Errorf("SupportURL mismatch")
	}
}

// ===========================================================================
// BrandingStore tests
// ===========================================================================

func TestNewBrandingStore(t *testing.T) {
	bs := NewBrandingStore()
	if bs == nil {
		t.Fatal("NewBrandingStore() returned nil")
	}
	if bs.platform == nil {
		t.Fatal("platform branding should not be nil")
	}
	if bs.tenants == nil {
		t.Fatal("tenants map should not be nil")
	}
}

func TestBrandingStore_GetPlatform(t *testing.T) {
	bs := NewBrandingStore()
	b := bs.GetPlatform()
	if b == nil {
		t.Fatal("GetPlatform() returned nil")
	}
	if b.AppName != "DeployMonster" {
		t.Errorf("default platform AppName = %q, want %q", b.AppName, "DeployMonster")
	}
}

func TestBrandingStore_SetPlatform(t *testing.T) {
	bs := NewBrandingStore()
	custom := &Branding{
		AppName:      "CustomPlatform",
		PrimaryColor: "#abcdef",
	}
	bs.SetPlatform(custom)

	got := bs.GetPlatform()
	if got.AppName != "CustomPlatform" {
		t.Errorf("AppName = %q, want %q", got.AppName, "CustomPlatform")
	}
	if got.PrimaryColor != "#abcdef" {
		t.Errorf("PrimaryColor = %q, want %q", got.PrimaryColor, "#abcdef")
	}
}

func TestBrandingStore_GetTenant_WithOverride(t *testing.T) {
	bs := NewBrandingStore()

	tenantBranding := &Branding{
		AppName:      "TenantApp",
		PrimaryColor: "#111111",
	}
	bs.SetTenant("tenant-1", tenantBranding)

	got := bs.GetTenant("tenant-1")
	if got.AppName != "TenantApp" {
		t.Errorf("AppName = %q, want %q", got.AppName, "TenantApp")
	}
}

func TestBrandingStore_GetTenant_FallbackToPlatform(t *testing.T) {
	bs := NewBrandingStore()

	// No tenant-specific branding — should fall back to platform default
	got := bs.GetTenant("unknown-tenant")
	if got.AppName != "DeployMonster" {
		t.Errorf("expected platform default, got AppName = %q", got.AppName)
	}
}

func TestBrandingStore_SetTenant(t *testing.T) {
	bs := NewBrandingStore()

	bs.SetTenant("t1", &Branding{AppName: "Tenant1"})
	bs.SetTenant("t2", &Branding{AppName: "Tenant2"})

	if bs.GetTenant("t1").AppName != "Tenant1" {
		t.Error("tenant t1 branding not set correctly")
	}
	if bs.GetTenant("t2").AppName != "Tenant2" {
		t.Error("tenant t2 branding not set correctly")
	}
}

func TestBrandingStore_SetTenant_Overwrite(t *testing.T) {
	bs := NewBrandingStore()

	bs.SetTenant("t1", &Branding{AppName: "Old"})
	bs.SetTenant("t1", &Branding{AppName: "New"})

	got := bs.GetTenant("t1")
	if got.AppName != "New" {
		t.Errorf("expected overwritten branding, got AppName = %q", got.AppName)
	}
}

func TestBrandingStore_ThreadSafety(t *testing.T) {
	bs := NewBrandingStore()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		tenantID := fmt.Sprintf("tenant-%d", i)

		go func(id string) {
			defer wg.Done()
			bs.SetTenant(id, &Branding{AppName: id})
		}(tenantID)

		go func(id string) {
			defer wg.Done()
			_ = bs.GetTenant(id)
		}(tenantID)

		go func() {
			defer wg.Done()
			_ = bs.GetPlatform()
		}()
	}
	wg.Wait()

	// Also test concurrent SetPlatform
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			bs.SetPlatform(&Branding{AppName: fmt.Sprintf("Platform-%d", i)})
		}(i)
		go func() {
			defer wg.Done()
			_ = bs.GetPlatform()
		}()
	}
	wg.Wait()
}
