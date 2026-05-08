package secrets

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_Start_WithSalt(t *testing.T) {
	bolt := newFakeBolt()
	// Seed a salt so the legacy migration path is skipped
	_ = bolt.Set(VaultBucket, VaultSaltKey, "c2FsdC12YWx1ZQ==", 0)

	m := &Module{
		bolt:   bolt,
		logger: slog.Default(),
		vault:  NewVault("test-master-key-32-bytes-long-"),
	}

	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestModule_Start_LegacyMigration(t *testing.T) {
	store := newMockSecretStore()
	bolt := newFakeBolt()
	// No salt persisted — triggers legacy migration path

	m := &Module{
		bolt:   bolt,
		store:  store,
		logger: slog.Default(),
		vault:  NewVault("test-master-key-32-bytes-long-"),
	}

	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// After migration, a salt should be persisted
	var stored string
	if err := bolt.Get(VaultBucket, VaultSaltKey, &stored); err != nil {
		t.Fatalf("salt not persisted after migration: %v", err)
	}
	if stored == "" {
		t.Error("expected non-empty salt after migration")
	}
}

func TestModule_Start_NilBolt(t *testing.T) {
	m := &Module{
		logger: slog.Default(),
		vault:  NewVault("test-master-key-32-bytes-long-"),
	}

	ctx := context.Background()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start with nil bolt: %v", err)
	}
}

func TestModule_PersistSalt_NilBolt(t *testing.T) {
	m := &Module{
		logger: slog.Default(),
		vault:  NewVault("test-master-key-32-bytes-long-"),
	}

	// Should not panic and should return nil when bolt is nil
	if err := m.persistSalt([]byte("some-salt")); err != nil {
		t.Errorf("persistSalt with nil bolt: %v", err)
	}
}

func (m *mockSecretStore) CreateServer(_ context.Context, _ *core.Server) error { return nil }
func (m *mockSecretStore) GetServer(_ context.Context, _ string) (*core.Server, error) {
	return nil, core.ErrNotFound
}
func (m *mockSecretStore) ListServersByTenant(_ context.Context, _ string) ([]core.Server, error) {
	return nil, nil
}
func (m *mockSecretStore) ListAllServers(_ context.Context) ([]core.Server, error) { return nil, nil }
func (m *mockSecretStore) UpdateServerStatus(_ context.Context, _, _ string) error { return nil }
func (m *mockSecretStore) DeleteServer(_ context.Context, _ string) error          { return nil }
