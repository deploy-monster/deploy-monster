package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func BenchmarkHMAC_SHA256(b *testing.B) {
	secret := "webhook-secret-key-123"
	body := []byte(`{"ref":"refs/heads/main","commits":[{"id":"abc123","message":"test commit"}]}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		hex.EncodeToString(mac.Sum(nil))
	}
}

func BenchmarkSignPayload(b *testing.B) {
	secret := "outbound-webhook-secret"
	body := []byte(`{"event":"app.deployed","data":{"app_id":"app_123","version":5}}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		signPayload(body, secret)
	}
}

func BenchmarkSignPayload_LargeBody(b *testing.B) {
	secret := "outbound-webhook-secret"
	body := make([]byte, 64*1024)
	for i := range body {
		body[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		signPayload(body, secret)
	}
}
