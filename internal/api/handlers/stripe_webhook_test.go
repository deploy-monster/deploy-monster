package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/billing"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// oversizedReader produces infinite bytes — used to exceed the webhook body
// limit without allocating the full payload up front.
type oversizedReader struct{ remaining int }

func (r *oversizedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	r.remaining -= n
	return n, nil
}

// stripeStub is the minimal store used by these HTTP tests. The billing event
// handler only calls GetTenant / UpdateTenant for subscription events; for
// tests that don't exercise subscription flows we can return errors.
type stripeStub struct {
	core.Store
	tenants map[string]*core.Tenant
}

func (s *stripeStub) GetTenant(_ context.Context, id string) (*core.Tenant, error) {
	t, ok := s.tenants[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *t
	return &cp, nil
}

func (s *stripeStub) UpdateTenant(_ context.Context, t *core.Tenant) error {
	s.tenants[t.ID] = t
	return nil
}

func signStripe(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func newStripeWebhookTestHandler(t *testing.T) (*StripeWebhookHandler, string, *stripeStub) {
	t.Helper()
	secret := "whsec_test"
	store := &stripeStub{tenants: map[string]*core.Tenant{
		"tenant-1": {ID: "tenant-1", PlanID: "free"},
	}}
	client := billing.NewStripeClient("sk_test", secret)
	events := core.NewEventBus(slog.Default())
	plans := []billing.Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	eventHandler := billing.NewStripeEventHandler(store, events, client, plans, slog.Default())
	return NewStripeWebhookHandler(eventHandler, slog.Default()), secret, store
}

func TestStripeWebhookHandler_NilEventsReturns503(t *testing.T) {
	h := NewStripeWebhookHandler(nil, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(`{}`))
	req.Header.Set("Stripe-Signature", "t=1,v1=abc")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rr.Code)
	}
}

func TestStripeWebhookHandler_MissingSignature(t *testing.T) {
	h, _, _ := newStripeWebhookTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Stripe-Signature") {
		t.Errorf("body should mention missing header: %s", rr.Body.String())
	}
}

func TestStripeWebhookHandler_InvalidSignature(t *testing.T) {
	h, _, _ := newStripeWebhookTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", strings.NewReader(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{}}}`))
	req.Header.Set("Stripe-Signature", "t=1,v1=0000")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestStripeWebhookHandler_ValidCheckoutCompleted(t *testing.T) {
	h, secret, _ := newStripeWebhookTestHandler(t)

	payload := []byte(`{
		"id":"evt_1","type":"checkout.session.completed",
		"data":{"object":{"id":"cs_1","customer":"cus_1","subscription":"sub_1",
			"payment_status":"paid","amount_total":1500,"currency":"usd",
			"metadata":{"tenant_id":"tenant-1"}}}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signStripe(t, secret, payload))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "received") {
		t.Errorf("body should confirm receipt: %s", rr.Body.String())
	}
}

func TestStripeWebhookHandler_ValidSubscriptionUpdated(t *testing.T) {
	h, secret, store := newStripeWebhookTestHandler(t)

	payload := []byte(`{
		"id":"evt_1","type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","customer":"cus_1","status":"active",
			"metadata":{"tenant_id":"tenant-1"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}}}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signStripe(t, secret, payload))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if store.tenants["tenant-1"].PlanID != "pro" {
		t.Errorf("expected plan upgrade, got %q", store.tenants["tenant-1"].PlanID)
	}
}

func TestStripeWebhookHandler_MalformedBodyReturns500(t *testing.T) {
	h, secret, _ := newStripeWebhookTestHandler(t)

	// Signature is valid for the payload, but the payload itself is malformed
	// — the billing handler returns a generic decode error, which the HTTP
	// adapter maps to 500 so Stripe retries delivery.
	payload := []byte(`{not valid json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewReader(payload))
	req.Header.Set("Stripe-Signature", signStripe(t, secret, payload))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rr.Code)
	}
}

func TestStripeWebhookHandler_OversizedBodyReturns413(t *testing.T) {
	h, _, _ := newStripeWebhookTestHandler(t)

	// Craft a body larger than the limit. The handler should reject before
	// attempting signature verification.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe",
		&oversizedReader{remaining: maxStripeWebhookBody + 1024})
	req.Header.Set("Stripe-Signature", "t=1,v1=abc")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rr.Code)
	}
}

func TestStripeWebhookHandler_NilLoggerDefaults(t *testing.T) {
	// Passing nil logger should not panic — handler defaults to slog.Default.
	events := billing.NewStripeEventHandler(
		&stripeStub{tenants: map[string]*core.Tenant{}},
		core.NewEventBus(slog.Default()),
		billing.NewStripeClient("sk_test", "whsec_test"),
		nil, slog.Default(),
	)
	h := NewStripeWebhookHandler(events, nil)
	if h == nil || h.logger == nil {
		t.Fatal("expected handler with defaulted logger")
	}
}
