package auth

import "testing"

func BenchmarkGenerateTokenPair(b *testing.B) {
	svc := NewJWTService("benchmark-secret-key-at-least-32-bytes!")
	for i := 0; i < b.N; i++ {
		svc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")
	}
}

func BenchmarkValidateAccessToken(b *testing.B) {
	svc := NewJWTService("benchmark-secret-key-at-least-32-bytes!")
	pair, _ := svc.GenerateTokenPair("user-1", "tenant-1", "role_admin", "test@example.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.ValidateAccessToken(pair.AccessToken)
	}
}

func BenchmarkHashPassword_Short(b *testing.B) {
	for i := 0; i < b.N; i++ {
		HashPassword("TestPassword123!")
	}
}
