package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestNewStripeClient(t *testing.T) {
	client := NewStripeClient("sk_test_123", "whsec_test")
	if client == nil {
		t.Fatal("NewStripeClient returned nil")
	}
	if client.secretKey != "sk_test_123" {
		t.Errorf("secretKey = %q, want %q", client.secretKey, "sk_test_123")
	}
	if client.webhookKey != "whsec_test" {
		t.Errorf("webhookKey = %q, want %q", client.webhookKey, "whsec_test")
	}
	if client.client == nil {
		t.Error("HTTP client should be initialized")
	}
}

func TestVerifyWebhookSignature(t *testing.T) {
	webhookKey := "whsec_test_secret"
	client := NewStripeClient("sk_test", webhookKey)

	payload := []byte(`{"type":"checkout.session.completed"}`)
	timestamp := "1616161616"

	// Compute a valid signature.
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(webhookKey))
	mac.Write([]byte(signedPayload))
	validSig := hex.EncodeToString(mac.Sum(nil))

	tests := []struct {
		name      string
		payload   []byte
		sigHeader string
		want      bool
	}{
		{
			name:      "valid signature",
			payload:   payload,
			sigHeader: fmt.Sprintf("t=%s,v1=%s", timestamp, validSig),
			want:      true,
		},
		{
			name:      "invalid signature",
			payload:   payload,
			sigHeader: fmt.Sprintf("t=%s,v1=%s", timestamp, "badsignature"),
			want:      false,
		},
		{
			name:      "empty sig header",
			payload:   payload,
			sigHeader: "",
			want:      false,
		},
		{
			name:      "missing timestamp",
			payload:   payload,
			sigHeader: fmt.Sprintf("v1=%s", validSig),
			want:      false,
		},
		{
			name:      "missing v1",
			payload:   payload,
			sigHeader: fmt.Sprintf("t=%s", timestamp),
			want:      false,
		},
		{
			name:      "tampered payload",
			payload:   []byte(`{"type":"modified"}`),
			sigHeader: fmt.Sprintf("t=%s,v1=%s", timestamp, validSig),
			want:      false,
		},
		{
			name:      "wrong timestamp",
			payload:   payload,
			sigHeader: fmt.Sprintf("t=%s,v1=%s", "9999999999", validSig),
			want:      false,
		},
		{
			name:      "malformed header no equals",
			payload:   payload,
			sigHeader: "garbage",
			want:      false,
		},
		{
			name:      "extra fields in header",
			payload:   payload,
			sigHeader: fmt.Sprintf("t=%s,v1=%s,v0=legacy", timestamp, validSig),
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.VerifyWebhookSignature(tt.payload, tt.sigHeader)
			if got != tt.want {
				t.Errorf("VerifyWebhookSignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifyWebhookSignature_EmptyWebhookKey(t *testing.T) {
	client := NewStripeClient("sk_test", "")

	// With empty webhook key, verification should always fail.
	got := client.VerifyWebhookSignature([]byte("payload"), "t=123,v1=abc")
	if got {
		t.Error("expected false when webhook key is empty")
	}
}
