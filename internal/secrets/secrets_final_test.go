package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

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
			result, err := m.ResolveAll("scope", tt.template)
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
			result, err := m.ResolveAll("scope", tt.template)
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

	_, err := m.ResolveAll("global", "db_pass=${SECRET:db_password}")
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

	_, err := m.Resolve("global", "any-name")
	if err == nil {
		t.Fatal("expected error from stub")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}
