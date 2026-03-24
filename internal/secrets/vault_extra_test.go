package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

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
	_, err := m.ResolveAll("global", "password=${SECRET:db_pass}")
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

	_, err := m.ResolveAll("global", template)
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
	result, err := m.ResolveAll("global", "no-secrets-here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "no-secrets-here" {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

func TestModule_ResolveAll_EmptyTemplate(t *testing.T) {
	m := New()
	result, err := m.ResolveAll("global", "")
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
	result, err := m.ResolveAll("global", "value=${SECRET:name")
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
	_, err := m.Resolve("global", "nonexistent")
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
	// Before Init, Health should still return HealthOK
	if m.Health() != 0 { // core.HealthOK == 0
		t.Errorf("Health: expected HealthOK (0), got %d", m.Health())
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
	if err := m.Stop(nil); err != nil {
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
