package auth

import "testing"

func BenchmarkPasswordHash(b *testing.B) {
	for i := 0; i < b.N; i++ {
		HashPassword("BenchmarkP@ss123!")
	}
}

func BenchmarkPasswordCompare(b *testing.B) {
	hash, _ := HashPassword("BenchmarkP@ss123!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyPassword(hash, "BenchmarkP@ss123!")
	}
}

func BenchmarkJWTGenerate(b *testing.B) {
	svc := MustNewJWTService("bench-secret-key-at-least-32-bytes-long!")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "bench@example.com")
	}
}

func BenchmarkJWTValidate(b *testing.B) {
	svc := MustNewJWTService("bench-secret-key-at-least-32-bytes-long!")
	pair, _ := svc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "bench@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.ValidateAccessToken(pair.AccessToken)
	}
}
