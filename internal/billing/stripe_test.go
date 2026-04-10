package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
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

// newTestStripeServer spins up an httptest server and returns a StripeClient
// rewired at it via the baseURL override. The handler lets each test assert
// over the requests it receives.
func newTestStripeServer(t *testing.T, handler http.HandlerFunc) (*StripeClient, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := NewStripeClient("sk_test", "whsec_test")
	client.baseURL = srv.URL
	return client, srv
}

func TestStripeClient_ReportUsage_Success(t *testing.T) {
	var (
		gotPath    string
		gotAction  string
		gotQty     string
		gotTime    string
		gotAuth    string
		gotContent string
	)
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContent = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("failed to parse body: %v", err)
		}
		gotAction = vals.Get("action")
		gotQty = vals.Get("quantity")
		gotTime = vals.Get("timestamp")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"mbur_1"}`))
	})

	ts := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := client.ReportUsage(context.Background(), "si_abc", 42, ts); err != nil {
		t.Fatalf("ReportUsage: %v", err)
	}

	if gotPath != "/subscription_items/si_abc/usage_records" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk_test" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotContent != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q", gotContent)
	}
	if gotAction != "increment" {
		t.Errorf("action = %q, want increment", gotAction)
	}
	if gotQty != "42" {
		t.Errorf("quantity = %q, want 42", gotQty)
	}
	if gotTime != fmt.Sprint(ts.Unix()) {
		t.Errorf("timestamp = %q, want %d", gotTime, ts.Unix())
	}
}

func TestStripeClient_ReportUsage_Validation(t *testing.T) {
	client := NewStripeClient("sk_test", "whsec_test")

	if err := client.ReportUsage(context.Background(), "", 10, time.Now()); err == nil {
		t.Error("expected error on empty subscription_item_id")
	}
	if err := client.ReportUsage(context.Background(), "si_1", -1, time.Now()); err == nil {
		t.Error("expected error on negative quantity")
	}
}

func TestStripeClient_ReportUsage_ZeroTimeDefaults(t *testing.T) {
	var gotTs string
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		gotTs = vals.Get("timestamp")
		w.WriteHeader(http.StatusOK)
	})

	if err := client.ReportUsage(context.Background(), "si_1", 1, time.Time{}); err != nil {
		t.Fatalf("ReportUsage: %v", err)
	}
	if gotTs == "" || gotTs == "0" {
		t.Errorf("expected ts to default to now, got %q", gotTs)
	}
}

func TestStripeClient_ReportUsage_APIError(t *testing.T) {
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid_request","type":"invalid_request_error"}}`))
	})

	err := client.ReportUsage(context.Background(), "si_1", 1, time.Now())
	if err == nil {
		t.Fatal("expected error from 400 response")
	}
	if !strings.Contains(err.Error(), "invalid_request") {
		t.Errorf("error should include Stripe message: %v", err)
	}
}

func TestStripeClient_APIError_FallbackToRawBody(t *testing.T) {
	// When the response isn't a structured Stripe error, we should still
	// surface the body snippet so operators can debug.
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream proxy rejected request"))
	})

	err := client.CancelSubscription(context.Background(), "sub_1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "upstream proxy") {
		t.Errorf("error should include body snippet: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("error should include status code: %v", err)
	}
}

func TestStripeClient_CreateCustomer_ParsesID(t *testing.T) {
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customers" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"cus_new_123"}`))
	})

	id, err := client.CreateCustomer(context.Background(), "u@x.com", "User", "tenant-1")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if id != "cus_new_123" {
		t.Errorf("id = %q", id)
	}
}

func TestStripeClient_CreateSubscription_ParsesID(t *testing.T) {
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"sub_new_1","status":"active"}`))
	})

	id, err := client.CreateSubscription(context.Background(), "cus_1", "price_1")
	if err != nil {
		t.Fatalf("CreateSubscription: %v", err)
	}
	if id != "sub_new_1" {
		t.Errorf("id = %q", id)
	}
}

func TestStripeClient_CreatePortalSession_ParsesURL(t *testing.T) {
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"url":"https://billing.stripe.com/p/session/abc"}`))
	})

	url, err := client.CreatePortalSession(context.Background(), "cus_1", "https://example.com/return")
	if err != nil {
		t.Fatalf("CreatePortalSession: %v", err)
	}
	if url != "https://billing.stripe.com/p/session/abc" {
		t.Errorf("url = %q", url)
	}
}

func TestStripeClient_DecodeErrorWrapped(t *testing.T) {
	// Server returns 200 with garbage JSON — client should surface a wrapped
	// decode error instead of silently returning empty values.
	client, _ := newTestStripeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json at all`))
	})

	_, err := client.CreateCustomer(context.Background(), "u@x.com", "U", "t1")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error should mention decode: %v", err)
	}
}

func TestStripeClient_APIBase_DefaultsToProduction(t *testing.T) {
	client := NewStripeClient("sk_test", "whsec_test")
	if client.apiBase() != stripeAPI {
		t.Errorf("apiBase() = %q, want %q", client.apiBase(), stripeAPI)
	}
}

func TestStripeClient_APIBase_OverrideRespected(t *testing.T) {
	client := NewStripeClient("sk_test", "whsec_test")
	client.baseURL = "http://127.0.0.1:1"
	if client.apiBase() != "http://127.0.0.1:1" {
		t.Errorf("apiBase() = %q, want override", client.apiBase())
	}
}
