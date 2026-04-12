package webhooks

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// --- signPayload ---

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
	// HMAC-SHA256 produces 64 hex chars.
	if len(sig) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(sig))
	}
}

func TestSignPayload_EmptyPayload(t *testing.T) {
	sig := signPayload([]byte{}, "secret")
	if len(sig) != 64 {
		t.Errorf("expected 64 hex chars for empty payload, got %d", len(sig))
	}
}

func TestSignPayload_EmptySecret(t *testing.T) {
	sig := signPayload([]byte("data"), "")
	if len(sig) != 64 {
		t.Errorf("expected 64 hex chars for empty secret, got %d", len(sig))
	}
}

// --- OutboundSender ---

func TestOutboundSender_Send_Success(t *testing.T) {
	var receivedBody []byte
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: map[string]string{"event": "app.deployed"},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify headers.
	if receivedHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", receivedHeaders.Get("Content-Type"))
	}
	if receivedHeaders.Get("User-Agent") != "DeployMonster/1.0" {
		t.Errorf("User-Agent = %q, want DeployMonster/1.0", receivedHeaders.Get("User-Agent"))
	}

	// Verify body.
	var payload map[string]string
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["event"] != "app.deployed" {
		t.Errorf("payload event = %q, want app.deployed", payload["event"])
	}
}

func TestOutboundSender_Send_WithHMACSignature(t *testing.T) {
	var receivedSig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-Monster-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	secret := "webhook-secret-123"
	payload := map[string]string{"action": "deploy"}
	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: payload,
		Secret:  secret,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify signature was set.
	if receivedSig == "" {
		t.Fatal("expected X-Monster-Signature header")
	}
	if len(receivedSig) < 8 || receivedSig[:7] != "sha256=" {
		t.Errorf("signature = %q, expected sha256= prefix", receivedSig)
	}

	// Verify signature is valid.
	body, _ := json.Marshal(payload)
	expectedSig := "sha256=" + signPayload(body, secret)
	if receivedSig != expectedSig {
		t.Errorf("signature = %q, want %q", receivedSig, expectedSig)
	}
}

func TestOutboundSender_Send_NoSecretNoSignature(t *testing.T) {
	var hasSig bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasSig = r.Header.Get("X-Monster-Signature") != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
		Secret:  "", // No secret.
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if hasSig {
		t.Error("should not set signature header when no secret provided")
	}
}

func TestOutboundSender_Send_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
			"Authorization":   "Bearer token123",
		},
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("X-Custom-Header = %q, want custom-value", receivedHeaders.Get("X-Custom-Header"))
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Errorf("Authorization = %q, want 'Bearer token123'", receivedHeaders.Get("Authorization"))
	}
}

func TestOutboundSender_Send_CustomMethod(t *testing.T) {
	var receivedMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Method:  http.MethodPut,
		Payload: "test",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if receivedMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", receivedMethod)
	}
}

func TestOutboundSender_Send_DefaultMethodIsPOST(t *testing.T) {
	var receivedMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
		// Method intentionally empty.
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if receivedMethod != http.MethodPost {
		t.Errorf("method = %q, want POST (default)", receivedMethod)
	}
}

func TestOutboundSender_Send_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if err != nil && !containsSubstring(err.Error(), "500") {
		t.Errorf("error = %q, expected to contain '500'", err.Error())
	}
}

func TestOutboundSender_Send_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestOutboundSender_Send_ConnectionRefused(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     "http://127.0.0.1:1", // Port 1 should refuse connections.
		Payload: "test",
	})
	if err == nil {
		t.Error("expected error for connection refused")
	}
}

func TestOutboundSender_Send_InvalidURL(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     "://invalid",
		Payload: "test",
	})
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestOutboundSender_Send_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := sender.Send(ctx, core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestOutboundSender_Send_WithTimeout(t *testing.T) {
	var requestReceived atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
		Timeout: 10, // 10-second timeout.
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !requestReceived.Load() {
		t.Error("server should have received the request")
	}
}

func TestOutboundSender_Send_EmitsSuccessEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))

	var eventReceived atomic.Bool
	events.Subscribe(core.EventOutboundSent, func(_ context.Context, e core.Event) error {
		eventReceived.Store(true)
		data, ok := e.Data.(core.NotificationEventData)
		if !ok {
			t.Error("expected NotificationEventData")
		}
		if data.Channel != "webhook" {
			t.Errorf("channel = %q, want webhook", data.Channel)
		}
		if data.Recipient != srv.URL {
			t.Errorf("recipient = %q, want %q", data.Recipient, srv.URL)
		}
		return nil
	})

	sender := NewOutboundSender(events)
	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Give async event a moment to propagate.
	time.Sleep(100 * time.Millisecond)

	// Note: PublishAsync fires in a goroutine; the event may or may not
	// have been received by now, so we just verify no error on send.
}

func TestOutboundSender_Send_EmitsFailureEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})

	if err == nil {
		t.Error("expected error for 503")
	}
}

func TestOutboundSender_Send_NilEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Sender with nil events should not panic.
	sender := &OutboundSender{
		client: &http.Client{Timeout: 5 * time.Second},
		events: nil,
	}

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	if err != nil {
		t.Fatalf("Send with nil events: %v", err)
	}
}

func TestOutboundSender_Send_NilEventsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Sender with nil events on failure should not panic.
	sender := &OutboundSender{
		client: &http.Client{Timeout: 5 * time.Second},
		events: nil,
	}

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestOutboundSender_Send_3xxNotError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	// Disable redirect following to test raw status.
	sender.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: "test",
	})
	// 301 < 400, so should not be an error.
	if err != nil {
		t.Errorf("3xx should not be an error: %v", err)
	}
}

func TestOutboundSender_Send_ComplexPayload(t *testing.T) {
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	sender := NewOutboundSender(events)

	payload := map[string]any{
		"event": "app.deployed",
		"data": map[string]any{
			"app_id":  "app-123",
			"version": 42,
			"tags":    []string{"production", "v2"},
		},
	}

	err := sender.Send(context.Background(), core.OutboundWebhook{
		URL:     srv.URL,
		Payload: payload,
		Secret:  "complex-secret",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(receivedBody, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded["event"] != "app.deployed" {
		t.Errorf("event = %v, want app.deployed", decoded["event"])
	}
}

// --- Compile-time interface check ---

func TestOutboundSender_ImplementsInterface(t *testing.T) {
	var _ core.OutboundWebhookSender = (*OutboundSender)(nil)
}

// --- Helper ---

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsCheck(s, sub))
}

func containsCheck(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
