package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

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
	_, err := m.ResolveAll("scope", "prefix-${SECRET:outer}-suffix")
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
	result, err := m.ResolveAll("scope", "abc${SECRET:name_no_close_here")
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
