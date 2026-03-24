package webhooks

import "testing"

func TestSignPayload_Deterministic(t *testing.T) {
	payload := []byte(`{"event":"push"}`)
	secret := "test-secret"

	sig1 := signPayload(payload, secret)
	sig2 := signPayload(payload, secret)

	if sig1 != sig2 {
		t.Error("same payload+secret should produce same signature")
	}
}

func TestSignPayload_DifferentSecrets(t *testing.T) {
	payload := []byte(`{"event":"push"}`)

	sig1 := signPayload(payload, "secret-a")
	sig2 := signPayload(payload, "secret-b")

	if sig1 == sig2 {
		t.Error("different secrets should produce different signatures")
	}
}

func TestSignPayload_Length(t *testing.T) {
	sig := signPayload([]byte("test"), "secret")
	// HMAC-SHA256 produces 64 hex chars
	if len(sig) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(sig))
	}
}
