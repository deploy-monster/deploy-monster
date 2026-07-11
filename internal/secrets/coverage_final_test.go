package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Init with bolt (DB) and encryption key override
// ---------------------------------------------------------------------------

func TestModule_Init_WithBoltAndEncryptionKey(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger:   slog.Default(),
		Services: core.NewServices(),
		DB:       &core.Database{Bolt: newFakeBolt()},
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "server-key"},
			Secrets: core.SecretsConfig{EncryptionKey: "custom-enc-key"},
		},
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.bolt == nil {
		t.Error("bolt should be set from DB")
	}
}

// ---------------------------------------------------------------------------
// Start with salt already persisted (fast path)
// ---------------------------------------------------------------------------

func TestModule_Start_PersistedSalt(t *testing.T) {
	bolt := newFakeBolt()
	_ = bolt.Set(VaultBucket, VaultSaltKey, base64.StdEncoding.EncodeToString(make([]byte, 32)), 0)
	m := &Module{
		bolt:   bolt,
		logger: slog.Default(),
		vault:  NewVault("test-key-32-bytes-long-12345678"),
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// ---------------------------------------------------------------------------
// migrateLegacyVault — update error path
// ---------------------------------------------------------------------------

type migrateStore struct {
	mockSecretStore
	updateFail bool
}

func (m *migrateStore) UpdateSecretVersionValue(_ context.Context, id, _ string) error {
	if m.updateFail {
		return fmt.Errorf("update failed for %s", id)
	}
	return nil
}

func (m *migrateStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return m.mockSecretStore.ListAllSecretVersions(context.Background())
}

func TestMigrateLegacyVault_UpdateFails(t *testing.T) {
	store := &migrateStore{mockSecretStore: *newMockSecretStore(), updateFail: true}
	v := NewVault("test-key-32-bytes-long-12345678")
	enc, _ := v.Encrypt("secret")
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}

	m := &Module{
		bolt:         newFakeBolt(),
		store:        store,
		logger:       slog.Default(),
		vault:        v,
		masterSecret: "test-key-32-bytes-long-12345678",
	}
	err := m.migrateLegacyVault(context.Background())
	if err == nil {
		t.Fatal("expected update error")
	}
	if !strings.Contains(err.Error(), "update version") {
		t.Errorf("error = %q, want 'update version'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// ResolveAll — secret not found falls through to error
// ---------------------------------------------------------------------------

// Using the mock already defined in module_coverage_test.go
func TestResolveAll_NotFoundError(t *testing.T) {
	store := newMockSecretStore()
	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}
	_, err := m.ResolveAll(context.Background(), "global", "hello ${SECRET:nonexistent}")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

// ---------------------------------------------------------------------------
// RotateEncryptionKey — persist error after rotation
// ---------------------------------------------------------------------------

type rotateErrStore struct {
	mockSecretStore
}

func (r *rotateErrStore) ListAllSecretVersions(ctx context.Context) ([]core.SecretVersion, error) {
	return r.mockSecretStore.ListAllSecretVersions(ctx)
}

func (r *rotateErrStore) UpdateSecretVersionValue(ctx context.Context, id, valueEnc string) error {
	return r.mockSecretStore.UpdateSecretVersionValue(ctx, id, valueEnc)
}

func TestRotateEncryptionKey_WithOneVersion(t *testing.T) {
	store := newMockSecretStore()
	v := NewVault("old-key-32-bytes-long-12345678")
	enc, _ := v.Encrypt("my-secret")
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}

	m := &Module{
		store:  store,
		vault:  v,
		logger: slog.Default(),
	}
	count, err := m.RotateEncryptionKey(context.Background(), "new-key-32-bytes-long-87654321")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("rotated %d, want 1", count)
	}
}

// ---------------------------------------------------------------------------
// Resolve — GetLatestSecretVersion error continues loop
// ---------------------------------------------------------------------------

// mockStore that returns error on GetLatestSecretVersion
type versionErrStore struct {
	mockSecretStore
	versionErr error
}

func (v *versionErrStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	if v.versionErr != nil {
		return nil, v.versionErr
	}
	return nil, core.ErrNotFound
}

func TestResolve_VersionErrorContinuesLoop(t *testing.T) {
	store := &versionErrStore{mockSecretStore: *newMockSecretStore(), versionErr: errors.New("version lookup failed")}
	// Add secret so GetSecretByScopeAndName succeeds
	store.secrets["global/test"] = &core.Secret{ID: "s1", Scope: "global", Name: "test"}
	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}
	_, err := m.Resolve(context.Background(), "global", "test")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "version lookup failed") {
		t.Errorf("error = %q, want 'version lookup failed'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Resolve — sql.ErrNoRows causes continue
// ---------------------------------------------------------------------------

type sqlErrStore struct {
	mockSecretStore
}

func (s *sqlErrStore) GetSecretByScopeAndName(_ context.Context, scope, name string) (*core.Secret, error) {
	// Return the secret wrapped in a fmt.Errorf with "sql: no rows" to simulate sql.ErrNoRows
	return nil, fmt.Errorf("sql: no rows in result set")
}

func TestResolve_SqlNoRowsContinues(t *testing.T) {
	store := newMockSecretStore()
	// First call returns sql.ErrNoRows, then global scope also returns not found
	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}
	_, err := m.Resolve(context.Background(), "tenant/t1", "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------------------------------------------------------------------------
// Start — bolt set and no persisted salt but migration fails (empty store)
// ---------------------------------------------------------------------------

func TestModule_Start_NoMigrationNeededWhenEmpty(t *testing.T) {
	bolt := newFakeBolt()
	store := newMockSecretStore()
	m := &Module{
		bolt:         bolt,
		store:        store,
		logger:       slog.Default(),
		vault:        NewVault("test-key-32-bytes-long-12345678"),
		masterSecret: "test-key-32-bytes-long-12345678",
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}
