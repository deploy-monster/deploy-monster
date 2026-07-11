package secrets

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Init — error paths and legacy branch
// ---------------------------------------------------------------------------

func TestModule_Init_ResolveVaultSaltError(t *testing.T) {
	// Stored invalid base64 causes resolveVaultSalt to return an error.
	bolt := newFakeBolt()
	_ = bolt.Set(VaultBucket, VaultSaltKey, "!!!invalid-base64!!!", 0)

	m := New()
	svc := core.NewServices()
	svc.Secrets = nil
	c := &core.Core{
		Logger:   slog.Default(),
		Services: svc,
		DB:       &core.Database{Bolt: bolt},
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "test-server-key-32-bytes-long!!"},
			Secrets: core.SecretsConfig{EncryptionKey: ""},
		},
	}
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for invalid base64 stored salt")
	}
	if !strings.Contains(err.Error(), "resolve vault salt") {
		t.Errorf("error = %q, want 'resolve vault salt'", err.Error())
	}
}

func TestModule_Init_UsedLegacyFlag(t *testing.T) {
	// No persisted salt + existing legacy secrets = usedLegacy path.
	bolt := newFakeBolt() // no vault/salt key
	store := newMockSecretStore()
	v := NewVault("test-key-32-bytes-long-12345678")
	enc, _ := v.Encrypt("pre-existing-secret")
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}

	m := New()
	svc := core.NewServices()
	c := &core.Core{
		Logger:   slog.Default(),
		Services: svc,
		DB:       &core.Database{Bolt: bolt},
		Store:    store,
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "test-key-32-bytes-long-12345678"},
			Secrets: core.SecretsConfig{EncryptionKey: ""},
		},
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.vault == nil {
		t.Fatal("vault is nil after Init")
	}
}

// ---------------------------------------------------------------------------
// Start — migration error path
// ---------------------------------------------------------------------------

func TestModule_Start_MigrationError(t *testing.T) {
	// Bolt without salt + store that fails ListAllSecretVersions.
	bolt := newFakeBolt()
	store := &fakeStore{listErr: errors.New("list failed")}

	m := &Module{
		bolt:         bolt,
		store:        store,
		logger:       slog.Default(),
		vault:        NewVault("test-key-32-bytes-long-12345678"),
		masterSecret: "test-key-32-bytes-long-12345678",
	}
	err := m.Start(context.Background())
	if err == nil {
		t.Fatal("expected migration error")
	}
	if !strings.Contains(err.Error(), "vault migration") {
		t.Errorf("error = %q, want 'vault migration'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// migrateLegacyVault — decrypt error
// ---------------------------------------------------------------------------

func TestMigrateLegacyVault_DecryptError(t *testing.T) {
	bolt := newFakeBolt()
	store := &fakeStore{
		versions: []core.SecretVersion{
			{ID: "v1", ValueEnc: "corrupted-not-valid-ciphertext"},
		},
	}
	m := newTestModule(bolt, store)
	m.vault = NewVaultWithSalt("test-master-secret", LegacyVaultSalt())

	err := m.migrateLegacyVault(context.Background())
	if err == nil {
		t.Fatal("expected decrypt error")
	}
	if !strings.Contains(err.Error(), "decrypt legacy version") {
		t.Errorf("error = %q, want 'decrypt legacy version'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Resolve — version is nil branch
// ---------------------------------------------------------------------------

type nilVersionStore struct {
	mockSecretStore
}

func (n *nilVersionStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, nil
}

func TestResolve_VersionIsNil(t *testing.T) {
	store := &nilVersionStore{mockSecretStore: *newMockSecretStore()}
	store.secrets["global/x"] = &core.Secret{ID: "s1", Scope: "global", Name: "x"}
	m := &Module{store: store, vault: NewVault("test-key-32-bytes-long-12345678")}
	_, err := m.Resolve(context.Background(), "global", "x")
	if err == nil {
		t.Fatal("expected error for nil version")
	}
	if !strings.Contains(err.Error(), "no versions found") {
		t.Errorf("error = %q, want 'no versions found'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Resolve — logger.Debug branch (line 303-305)
// ---------------------------------------------------------------------------

func TestResolve_LoggerDebugCalled(t *testing.T) {
	store := newMockSecretStore()
	vault := NewVault("test-key-32-bytes-long-12345678")
	enc, _ := vault.Encrypt("secret-val")
	store.secrets["global/foo"] = &core.Secret{ID: "s1", Scope: "global", Name: "foo"}
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}

	m := &Module{
		store:  store,
		vault:  vault,
		logger: slog.Default(),
	}
	val, err := m.Resolve(context.Background(), "global", "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "secret-val" {
		t.Errorf("got %q, want %q", val, "secret-val")
	}
}

// ---------------------------------------------------------------------------
// RotateEncryptionKey — persistSalt error after successful rotation
// ---------------------------------------------------------------------------

type persistFailBolt struct {
	fakeBolt
	persistFail   bool
	persistCalled bool
}

func (p *persistFailBolt) Set(bucket, key string, value any, ttl int64) error {
	if bucket == VaultBucket && key == VaultSaltKey && p.persistFail {
		p.persistCalled = true
		return errors.New("bolt persist failed")
	}
	return p.fakeBolt.Set(bucket, key, value, ttl)
}

func TestRotateEncryptionKey_PersistSaltError(t *testing.T) {
	store := newMockSecretStore()
	v := NewVault("old-key-32-bytes-long-12345678")
	enc, _ := v.Encrypt("my-secret")
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}

	bolt := &persistFailBolt{
		fakeBolt:    *newFakeBolt(),
		persistFail: true,
	}

	m := &Module{
		store:        store,
		vault:        v,
		logger:       slog.Default(),
		bolt:         bolt,
		masterSecret: "old-key-32-bytes-long-12345678",
	}
	_, err := m.RotateEncryptionKey(context.Background(), "new-key-32-bytes-long-87654321")
	if err == nil {
		t.Fatal("expected persist error")
	}
	if !strings.Contains(err.Error(), "persist rotated salt") {
		t.Errorf("error = %q, want 'persist rotated salt'", err.Error())
	}
	if !bolt.persistCalled {
		t.Error("persistSalt was not called")
	}
}

// NOTE: In Go 1.26+ crypto/rand.Read failures are fatal panics, so
// the following error branches are unreachable in unit tests:
//   - vault.go:39-41 (rand.Read error in GenerateVaultSalt)
//   - vault.go:107-109 (io.ReadFull rand.Reader error in Encrypt)
//   - module.go:136-138 (GenerateVaultSalt error in resolveVaultSalt)
//   - module.go:188-190 (GenerateVaultSalt error in migrateLegacyVault)
//   - module.go:206-208 (newVault.Encrypt error in migrateLegacyVault)
//   - module.go:333-335 (GenerateVaultSalt error in RotateEncryptionKey)
//   - module.go:353-355 (newVault.Encrypt error in RotateEncryptionKey)
//
// NOTE: cipher.NewGCM(block) always succeeds for AES (16-byte blocks),
// so vault.go:102-104 and vault.go:128-130 are unreachable.
//
// NOTE: strings.Split always returns at least 1 element, so
// buildScopeHierarchy's `len(parts) < 1` check (module.go:241-243)
// is unreachable.
