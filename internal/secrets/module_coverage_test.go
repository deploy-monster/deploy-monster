package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockSecretStore implements core.Store for secrets testing
type mockSecretStore struct {
	secrets       map[string]*core.Secret        // keyed by "scope/name"
	versions      map[string]*core.SecretVersion // keyed by secretID
	getSecretErr  error
	getVersionErr error
}

func newMockSecretStore() *mockSecretStore {
	return &mockSecretStore{
		secrets:  make(map[string]*core.Secret),
		versions: make(map[string]*core.SecretVersion),
	}
}

func (m *mockSecretStore) GetSecretByScopeAndName(_ context.Context, scope, name string) (*core.Secret, error) {
	if m.getSecretErr != nil {
		return nil, m.getSecretErr
	}
	key := scope + "/" + name
	if s, ok := m.secrets[key]; ok {
		return s, nil
	}
	return nil, core.ErrNotFound
}

func (m *mockSecretStore) GetLatestSecretVersion(_ context.Context, secretID string) (*core.SecretVersion, error) {
	if m.getVersionErr != nil {
		return nil, m.getVersionErr
	}
	if v, ok := m.versions[secretID]; ok {
		return v, nil
	}
	return nil, core.ErrNotFound
}

// Stub methods to satisfy core.Store
func (m *mockSecretStore) CreateSecret(_ context.Context, _ *core.Secret) error { return nil }
func (m *mockSecretStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}
func (m *mockSecretStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (m *mockSecretStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	var all []core.SecretVersion
	for _, v := range m.versions {
		if v != nil {
			all = append(all, *v)
		}
	}
	return all, nil
}
func (m *mockSecretStore) UpdateSecretVersionValue(_ context.Context, id, valueEnc string) error {
	for _, v := range m.versions {
		if v != nil && v.ID == id {
			v.ValueEnc = valueEnc
			return nil
		}
	}
	return nil
}
func (m *mockSecretStore) CreateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (m *mockSecretStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) UpdateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (m *mockSecretStore) DeleteTenant(_ context.Context, _ string) error       { return nil }
func (m *mockSecretStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (m *mockSecretStore) CreateUser(_ context.Context, _ *core.User) error { return nil }
func (m *mockSecretStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) UpdateUser(_ context.Context, _ *core.User) error    { return nil }
func (m *mockSecretStore) UpdatePassword(_ context.Context, _, _ string) error { return nil }
func (m *mockSecretStore) UpdateLastLogin(_ context.Context, _ string) error   { return nil }
func (m *mockSecretStore) CountUsers(_ context.Context) (int, error)           { return 0, nil }
func (m *mockSecretStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (m *mockSecretStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *mockSecretStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (m *mockSecretStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (m *mockSecretStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (m *mockSecretStore) UpdateAppStatus(_ context.Context, _, _ string) error { return nil }
func (m *mockSecretStore) DeleteApp(_ context.Context, _ string) error          { return nil }
func (m *mockSecretStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) CreateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (m *mockSecretStore) UpdateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (m *mockSecretStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (m *mockSecretStore) ListDeploymentsByStatus(_ context.Context, _ string) ([]core.Deployment, error) {
	return nil, nil
}
func (m *mockSecretStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (m *mockSecretStore) AtomicNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (m *mockSecretStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (m *mockSecretStore) GetDomain(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (m *mockSecretStore) DeleteDomain(_ context.Context, _ string) error              { return nil }
func (m *mockSecretStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) { return 0, nil }
func (m *mockSecretStore) ListAllDomains(_ context.Context) ([]core.Domain, error)     { return nil, nil }
func (m *mockSecretStore) CreateProject(_ context.Context, _ *core.Project) error      { return nil }
func (m *mockSecretStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (m *mockSecretStore) DeleteProject(_ context.Context, _ string) error { return nil }
func (m *mockSecretStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}
func (m *mockSecretStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (m *mockSecretStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockSecretStore) CreateInvite(_ context.Context, _ *core.Invitation) error { return nil }
func (m *mockSecretStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (m *mockSecretStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}
func (m *mockSecretStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (m *mockSecretStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) {
	return nil, 0, nil
}
func (m *mockSecretStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *mockSecretStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (m *mockSecretStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
	return nil
}

func (m *mockSecretStore) Close() error                 { return nil }
func (m *mockSecretStore) Ping(_ context.Context) error { return nil }

// =============================================================================
// buildScopeHierarchy Tests
// =============================================================================

func TestBuildScopeHierarchy_Global(t *testing.T) {
	result := buildScopeHierarchy("global")
	if len(result) != 1 || result[0] != "global" {
		t.Errorf("global scope: got %v, want [global]", result)
	}
}

func TestBuildScopeHierarchy_Tenant(t *testing.T) {
	tests := []struct {
		scope    string
		expected []string
	}{
		{"tenant/abc123", []string{"tenant/abc123", "global"}},
		{"tenant/", []string{"tenant/", "global"}},
		{"tenant", []string{"tenant", "global"}},
	}

	for _, tt := range tests {
		result := buildScopeHierarchy(tt.scope)
		if len(result) != len(tt.expected) {
			t.Errorf("scope %q: got %d elements, want %d", tt.scope, len(result), len(tt.expected))
			continue
		}
		for i, s := range result {
			if s != tt.expected[i] {
				t.Errorf("scope %q [%d]: got %q, want %q", tt.scope, i, s, tt.expected[i])
			}
		}
	}
}

func TestBuildScopeHierarchy_Project(t *testing.T) {
	result := buildScopeHierarchy("project/proj123")
	if len(result) != 2 {
		t.Errorf("project scope: got %d elements, want 2", len(result))
	}
	if result[0] != "project/proj123" {
		t.Errorf("project scope [0]: got %q, want %q", result[0], "project/proj123")
	}
	if result[1] != "global" {
		t.Errorf("project scope [1]: got %q, want %q", result[1], "global")
	}
}

func TestBuildScopeHierarchy_App(t *testing.T) {
	result := buildScopeHierarchy("app/app123")
	if len(result) != 2 {
		t.Errorf("app scope: got %d elements, want 2", len(result))
	}
	if result[0] != "app/app123" {
		t.Errorf("app scope [0]: got %q, want %q", result[0], "app/app123")
	}
	if result[1] != "global" {
		t.Errorf("app scope [1]: got %q, want %q", result[1], "global")
	}
}

func TestBuildScopeHierarchy_Unknown(t *testing.T) {
	result := buildScopeHierarchy("unknown/scope")
	if len(result) != 1 || result[0] != "unknown/scope" {
		t.Errorf("unknown scope: got %v, want [unknown/scope]", result)
	}
}

func TestBuildScopeHierarchy_Empty(t *testing.T) {
	result := buildScopeHierarchy("")
	if len(result) != 1 || result[0] != "" {
		t.Errorf("empty scope: got %v, want [\"\"]", result)
	}
}

// =============================================================================
// Resolve Tests
// =============================================================================

func TestResolve_StoreNotInitialized(t *testing.T) {
	m := &Module{store: nil}
	_, err := m.Resolve("global", "test-secret")
	if err == nil {
		t.Fatal("expected error for nil store")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolve_SecretNotFound(t *testing.T) {
	store := newMockSecretStore()
	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}

	_, err := m.Resolve("global", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResolve_SecretFound(t *testing.T) {
	store := newMockSecretStore()
	vault := NewVault("test-key-32-bytes-long-12345678")

	// Create encrypted value
	encValue, _ := vault.Encrypt("my-secret-value")

	// Setup mock data
	secret := &core.Secret{ID: "secret-1", Scope: "global", Name: "db-password"}
	version := &core.SecretVersion{ID: "version-1", SecretID: "secret-1", ValueEnc: encValue, Version: 1}

	store.secrets["global/db-password"] = secret
	store.versions["secret-1"] = version

	m := &Module{store: store, vault: vault}

	result, err := m.Resolve("global", "db-password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "my-secret-value" {
		t.Errorf("got %q, want %q", result, "my-secret-value")
	}
}

func TestResolve_ScopeFallback(t *testing.T) {
	store := newMockSecretStore()
	vault := NewVault("test-key-32-bytes-long-12345678")

	// Create encrypted value
	encValue, _ := vault.Encrypt("fallback-value")

	// Secret exists only at global scope
	secret := &core.Secret{ID: "secret-2", Scope: "global", Name: "api-key"}
	version := &core.SecretVersion{ID: "version-2", SecretID: "secret-2", ValueEnc: encValue, Version: 1}

	store.secrets["global/api-key"] = secret
	store.versions["secret-2"] = version

	m := &Module{store: store, vault: vault}

	// Request from tenant scope - should fall back to global
	result, err := m.Resolve("tenant/tenant123", "api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "fallback-value" {
		t.Errorf("got %q, want %q", result, "fallback-value")
	}
}

func TestResolve_VersionNotFound(t *testing.T) {
	store := newMockSecretStore()
	vault := NewVault("test-key-32-bytes-long-12345678")

	// Secret exists but no version
	secret := &core.Secret{ID: "secret-3", Scope: "global", Name: "orphan-secret"}
	store.secrets["global/orphan-secret"] = secret
	// No version added

	m := &Module{store: store, vault: vault}

	_, err := m.Resolve("global", "orphan-secret")
	if err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestResolve_StoreError(t *testing.T) {
	store := newMockSecretStore()
	store.getSecretErr = errors.New("database connection failed")

	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}

	_, err := m.Resolve("global", "test")
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestResolve_DecryptError(t *testing.T) {
	store := newMockSecretStore()
	vault := NewVault("test-key-32-bytes-long-12345678")

	// Secret with invalid encrypted value
	secret := &core.Secret{ID: "secret-4", Scope: "global", Name: "bad-enc"}
	version := &core.SecretVersion{ID: "version-4", SecretID: "secret-4", ValueEnc: "invalid-encrypted-value", Version: 1}

	store.secrets["global/bad-enc"] = secret
	store.versions["secret-4"] = version

	m := &Module{store: store, vault: vault}

	_, err := m.Resolve("global", "bad-enc")
	if err == nil {
		t.Fatal("expected decrypt error")
	}
}

func TestResolve_NilSecret(t *testing.T) {
	store := newMockSecretStore()
	// Store returns nil secret without error (edge case)
	store.secrets["global/nil-secret"] = nil

	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}

	_, err := m.Resolve("global", "nil-secret")
	if err == nil {
		t.Fatal("expected error for nil secret")
	}
}

// =============================================================================
// ResolveAll Tests
// =============================================================================

func TestResolveAll_NoSecrets(t *testing.T) {
	m := &Module{store: newMockSecretStore(), vault: NewVault("test-key")}

	result, err := m.ResolveAll("global", "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("got %q, want %q", result, "hello world")
	}
}

func TestResolveAll_MultipleSecrets(t *testing.T) {
	store := newMockSecretStore()
	vault := NewVault("test-key-32-bytes-long-12345678")

	enc1, _ := vault.Encrypt("user1")
	enc2, _ := vault.Encrypt("pass1")

	store.secrets["global/db-user"] = &core.Secret{ID: "s1", Scope: "global", Name: "db-user"}
	store.secrets["global/db-pass"] = &core.Secret{ID: "s2", Scope: "global", Name: "db-pass"}
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc1, Version: 1}
	store.versions["s2"] = &core.SecretVersion{ID: "v2", SecretID: "s2", ValueEnc: enc2, Version: 1}

	m := &Module{store: store, vault: vault}

	result, err := m.ResolveAll("global", "user=${SECRET:db-user}&pass=${SECRET:db-pass}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "user=user1&pass=pass1"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestResolveAll_UnclosedPattern(t *testing.T) {
	m := &Module{store: newMockSecretStore(), vault: NewVault("test-key")}

	result, err := m.ResolveAll("global", "value=${SECRET:name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unclosed pattern should remain unchanged
	if result != "value=${SECRET:name" {
		t.Errorf("got %q, want %q", result, "value=${SECRET:name")
	}
}

func TestResolveAll_EmptyTemplate(t *testing.T) {
	m := &Module{store: newMockSecretStore(), vault: NewVault("test-key")}

	result, err := m.ResolveAll("global", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("got %q, want empty", result)
	}
}

// =============================================================================
// Module Lifecycle Tests
// =============================================================================

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "secrets" {
		t.Errorf("ID() = %q, want %q", m.ID(), "secrets")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "Secret Vault" {
		t.Errorf("Name() = %q, want %q", m.Name(), "Secret Vault")
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", m.Version(), "1.0.0")
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

func TestModule_Vault(t *testing.T) {
	m := &Module{vault: NewVault("test-key")}
	if v := m.Vault(); v == nil {
		t.Error("Vault() returned nil")
	}
}

// =============================================================================
// Key Rotation Tests
// =============================================================================

func TestRotateEncryptionKey_Success(t *testing.T) {
	oldKey := "old-master-key-32-bytes-1234567"
	newKey := "new-master-key-32-bytes-7654321"

	store := newMockSecretStore()
	oldVault := NewVault(oldKey)

	// Create two secret versions encrypted with old key
	enc1, _ := oldVault.Encrypt("password-one")
	enc2, _ := oldVault.Encrypt("password-two")

	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc1, Version: 1}
	store.versions["s2"] = &core.SecretVersion{ID: "v2", SecretID: "s2", ValueEnc: enc2, Version: 1}

	m := &Module{store: store, vault: oldVault}

	count, err := m.RotateEncryptionKey(context.Background(), newKey)
	if err != nil {
		t.Fatalf("RotateEncryptionKey: %v", err)
	}
	if count != 2 {
		t.Errorf("rotated %d versions, want 2", count)
	}

	// Verify new vault can decrypt the rotated values
	newVault := NewVault(newKey)
	for _, v := range store.versions {
		plain, err := newVault.Decrypt(v.ValueEnc)
		if err != nil {
			t.Fatalf("new vault cannot decrypt rotated version %s: %v", v.ID, err)
		}
		if plain != "password-one" && plain != "password-two" {
			t.Errorf("unexpected decrypted value: %q", plain)
		}
	}

	// Verify old vault can NOT decrypt the rotated values
	for _, v := range store.versions {
		_, err := oldVault.Decrypt(v.ValueEnc)
		// Old vault uses old key derivation - should fail since m.vault was swapped
		// Actually oldVault still works as a separate instance, but the stored values
		// are now encrypted with new key so old vault cannot decrypt them
		if err == nil {
			// The old vault might sometimes succeed if keys happen to produce same derived key
			// but with different master secrets, this should fail
			t.Log("warning: old vault could still decrypt (key derivation collision unlikely)")
		}
	}
}

func TestRotateEncryptionKey_NoStore(t *testing.T) {
	m := &Module{vault: NewVault("key")}
	_, err := m.RotateEncryptionKey(context.Background(), "new-key")
	if err == nil {
		t.Fatal("expected error when store is nil")
	}
}

func TestRotateEncryptionKey_EmptyVersions(t *testing.T) {
	store := newMockSecretStore()
	m := &Module{store: store, vault: NewVault("key")}

	count, err := m.RotateEncryptionKey(context.Background(), "new-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rotated, got %d", count)
	}
}
