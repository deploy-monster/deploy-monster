package secrets

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// === merged from final90_test.go ===

// =============================================================================
// Decrypt — empty input (zero-length base64 decodes to empty bytes)
// =============================================================================

func TestVault_Decrypt_EmptyString(t *testing.T) {
	vault := NewVault("empty-input-key")

	_, err := vault.Decrypt("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

// =============================================================================
// Decrypt — exactly nonce size (12 bytes) but no ciphertext — GCM Open fails
// =============================================================================

func TestVault_Decrypt_ExactNonceNoPayload(t *testing.T) {
	vault := NewVault("nonce-only-key")

	// 12 bytes = exactly the GCM nonce size, but no ciphertext follows
	data := make([]byte, 12)
	encoded := base64.StdEncoding.EncodeToString(data)

	_, err := vault.Decrypt(encoded)
	if err == nil {
		t.Error("expected error for nonce-only ciphertext (no auth tag)")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("expected 'decrypt' error, got: %v", err)
	}
}

// =============================================================================
// ResolveAll — nested ${SECRET:x} patterns (resolve error on first ref)
// =============================================================================

func TestModule_ResolveAll_NestedSecretRef(t *testing.T) {
	m := New()

	// Template where a resolved value would itself contain ${SECRET:...}
	// Since Resolve is a stub returning error, we just verify first ref fails
	_, err := m.ResolveAll(context.Background(), "scope", "prefix-${SECRET:outer}-suffix")
	if err == nil {
		t.Fatal("expected error from Resolve stub")
	}
	if !strings.Contains(err.Error(), "outer") {
		t.Errorf("error should mention 'outer', got: %v", err)
	}
}

// =============================================================================
// ResolveAll — multiple patterns in sequence with closing braces
// =============================================================================

func TestModule_ResolveAll_MidStringUnclosed(t *testing.T) {
	m := New()

	// This has ${SECRET: but the closing brace is missing at a weird offset
	result, err := m.ResolveAll(context.Background(), "scope", "abc${SECRET:name_no_close_here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be unchanged because no closing brace
	if result != "abc${SECRET:name_no_close_here" {
		t.Errorf("expected unchanged, got %q", result)
	}
}

// =============================================================================
// Vault — Decrypt with just under nonce size
// =============================================================================

func TestVault_Decrypt_OneByteShort(t *testing.T) {
	vault := NewVault("short-nonce-key")

	// 11 bytes = one byte short of nonce size
	data := make([]byte, 11)
	encoded := base64.StdEncoding.EncodeToString(data)

	_, err := vault.Decrypt(encoded)
	if err == nil {
		t.Error("expected error for data shorter than nonce")
	}
	if !strings.Contains(err.Error(), "ciphertext too short") {
		t.Errorf("expected 'ciphertext too short', got: %v", err)
	}
}

// =============================================================================
// Encrypt — empty plaintext round-trip
// =============================================================================

func TestVault_EncryptDecrypt_EmptyPlaintext(t *testing.T) {
	vault := NewVault("empty-plaintext-key")

	enc, err := vault.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	dec, err := vault.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if dec != "" {
		t.Errorf("expected empty string, got %q", dec)
	}
}

// === merged from module_boost_test.go ===

func TestModule_Start_WithSalt(t *testing.T) {
	kv := newFakeKV()
	// Seed a salt so the legacy migration path is skipped
	_ = kv.Set(VaultBucket, VaultSaltKey, "c2FsdC12YWx1ZQ==", 0)

	m := &Module{
		kv:     kv,
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
	kv := newFakeKV()
	// No salt persisted — triggers legacy migration path

	m := &Module{
		kv:     kv,
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
	if err := kv.Get(VaultBucket, VaultSaltKey, &stored); err != nil {
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
		t.Fatalf("Start with nil kv: %v", err)
	}
}

func TestModule_PersistSalt_NilBolt(t *testing.T) {
	m := &Module{
		logger: slog.Default(),
		vault:  NewVault("test-master-key-32-bytes-long-"),
	}

	// Should not panic and should return nil when kv is nil
	if err := m.persistSalt([]byte("some-salt")); err != nil {
		t.Errorf("persistSalt with nil kv: %v", err)
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

// === merged from rotate_followup_test.go ===

// rotateStoreWrapper composes the existing mockSecretStore so we can
// override the two methods RotateEncryptionKey actually calls and
// inject error scenarios that the success-path test does not exercise.
type rotateStoreWrapper struct {
	*mockSecretStore
	listErr   error
	updateErr error
}

func (w *rotateStoreWrapper) ListAllSecretVersions(ctx context.Context) ([]core.SecretVersion, error) {
	if w.listErr != nil {
		return nil, w.listErr
	}
	return w.mockSecretStore.ListAllSecretVersions(ctx)
}

func (w *rotateStoreWrapper) UpdateSecretVersionValue(ctx context.Context, id, valueEnc string) error {
	if w.updateErr != nil {
		return w.updateErr
	}
	return w.mockSecretStore.UpdateSecretVersionValue(ctx, id, valueEnc)
}

func TestRotateEncryptionKey_ListVersionsError(t *testing.T) {
	store := &rotateStoreWrapper{
		mockSecretStore: newMockSecretStore(),
		listErr:         errors.New("boom: db unavailable"),
	}
	m := &Module{store: store, vault: NewVault("any-32-bytes-or-more-master-key!")}

	rotated, err := m.RotateEncryptionKey(context.Background(), "new-master-key-32-bytes-7654321")
	if err == nil {
		t.Fatal("expected error when ListAllSecretVersions fails")
	}
	if !strings.Contains(err.Error(), "list secret versions") {
		t.Errorf("err = %v, want wrapped 'list secret versions'", err)
	}
	if rotated != 0 {
		t.Errorf("rotated = %d, want 0 on early failure", rotated)
	}
}

func TestRotateEncryptionKey_DecryptFailureMidway(t *testing.T) {
	old := NewVault("old-master-key-32-bytes-1234567")
	store := newMockSecretStore()

	// First version is properly encrypted; second carries a corrupted
	// ciphertext so the rotation loop fails on the second iteration.
	good, _ := old.Encrypt("ok-secret")
	store.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: good, Version: 1}
	store.versions["s2"] = &core.SecretVersion{ID: "v2", SecretID: "s2", ValueEnc: "not-base64!!", Version: 1}

	m := &Module{store: store, vault: old}

	rotated, err := m.RotateEncryptionKey(context.Background(), "new-master-key-32-bytes-7654321")
	if err == nil {
		t.Fatal("expected decrypt error on corrupted ciphertext")
	}
	if !strings.Contains(err.Error(), "decrypt version") {
		t.Errorf("err = %v, want wrapped 'decrypt version'", err)
	}
	// Map iteration order is unstable; the failure may interrupt before
	// the good version was rotated, so allow rotated >= 0.
	if rotated < 0 || rotated > 1 {
		t.Errorf("rotated = %d, want 0 or 1 (depends on map iteration order)", rotated)
	}
}

func TestRotateEncryptionKey_UpdateStoreError(t *testing.T) {
	old := NewVault("old-master-key-32-bytes-1234567")
	enc, _ := old.Encrypt("password")

	wrapper := &rotateStoreWrapper{mockSecretStore: newMockSecretStore()}
	wrapper.versions["s1"] = &core.SecretVersion{ID: "v1", SecretID: "s1", ValueEnc: enc, Version: 1}
	wrapper.updateErr = errors.New("write disk full")

	m := &Module{store: wrapper, vault: old}

	rotated, err := m.RotateEncryptionKey(context.Background(), "new-master-key-32-bytes-7654321")
	if err == nil {
		t.Fatal("expected error when UpdateSecretVersionValue fails")
	}
	if !strings.Contains(err.Error(), "update version") {
		t.Errorf("err = %v, want wrapped 'update version'", err)
	}
	if rotated != 0 {
		t.Errorf("rotated = %d, want 0 (failure on first iteration)", rotated)
	}
}

// === merged from secrets_final_test.go ===

// =============================================================================
// Coverage targets:
//   module.go:12  init         50%  — init() calls RegisterModule; tested via New()
//   module.go:73  ResolveAll   92.9% — line 93 (success replacement) needs Resolve to succeed
//   vault.go:31   Encrypt      72.7% — error branches at 34,39,44 unreachable with valid key
//   vault.go:52   Decrypt      88.2% — already covered by other files
//
// This file adds ONLY tests with unique names not found in any other test file.
// =============================================================================

// TestFinal_Vault_EncryptDecrypt_Roundtrip exercises the full Encrypt+Decrypt
// path end-to-end. Encrypt lines 31-48 and Decrypt lines 52-79 are all hit on success.
func TestFinal_Vault_EncryptDecrypt_Roundtrip(t *testing.T) {
	vault := NewVault("final-roundtrip-key")

	tests := []struct {
		name      string
		plaintext string
	}{
		{"ascii", "hello world"},
		{"empty", ""},
		{"binary", string([]byte{0, 1, 2, 255, 254, 253})},
		{"long", strings.Repeat("abcdefgh", 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := vault.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if enc == "" {
				t.Fatal("ciphertext should not be empty")
			}

			dec, err := vault.Decrypt(enc)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if dec != tt.plaintext {
				t.Errorf("round-trip mismatch: got len=%d, want len=%d", len(dec), len(tt.plaintext))
			}
		})
	}
}

// TestFinal_Vault_Decrypt_AllErrorPaths exercises every Decrypt error branch.
func TestFinal_Vault_Decrypt_AllErrorPaths(t *testing.T) {
	vault := NewVault("final-decrypt-errors")

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "invalid base64 characters",
			input:   "!!!invalid-base64!!!",
			wantErr: "decode base64",
		},
		{
			name:    "too short for nonce (3 bytes)",
			input:   base64.StdEncoding.EncodeToString([]byte{1, 2, 3}),
			wantErr: "ciphertext too short",
		},
		{
			name:    "exactly nonce size no payload",
			input:   base64.StdEncoding.EncodeToString(make([]byte, 12)),
			wantErr: "decrypt",
		},
		{
			name:    "valid length but garbage ciphertext",
			input:   base64.StdEncoding.EncodeToString(make([]byte, 100)),
			wantErr: "decrypt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := vault.Decrypt(tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestFinal_Vault_Encrypt_NonceUniqueness verifies that two encryptions of the
// same plaintext produce different ciphertexts (nonce generation at line 43-44).
func TestFinal_Vault_Encrypt_NonceUniqueness(t *testing.T) {
	vault := NewVault("final-nonce-unique")

	results := make(map[string]bool)
	for i := 0; i < 20; i++ {
		enc, err := vault.Encrypt("same-plaintext-value")
		if err != nil {
			t.Fatalf("Encrypt iteration %d: %v", i, err)
		}
		if results[enc] {
			t.Fatal("duplicate ciphertext detected — nonce reuse")
		}
		results[enc] = true
	}
}

// TestFinal_Module_ResolveAll_NoSecretRefs exercises the early return path
// in ResolveAll when the template contains no ${SECRET:} patterns.
func TestFinal_Module_ResolveAll_NoSecretRefs(t *testing.T) {
	m := New()

	tests := []struct {
		name     string
		template string
	}{
		{"plain text", "no secrets here"},
		{"empty", ""},
		{"dollar sign", "$NOT_A_SECRET"},
		{"partial match", "${NOT_SECRET:foo}"},
		{"just braces", "${}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := m.ResolveAll(context.Background(), "scope", tt.template)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.template {
				t.Errorf("expected unchanged %q, got %q", tt.template, result)
			}
		})
	}
}

// TestFinal_Module_ResolveAll_UnclosedBraceVariants tests that unclosed
// ${SECRET: patterns are left as-is (covers the break at line 84).
func TestFinal_Module_ResolveAll_UnclosedBraceVariants(t *testing.T) {
	m := New()

	tests := []struct {
		name     string
		template string
	}{
		{"at end", "prefix${SECRET:key"},
		{"only prefix", "${SECRET:"},
		{"prefix with text after", "${SECRET:key but no closing brace and more text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := m.ResolveAll(context.Background(), "scope", tt.template)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.template {
				t.Errorf("expected unchanged %q, got %q", tt.template, result)
			}
		})
	}
}

// TestFinal_Module_ResolveAll_ResolveReturnsError exercises the error path
// at line 90 where Resolve fails for a valid ${SECRET:name} reference.
func TestFinal_Module_ResolveAll_ResolveReturnsError(t *testing.T) {
	m := New()

	_, err := m.ResolveAll(context.Background(), "global", "db_pass=${SECRET:db_password}")
	if err == nil {
		t.Fatal("expected error from Resolve stub")
	}
	if !strings.Contains(err.Error(), "db_password") {
		t.Errorf("error should reference secret name, got: %v", err)
	}
}

// TestFinal_Module_Resolve_StubError ensures Resolve returns an error.
func TestFinal_Module_Resolve_StubError(t *testing.T) {
	m := New()

	_, err := m.Resolve(context.Background(), "global", "any-name")
	if err == nil {
		t.Fatal("expected error from stub")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// === merged from vault_extra_test.go ===

// ---------------------------------------------------------------------------
// Encrypt / Decrypt with various data sizes
// ---------------------------------------------------------------------------

func TestVault_EncryptDecrypt_VariousSizes(t *testing.T) {
	vault := NewVault("size-test-key")

	tests := []struct {
		name string
		size int
	}{
		{"1 byte", 1},
		{"16 bytes (AES block)", 16},
		{"32 bytes", 32},
		{"256 bytes", 256},
		{"1 KB", 1024},
		{"10 KB", 10 * 1024},
		{"100 KB", 100 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plaintext := strings.Repeat("A", tt.size)

			encrypted, err := vault.Encrypt(plaintext)
			if err != nil {
				t.Fatalf("Encrypt(%d bytes): %v", tt.size, err)
			}

			decrypted, err := vault.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt(%d bytes): %v", tt.size, err)
			}

			if decrypted != plaintext {
				t.Errorf("round-trip failed for %d bytes: lengths %d vs %d",
					tt.size, len(plaintext), len(decrypted))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Decrypt with wrong key — expect error
// ---------------------------------------------------------------------------

func TestVault_DecryptWrongKey(t *testing.T) {
	correct := NewVault("correct-key")
	wrong := NewVault("wrong-key")

	encrypted, err := correct.Encrypt("top-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = wrong.Decrypt(encrypted)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("expected error to mention 'decrypt', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Decrypt with corrupted ciphertext
// ---------------------------------------------------------------------------

func TestVault_DecryptCorruptedCiphertext(t *testing.T) {
	vault := NewVault("corruption-key")

	tests := []struct {
		name  string
		input string
	}{
		{"not base64", "!!!not-valid-base64!!!"},
		{"empty string", ""},
		{"too short ciphertext", base64.StdEncoding.EncodeToString([]byte("short"))},
		{"flipped bits", flipBitsInCiphertext(t, vault)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := vault.Decrypt(tt.input)
			if err == nil {
				t.Error("expected error for corrupted ciphertext")
			}
		})
	}
}

// flipBitsInCiphertext encrypts data, then flips some bits in the ciphertext
// portion (after the nonce) so that GCM authentication fails.
func flipBitsInCiphertext(t *testing.T, vault *Vault) string {
	t.Helper()
	enc, err := vault.Encrypt("test-data-for-corruption")
	if err != nil {
		t.Fatalf("Encrypt for corruption test: %v", err)
	}
	raw, _ := base64.StdEncoding.DecodeString(enc)
	// Flip bits in the last byte (inside the ciphertext, not the nonce)
	if len(raw) > 0 {
		raw[len(raw)-1] ^= 0xFF
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// ---------------------------------------------------------------------------
// Encrypt produces valid base64 output
// ---------------------------------------------------------------------------

func TestVault_EncryptOutputIsBase64(t *testing.T) {
	vault := NewVault("base64-test")

	encrypted, err := vault.Encrypt("hello world")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Errorf("encrypted output is not valid base64: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewVault with different secrets produces different keys
// ---------------------------------------------------------------------------

func TestNewVault_DifferentSecretsProduceDifferentKeys(t *testing.T) {
	v1 := NewVault("secret-alpha")
	v2 := NewVault("secret-beta")

	if string(v1.key) == string(v2.key) {
		t.Error("different master secrets should produce different derived keys")
	}
}

// ---------------------------------------------------------------------------
// NewVault key is always 32 bytes (AES-256)
// ---------------------------------------------------------------------------

func TestNewVault_KeyLength(t *testing.T) {
	secrets := []string{"", "short", "a-much-longer-secret-key-that-is-definitely-over-32-bytes"}

	for _, s := range secrets {
		v := NewVault(s)
		if len(v.key) != 32 {
			t.Errorf("NewVault(%q): expected 32-byte key, got %d bytes", s, len(v.key))
		}
	}
}

// ---------------------------------------------------------------------------
// Resolve — ${SECRET:name} substitution via ResolveAll
// ---------------------------------------------------------------------------

func TestModule_ResolveAll_SingleSecret(t *testing.T) {
	m := New()
	// ResolveAll calls Resolve internally, which returns "not found" for now.
	// We verify the parsing logic: it should attempt to resolve the reference.
	_, err := m.ResolveAll(context.Background(), "global", "password=${SECRET:db_pass}")
	if err == nil {
		t.Fatal("expected error because Resolve is a stub returning 'not found'")
	}
	if !strings.Contains(err.Error(), "db_pass") {
		t.Errorf("error should mention the secret name 'db_pass', got: %v", err)
	}
}

func TestModule_ResolveAll_MultipleSecrets(t *testing.T) {
	m := New()
	template := "host=${SECRET:db_host} pass=${SECRET:db_pass}"

	_, err := m.ResolveAll(context.Background(), "global", template)
	if err == nil {
		t.Fatal("expected error from stub Resolve")
	}
	// The first secret reference should be attempted (db_host)
	if !strings.Contains(err.Error(), "db_host") {
		t.Errorf("error should mention 'db_host', got: %v", err)
	}
}

func TestModule_ResolveAll_NoSecrets(t *testing.T) {
	m := New()
	result, err := m.ResolveAll(context.Background(), "global", "no-secrets-here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "no-secrets-here" {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

func TestModule_ResolveAll_EmptyTemplate(t *testing.T) {
	m := New()
	result, err := m.ResolveAll(context.Background(), "global", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestModule_ResolveAll_UnclosedBrace(t *testing.T) {
	m := New()
	// ${SECRET:name without closing brace — should be left as-is
	result, err := m.ResolveAll(context.Background(), "global", "value=${SECRET:name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "value=${SECRET:name" {
		t.Errorf("expected unchanged string for unclosed brace, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Resolve — stub returns error
// ---------------------------------------------------------------------------

func TestModule_Resolve_NotFound(t *testing.T) {
	m := New()
	_, err := m.Resolve(context.Background(), "global", "nonexistent")
	if err == nil {
		t.Fatal("expected error from stub Resolve")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Module lifecycle: ID, Name, Version, Health
// ---------------------------------------------------------------------------

func TestModule_Identity(t *testing.T) {
	m := New()

	if m.ID() != "secrets" {
		t.Errorf("ID: expected 'secrets', got %q", m.ID())
	}
	if m.Name() != "Secret Vault" {
		t.Errorf("Name: expected 'Secret Vault', got %q", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version: expected '1.0.0', got %q", m.Version())
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	// Before Init, vault is nil → HealthDown
	if m.Health() != core.HealthDown {
		t.Errorf("Health before Init: expected HealthDown, got %d", m.Health())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies: expected [core.db], got %v", deps)
	}
}

func TestModule_RoutesAndEvents(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes: expected nil, got %v", routes)
	}
	if events := m.Events(); events != nil {
		t.Errorf("Events: expected nil, got %v", events)
	}
}

func TestModule_VaultAccessor(t *testing.T) {
	m := New()
	// Before Init, vault is nil
	if m.Vault() != nil {
		t.Error("Vault() should be nil before Init")
	}
}

func TestModule_StopIsNoop(t *testing.T) {
	m := New()
	if err := m.Stop(context.TODO()); err != nil {
		t.Errorf("Stop: unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Deterministic key derivation: same secret → same key
// ---------------------------------------------------------------------------

func TestNewVault_Deterministic(t *testing.T) {
	v1 := NewVault("deterministic-test")
	v2 := NewVault("deterministic-test")

	if string(v1.key) != string(v2.key) {
		t.Error("same master secret should produce the same derived key")
	}

	// Cross-verify: encrypt with one, decrypt with the other
	enc, err := v1.Encrypt("cross-check")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	dec, err := v2.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "cross-check" {
		t.Errorf("expected 'cross-check', got %q", dec)
	}
}
