package secrets

import "testing"

// FuzzEncryptDecrypt verifies that any plaintext survives an encrypt→decrypt
// round-trip without data loss or corruption.
func FuzzEncryptDecrypt(f *testing.F) {
	// Seed corpus: empty, short, medium, binary-ish
	f.Add("")
	f.Add("hello world")
	f.Add("super-secret-database-password-123!")
	f.Add("unicode: \u00e4\u00f6\u00fc\u00df \u2603 \U0001f680")
	f.Add(string([]byte{0, 1, 2, 255, 254, 253}))

	vault := NewVault("fuzz-master-key-for-testing")

	f.Fuzz(func(t *testing.T, plaintext string) {
		encrypted, err := vault.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt failed: %v", err)
		}

		decrypted, err := vault.Decrypt(encrypted)
		if err != nil {
			t.Fatalf("Decrypt failed: %v", err)
		}

		if decrypted != plaintext {
			t.Errorf("round-trip mismatch: got %q, want %q", decrypted, plaintext)
		}
	})
}

// FuzzKeyDerivation verifies that NewVault always produces a 32-byte key
// regardless of the passphrase input.
func FuzzKeyDerivation(f *testing.F) {
	f.Add("")
	f.Add("short")
	f.Add("a-much-longer-secret-key-that-is-definitely-over-32-bytes")
	f.Add(string([]byte{0, 0, 0, 0}))

	f.Fuzz(func(t *testing.T, passphrase string) {
		v := NewVault(passphrase)
		if len(v.key) != 32 {
			t.Errorf("expected 32-byte key for passphrase %q, got %d bytes", passphrase, len(v.key))
		}
	})
}
