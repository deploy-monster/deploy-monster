package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

// =============================================================================
// Vault.Encrypt — error paths for short/invalid ciphertext on decrypt
// =============================================================================

func TestVault_Decrypt_TooShortForNonce(t *testing.T) {
	vault := NewVault("nonce-test-key")

	// Create a base64 string that decodes to fewer bytes than GCM nonce (12 bytes)
	shortData := make([]byte, 5)
	encoded := base64.StdEncoding.EncodeToString(shortData)

	_, err := vault.Decrypt(encoded)
	if err == nil {
		t.Error("expected error for ciphertext shorter than nonce")
	}
	if !strings.Contains(err.Error(), "ciphertext too short") {
		t.Errorf("expected 'ciphertext too short', got: %v", err)
	}
}

func TestVault_Decrypt_InvalidBase64(t *testing.T) {
	vault := NewVault("base64-err-key")

	_, err := vault.Decrypt("not!valid!base64!@#$")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "decode base64") {
		t.Errorf("expected 'decode base64' error, got: %v", err)
	}
}

func TestVault_Decrypt_TamperedCiphertext(t *testing.T) {
	vault := NewVault("tamper-test-key")

	// Encrypt something valid
	enc, err := vault.Encrypt("secret-data")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// Decode, tamper with the ciphertext portion, re-encode
	raw, _ := base64.StdEncoding.DecodeString(enc)
	// Flip middle byte (past the nonce)
	midpoint := len(raw) / 2
	if midpoint < 12 {
		midpoint = 13
	}
	raw[midpoint] ^= 0xFF
	tampered := base64.StdEncoding.EncodeToString(raw)

	_, err = vault.Decrypt(tampered)
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("expected 'decrypt' error, got: %v", err)
	}
}

// =============================================================================
// Vault.Encrypt — round-trip with binary data
// =============================================================================

func TestVault_EncryptDecrypt_BinaryData(t *testing.T) {
	vault := NewVault("binary-test-key")

	// Create binary data with all byte values
	data := make([]byte, 256)
	for i := range 256 {
		data[i] = byte(i)
	}

	enc, err := vault.Encrypt(string(data))
	if err != nil {
		t.Fatalf("Encrypt binary: %v", err)
	}

	dec, err := vault.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt binary: %v", err)
	}

	if dec != string(data) {
		t.Errorf("binary round-trip failed: got %d bytes, want %d", len(dec), len(data))
	}
}

// =============================================================================
// Module — ResolveAll edge case: ${SECRET: prefix without closing brace
// at end of string
// =============================================================================

func TestModule_ResolveAll_SecretAtEndNoClose(t *testing.T) {
	m := New()
	// Template ends with ${SECRET:name — no closing brace
	result, err := m.ResolveAll("scope", "prefix-${SECRET:x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "prefix-${SECRET:x" {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

func TestModule_ResolveAll_OnlyPrefix(t *testing.T) {
	m := New()
	// Just the prefix without any name
	result, err := m.ResolveAll("scope", "${SECRET:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not loop forever; the closing brace check should break the loop
	if result != "${SECRET:" {
		t.Errorf("expected unchanged string, got %q", result)
	}
}

// =============================================================================
// Module — init() coverage via New()
// =============================================================================

func TestModule_New_ReturnsNonNil(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	// Vault should be nil before Init
	if m.vault != nil {
		t.Error("vault should be nil before Init")
	}
	// Store should be nil before Init
	if m.store != nil {
		t.Error("store should be nil before Init")
	}
}

// TestVault_Encrypt_LargePayload exercises the full Encrypt path
// with a payload large enough to ensure all internal buffer paths are hit.
func TestVault_Encrypt_LargePayload(t *testing.T) {
	vault := NewVault("large-payload-key")

	// 1MB payload
	data := strings.Repeat("x", 1024*1024)
	enc, err := vault.Encrypt(data)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}

	dec, err := vault.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}
	if len(dec) != len(data) {
		t.Errorf("size mismatch: got %d, want %d", len(dec), len(data))
	}
}
