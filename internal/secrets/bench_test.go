package secrets

import (
	"testing"
)

func BenchmarkEncrypt_Small(b *testing.B) {
	v := NewVault("benchmark-key-32-chars-long!!!!!")
	data := "small-secret-value"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Encrypt(data)
	}
}

func BenchmarkEncrypt_Medium(b *testing.B) {
	v := NewVault("benchmark-key-32-chars-long!!!!!")
	data := string(make([]byte, 1024))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Encrypt(data)
	}
}

func BenchmarkEncrypt_Large(b *testing.B) {
	v := NewVault("benchmark-key-32-chars-long!!!!!")
	data := string(make([]byte, 100*1024))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Encrypt(data)
	}
}

func BenchmarkDecrypt_Bench(b *testing.B) {
	v := NewVault("benchmark-key-32-chars-long!!!!!")
	encrypted, _ := v.Encrypt("secret-to-decrypt")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Decrypt(encrypted)
	}
}

func BenchmarkEncryptDecrypt_RoundTrip(b *testing.B) {
	v := NewVault("benchmark-key-32-chars-long!!!!!")
	data := "round-trip-secret-value-for-benchmarking"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, _ := v.Encrypt(data)
		v.Decrypt(enc)
	}
}
