package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// ---------------------------------------------------------------------------
// Vault salt primitives
// ---------------------------------------------------------------------------

func TestGenerateVaultSalt_UniqueAndRightLength(t *testing.T) {
	seen := make(map[string]bool, 8)
	for i := 0; i < 8; i++ {
		salt, err := GenerateVaultSalt()
		if err != nil {
			t.Fatalf("GenerateVaultSalt: %v", err)
		}
		if len(salt) != VaultSaltLen {
			t.Errorf("salt length %d, want %d", len(salt), VaultSaltLen)
		}
		if seen[string(salt)] {
			t.Fatalf("duplicate salt at iteration %d — crypto/rand broken?", i)
		}
		seen[string(salt)] = true
	}
}

func TestLegacyVaultSalt_Stable(t *testing.T) {
	// The legacy salt MUST be stable across calls — every legacy
	// install derived its key from this exact value and any drift
	// would break decryption of existing secrets.
	a := LegacyVaultSalt()
	b := LegacyVaultSalt()
	if !bytes.Equal(a, b) {
		t.Errorf("LegacyVaultSalt drifted between calls: %x vs %x", a, b)
	}
	if len(a) != 16 {
		t.Errorf("legacy salt length %d, want 16", len(a))
	}
}

func TestNewVaultWithSalt_DifferentSaltsYieldDifferentKeys(t *testing.T) {
	s1, _ := GenerateVaultSalt()
	s2, _ := GenerateVaultSalt()

	v1 := NewVaultWithSalt("same-master", s1)
	v2 := NewVaultWithSalt("same-master", s2)

	ct1, err := v1.Encrypt("hello")
	if err != nil {
		t.Fatalf("encrypt v1: %v", err)
	}
	// v2 must NOT be able to decrypt v1's ciphertext — different salt
	// means different AES key.
	if _, err := v2.Decrypt(ct1); err == nil {
		t.Error("vault2 decrypted vault1 ciphertext; salts must produce distinct keys")
	}
}

func TestNewVaultWithSalt_EmptySaltFallsBackToLegacy(t *testing.T) {
	// Defensive: nil/empty salt should degrade to legacy salt so
	// callers can't accidentally construct an unkeyed vault.
	v := NewVaultWithSalt("key", nil)
	legacy := NewVault("key")

	ct, err := legacy.Encrypt("round-trip")
	if err != nil {
		t.Fatalf("legacy encrypt: %v", err)
	}
	got, err := v.Decrypt(ct)
	if err != nil {
		t.Fatalf("nil-salt vault couldn't decrypt legacy ciphertext: %v", err)
	}
	if got != "round-trip" {
		t.Errorf("decrypted %q, want round-trip", got)
	}
}

// ---------------------------------------------------------------------------
// Module resolveVaultSalt / migrateLegacyVault
// ---------------------------------------------------------------------------

// fakeBolt is the minimum BoltStorer surface resolveVaultSalt touches.
type fakeBolt struct {
	mu  sync.Mutex
	kv  map[string]string
	err error // injected error for Get/Set
}

func newFakeBolt() *fakeBolt { return &fakeBolt{kv: map[string]string{}} }

func (b *fakeBolt) key(bucket, key string) string { return bucket + "/" + key }

func (b *fakeBolt) Set(bucket, key string, value any, _ int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	s, ok := value.(string)
	if !ok {
		return fmt.Errorf("fakeBolt.Set: non-string value")
	}
	b.kv[b.key(bucket, key)] = s
	return nil
}

func (b *fakeBolt) BatchSet([]core.BoltBatchItem) error { return nil }

func (b *fakeBolt) Get(bucket, key string, dest any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return b.err
	}
	v, ok := b.kv[b.key(bucket, key)]
	if !ok {
		return errors.New("not found")
	}
	sp, ok := dest.(*string)
	if !ok {
		return fmt.Errorf("fakeBolt.Get: dest must be *string")
	}
	*sp = v
	return nil
}

func (b *fakeBolt) Delete(string, string) error   { return nil }
func (b *fakeBolt) List(string) ([]string, error) { return nil, nil }
func (b *fakeBolt) Close() error                  { return nil }
func (b *fakeBolt) GetAPIKeyByPrefix(context.Context, string) (*models.APIKey, error) {
	return nil, nil
}
func (b *fakeBolt) GetWebhookSecret(string) (string, error) { return "", nil }

// fakeStore is a minimal core.Store stub covering only the secret
// methods resolveVaultSalt and migrateLegacyVault use.
type fakeStore struct {
	versions []core.SecretVersion
	updates  map[string]string // id -> new encrypted value

	core.Store // embed to satisfy the interface — unused methods panic on use
}

func (s *fakeStore) ListAllSecretVersions(context.Context) ([]core.SecretVersion, error) {
	return s.versions, nil
}

func (s *fakeStore) UpdateSecretVersionValue(_ context.Context, id, enc string) error {
	if s.updates == nil {
		s.updates = map[string]string{}
	}
	s.updates[id] = enc
	for i := range s.versions {
		if s.versions[i].ID == id {
			s.versions[i].ValueEnc = enc
		}
	}
	return nil
}

// ---------------------------------------------------------------------------

func newTestModule(bolt core.BoltStorer, store core.Store) *Module {
	return &Module{
		bolt:         bolt,
		store:        store,
		logger:       slog.Default(),
		masterSecret: "test-master-secret",
	}
}

func TestResolveVaultSalt_FreshInstall_GeneratesAndPersists(t *testing.T) {
	bolt := newFakeBolt()
	m := newTestModule(bolt, &fakeStore{})

	salt, usedLegacy, err := m.resolveVaultSalt(context.Background())
	if err != nil {
		t.Fatalf("resolveVaultSalt: %v", err)
	}
	if usedLegacy {
		t.Error("fresh install should not use legacy salt")
	}
	if len(salt) != VaultSaltLen {
		t.Errorf("salt length %d, want %d", len(salt), VaultSaltLen)
	}

	// Salt must have been persisted so the next boot skips generation.
	stored, ok := bolt.kv["vault/salt"]
	if !ok {
		t.Fatal("fresh-install path did not persist salt to bolt")
	}
	decoded, _ := base64.StdEncoding.DecodeString(stored)
	if !bytes.Equal(decoded, salt) {
		t.Error("persisted salt does not match returned salt")
	}
}

func TestResolveVaultSalt_SubsequentBoot_ReusesPersisted(t *testing.T) {
	bolt := newFakeBolt()
	expected, _ := GenerateVaultSalt()
	bolt.kv["vault/salt"] = base64.StdEncoding.EncodeToString(expected)

	m := newTestModule(bolt, &fakeStore{})
	salt, usedLegacy, err := m.resolveVaultSalt(context.Background())
	if err != nil {
		t.Fatalf("resolveVaultSalt: %v", err)
	}
	if usedLegacy {
		t.Error("boot with persisted salt should not flag legacy")
	}
	if !bytes.Equal(salt, expected) {
		t.Error("resolved salt does not match stored salt")
	}
}

func TestResolveVaultSalt_LegacyUpgrade_FlagsMigration(t *testing.T) {
	bolt := newFakeBolt() // no vault/salt key
	store := &fakeStore{
		versions: []core.SecretVersion{
			{ID: "v1", SecretID: "s1", ValueEnc: "placeholder"},
		},
	}
	m := newTestModule(bolt, store)

	salt, usedLegacy, err := m.resolveVaultSalt(context.Background())
	if err != nil {
		t.Fatalf("resolveVaultSalt: %v", err)
	}
	if !usedLegacy {
		t.Fatal("existing secrets + no persisted salt must flag legacy migration")
	}
	if len(salt) != VaultSaltLen {
		t.Errorf("salt length %d, want %d", len(salt), VaultSaltLen)
	}
	// Must NOT persist the new salt yet — migration runs in Start().
	if _, ok := bolt.kv["vault/salt"]; ok {
		t.Error("legacy upgrade path must not persist salt until migration completes")
	}
}

func TestMigrateLegacyVault_ReEncryptsAllVersions(t *testing.T) {
	// Seed the store with a version encrypted under the legacy salt.
	const master = "reencrypt-master"
	legacy := NewVault(master)
	ct, err := legacy.Encrypt("original-plaintext")
	if err != nil {
		t.Fatalf("legacy encrypt: %v", err)
	}

	bolt := newFakeBolt()
	store := &fakeStore{
		versions: []core.SecretVersion{
			{ID: "v1", SecretID: "s1", ValueEnc: ct, Version: 1},
		},
	}
	m := &Module{
		bolt:         bolt,
		store:        store,
		logger:       slog.Default(),
		masterSecret: master,
		vault:        legacy, // booted with legacy vault
	}

	if err := m.migrateLegacyVault(context.Background()); err != nil {
		t.Fatalf("migrateLegacyVault: %v", err)
	}

	// Salt must now be persisted.
	storedSaltB64, ok := bolt.kv["vault/salt"]
	if !ok {
		t.Fatal("migration did not persist salt")
	}
	storedSalt, _ := base64.StdEncoding.DecodeString(storedSaltB64)

	// The new ciphertext must decrypt under the new-salt vault.
	if store.updates["v1"] == "" {
		t.Fatal("migration did not update version v1")
	}
	newVault := NewVaultWithSalt(master, storedSalt)
	got, err := newVault.Decrypt(store.updates["v1"])
	if err != nil {
		t.Fatalf("new vault couldn't decrypt migrated ciphertext: %v", err)
	}
	if got != "original-plaintext" {
		t.Errorf("decrypted %q, want original-plaintext", got)
	}

	// And the module's live vault was swapped.
	if m.vault == legacy {
		t.Error("module.vault still points at legacy vault after migration")
	}
}

func TestMigrateLegacyVault_IdempotentOnRestart(t *testing.T) {
	// Simulate: legacy boot → migration runs → second boot must not
	// re-migrate because the persisted salt is now present.
	const master = "idempotent-master"
	legacy := NewVault(master)
	ct, _ := legacy.Encrypt("payload")

	bolt := newFakeBolt()
	store := &fakeStore{
		versions: []core.SecretVersion{
			{ID: "v1", SecretID: "s1", ValueEnc: ct, Version: 1},
		},
	}
	m := &Module{
		bolt:         bolt,
		store:        store,
		logger:       slog.Default(),
		masterSecret: master,
		vault:        legacy,
	}
	if err := m.migrateLegacyVault(context.Background()); err != nil {
		t.Fatalf("first migration: %v", err)
	}

	// Second boot: resolveVaultSalt must see the persisted salt and
	// return usedLegacy=false.
	_, usedLegacy, err := m.resolveVaultSalt(context.Background())
	if err != nil {
		t.Fatalf("second resolveVaultSalt: %v", err)
	}
	if usedLegacy {
		t.Error("second boot flagged legacy migration; salt persistence failed to take effect")
	}
}
