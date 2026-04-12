package enterprise

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"

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
func (s *mockStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
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
func (s *mockStore) UpdateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (s *mockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}
func (s *mockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (s *mockStore) ListDeploymentsByStatus(_ context.Context, _ string) ([]core.Deployment, error) {
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
func (s *mockStore) DeleteDomain(_ context.Context, _ string) error              { return nil }
func (s *mockStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) { return 0, nil }
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
func (s *mockStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return nil, nil
}
func (s *mockStore) UpdateSecretVersionValue(_ context.Context, _, _ string) error {
	return nil
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
func (s *mockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) {
	return nil, 0, nil
}
func (s *mockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (s *mockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
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

func TestBrandingStore_ThreadSafety(t *testing.T) {
	bs := NewBrandingStore()

	var wg sync.WaitGroup
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
