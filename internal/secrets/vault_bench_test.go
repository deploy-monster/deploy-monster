package secrets

import "testing"

func BenchmarkEncrypt(b *testing.B) {
	vault := NewVault("benchmark-key")
	plaintext := "super-secret-database-password-that-is-fairly-long"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vault.Encrypt(plaintext)
	}
}

func BenchmarkDecrypt(b *testing.B) {
	vault := NewVault("benchmark-key")
	encrypted, _ := vault.Encrypt("super-secret-database-password")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vault.Decrypt(encrypted)
	}
}
