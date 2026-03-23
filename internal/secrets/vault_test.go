package secrets

import "testing"

func TestVault_EncryptDecrypt(t *testing.T) {
	vault := NewVault("test-master-secret-key")

	plaintext := "super-secret-database-password-123!"
	encrypted, err := vault.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	if encrypted == plaintext {
		t.Error("encrypted should not equal plaintext")
	}

	decrypted, err := vault.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestVault_DifferentKeys(t *testing.T) {
	v1 := NewVault("key-one")
	v2 := NewVault("key-two")

	encrypted, _ := v1.Encrypt("secret")

	_, err := v2.Decrypt(encrypted)
	if err == nil {
		t.Error("decrypting with wrong key should fail")
	}
}

func TestVault_UniqueEncryption(t *testing.T) {
	vault := NewVault("test-key")

	e1, _ := vault.Encrypt("same-plaintext")
	e2, _ := vault.Encrypt("same-plaintext")

	if e1 == e2 {
		t.Error("same plaintext should produce different ciphertext (random nonce)")
	}

	// But both should decrypt to the same value
	d1, _ := vault.Decrypt(e1)
	d2, _ := vault.Decrypt(e2)
	if d1 != d2 {
		t.Error("both should decrypt to same plaintext")
	}
}

func TestVault_EmptyString(t *testing.T) {
	vault := NewVault("test-key")

	encrypted, err := vault.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	decrypted, err := vault.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if decrypted != "" {
		t.Error("expected empty string")
	}
}
