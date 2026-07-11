package billing

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// StripeClient.post error paths

func TestStripeClientPost_RequestBuildError(t *testing.T) {
	c := &StripeClient{secretKey: "sk_test", client: &http.Client{Timeout: time.Second}}
	err := c.post(context.Background(), "\x00bad", url.Values{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStripeClientPost_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":{"message":"err"}}`))
	}))
	defer srv.Close()
	c := &StripeClient{baseURL: srv.URL, secretKey: "sk_test", client: &http.Client{Timeout: time.Second}}
	err := c.post(context.Background(), "/t", url.Values{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestStripeClientPost_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`bad json`))
	}))
	defer srv.Close()
	c := &StripeClient{baseURL: srv.URL, secretKey: "sk_test", client: &http.Client{Timeout: time.Second}}
	var dest struct{ ID string }
	err := c.post(context.Background(), "/t", url.Values{}, &dest)
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestStripeClientPost_NilDestOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer srv.Close()
	c := &StripeClient{baseURL: srv.URL, secretKey: "sk_test", client: &http.Client{Timeout: time.Second}}
	err := c.post(context.Background(), "/t", url.Values{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Webhook handler decode errors

func TestWebhookHandle_SubUpdated_DecodeErr(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`bad`)},
	}
	if err := h.handleSubscriptionUpdated(context.Background(), env); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebhookHandle_SubUpdated_EmptyID(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`{"id":""}`)},
	}
	if err := h.handleSubscriptionUpdated(context.Background(), env); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestWebhookHandle_SubCanceled_DecodeErr(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`bad`)},
	}
	if err := h.handleSubscriptionCanceled(context.Background(), env); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebhookHandle_InvoicePaid_DecodeErr(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`bad`)},
	}
	if err := h.handleInvoicePaid(context.Background(), env); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebhookHandle_InvoiceFailed_DecodeErr(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`bad`)},
	}
	if err := h.handleInvoicePaymentFailed(context.Background(), env); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebhookHandle_PaymentIntent_DecodeErr(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`bad`)},
	}
	if err := h.handlePaymentIntentSucceeded(context.Background(), env); err == nil {
		t.Fatal("expected error")
	}
}

func TestWebhookHandle_Checkout_DecodeErr(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	env := stripeEventEnvelope{Data: struct {
		Object json.RawMessage `json:"object"`
	}{Object: json.RawMessage(`bad`)},
	}
	if err := h.handleCheckoutCompleted(context.Background(), env); err == nil {
		t.Fatal("expected error")
	}
}

// emit tests

func TestWebhookHandle_Emit_NilEvents(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	h.emit(context.Background(), "evt", "t1", nil)
}

func TestWebhookHandle_Emit_WithEvents(t *testing.T) {
	bus := core.NewEventBus(slog.Default())
	h := &StripeEventHandler{logger: slog.Default(), events: bus}
	h.emit(context.Background(), "evt", "t1", map[string]any{"k": "v"})
}

// handleInvoicePaid — decode error test already above
