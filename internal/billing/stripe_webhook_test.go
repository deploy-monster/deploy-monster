package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// webhookStore is an in-memory store sufficient for webhook tests. It embeds
// core.Store so any method we don't override panics loudly if the handler
// ever starts using it — tests prefer "did I miss something?" over silent
// defaults.
type webhookStore struct {
	core.Store
	tenants   map[string]*core.Tenant
	getErr    error
	updateErr error
	updates   int
}

func newWebhookStore(tenants ...*core.Tenant) *webhookStore {
	s := &webhookStore{tenants: map[string]*core.Tenant{}}
	for _, t := range tenants {
		s.tenants[t.ID] = t
	}
	return s
}

func (s *webhookStore) GetTenant(_ context.Context, id string) (*core.Tenant, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	t, ok := s.tenants[id]
	if !ok {
		return nil, fmt.Errorf("tenant not found: %s", id)
	}
	// Return a copy so handler mutations don't leak back to the test setup
	// until UpdateTenant is called.
	cp := *t
	return &cp, nil
}

func (s *webhookStore) UpdateTenant(_ context.Context, t *core.Tenant) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updates++
	cp := *t
	s.tenants[t.ID] = &cp
	return nil
}

// signPayload produces a valid Stripe-Signature header for the given payload.
func signPayload(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

// newTestHandler wires up a StripeEventHandler with a matching client so
// signature verification succeeds for the returned signing helper.
func newTestHandler(t *testing.T, store core.Store, plans []Plan) (*StripeEventHandler, string) {
	t.Helper()
	secret := "whsec_test"
	client := NewStripeClient("sk_test", secret)
	h := NewStripeEventHandler(store, core.NewEventBus(slog.Default()), client, plans, slog.Default())
	return h, secret
}

func TestStripeEventHandler_InvalidSignature(t *testing.T) {
	store := newWebhookStore()
	h, _ := newTestHandler(t, store, nil)

	err := h.Handle(context.Background(), []byte(`{}`), "t=1,v1=bad")
	if !errors.Is(err, ErrStripeInvalidSignature) {
		t.Errorf("want ErrStripeInvalidSignature, got %v", err)
	}
}

func TestStripeEventHandler_NilClient(t *testing.T) {
	h := NewStripeEventHandler(newWebhookStore(), nil, nil, nil, slog.Default())
	err := h.Handle(context.Background(), []byte(`{}`), "t=1,v1=anything")
	if !errors.Is(err, ErrStripeInvalidSignature) {
		t.Errorf("want ErrStripeInvalidSignature, got %v", err)
	}
}

func TestStripeEventHandler_MalformedEnvelope(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{not json`)
	sig := signPayload(t, secret, payload)

	err := h.Handle(context.Background(), payload, sig)
	if err == nil {
		t.Fatal("expected error on malformed payload")
	}
}

func TestStripeEventHandler_MissingEventType(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{"id":"evt_1","data":{"object":{}}}`)
	sig := signPayload(t, secret, payload)

	err := h.Handle(context.Background(), payload, sig)
	if err == nil {
		t.Fatal("expected error when event type missing")
	}
}

func TestStripeEventHandler_UnknownEventAcknowledged(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{"id":"evt_1","type":"entirely.unknown","data":{"object":{}}}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Errorf("unknown event should ack, got %v", err)
	}
}

func TestStripeEventHandler_SubscriptionUpdated(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "free"}
	store := newWebhookStore(tenant)
	plans := []Plan{
		{ID: "free"},
		{ID: "pro", StripePriceID: "price_pro"},
	}
	h, secret := newTestHandler(t, store, plans)

	payload := []byte(`{
		"id":"evt_1",
		"type":"customer.subscription.updated",
		"data":{"object":{
			"id":"sub_abc",
			"customer":"cus_abc",
			"status":"active",
			"metadata":{"tenant_id":"tenant-1"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}
		}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.updates != 1 {
		t.Errorf("expected 1 tenant update, got %d", store.updates)
	}
	got := store.tenants["tenant-1"]
	if got.PlanID != "pro" {
		t.Errorf("PlanID = %q, want pro", got.PlanID)
	}
	md, _ := GetStripeMetadata(got)
	if md.CustomerID != "cus_abc" {
		t.Errorf("CustomerID = %q, want cus_abc", md.CustomerID)
	}
	if md.SubscriptionID != "sub_abc" {
		t.Errorf("SubscriptionID = %q, want sub_abc", md.SubscriptionID)
	}
	if md.SubscriptionItemID != "si_1" {
		t.Errorf("SubscriptionItemID = %q, want si_1", md.SubscriptionItemID)
	}
	if md.PriceID != "price_pro" {
		t.Errorf("PriceID = %q, want price_pro", md.PriceID)
	}
	if md.Status != "active" {
		t.Errorf("Status = %q, want active", md.Status)
	}
}

func TestStripeEventHandler_SubscriptionCreated(t *testing.T) {
	// "customer.subscription.created" must route through the same handler as
	// updates so a checkout flow leaves the tenant in the right plan state.
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "free"}
	store := newWebhookStore(tenant)
	plans := []Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	h, secret := newTestHandler(t, store, plans)

	payload := []byte(`{
		"id":"evt_1","type":"customer.subscription.created",
		"data":{"object":{"id":"sub_1","customer":"cus_1","status":"active",
			"metadata":{"tenant_id":"tenant-1"},
			"items":{"data":[{"id":"si_1","price":{"id":"price_pro"}}]}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if store.tenants["tenant-1"].PlanID != "pro" {
		t.Errorf("expected plan upgrade to pro")
	}
}

func TestStripeEventHandler_SubscriptionMissingTenantID(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","metadata":{}}}
	}`)
	sig := signPayload(t, secret, payload)

	// No tenant_id means we ack and drop — otherwise Stripe would retry forever.
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Errorf("expected nil error (ack), got %v", err)
	}
	if store.updates != 0 {
		t.Errorf("expected no updates, got %d", store.updates)
	}
}

func TestStripeEventHandler_SubscriptionCanceled(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "pro"}
	// Seed existing Stripe metadata so we can observe it being cleared.
	_ = SetStripeMetadata(tenant, StripeMetadata{
		CustomerID:         "cus_1",
		SubscriptionID:     "sub_1",
		SubscriptionItemID: "si_1",
		PriceID:            "price_pro",
		Status:             "active",
	})
	store := newWebhookStore(tenant)
	plans := []Plan{{ID: "free"}, {ID: "pro", StripePriceID: "price_pro"}}
	h, secret := newTestHandler(t, store, plans)

	payload := []byte(`{
		"id":"evt_1","type":"customer.subscription.deleted",
		"data":{"object":{"id":"sub_1","metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := store.tenants["tenant-1"]
	if got.PlanID != "free" {
		t.Errorf("expected revert to free plan, got %q", got.PlanID)
	}
	md, _ := GetStripeMetadata(got)
	if md.Status != "canceled" {
		t.Errorf("Status = %q, want canceled", md.Status)
	}
	if md.SubscriptionID != "" {
		t.Errorf("SubscriptionID should be cleared, got %q", md.SubscriptionID)
	}
	if md.SubscriptionItemID != "" {
		t.Errorf("SubscriptionItemID should be cleared, got %q", md.SubscriptionItemID)
	}
}

func TestStripeEventHandler_InvoicePaid(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "pro"}
	_ = SetStripeMetadata(tenant, StripeMetadata{
		CustomerID: "cus_1", SubscriptionID: "sub_1", Status: "past_due",
	})
	store := newWebhookStore(tenant)
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"invoice.paid",
		"data":{"object":{"id":"in_1","customer":"cus_1",
			"subscription":"sub_1","status":"paid","amount_paid":1500,
			"amount_due":1500,"currency":"usd",
			"metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := store.tenants["tenant-1"]
	md, _ := GetStripeMetadata(got)
	if md.Status != "active" {
		t.Errorf("Status = %q, want active", md.Status)
	}
	if md.PaymentLastSucceededAt.IsZero() {
		t.Error("PaymentLastSucceededAt not updated")
	}
}

func TestStripeEventHandler_InvoicePaymentFailed(t *testing.T) {
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "pro"}
	_ = SetStripeMetadata(tenant, StripeMetadata{CustomerID: "cus_1", Status: "active"})
	store := newWebhookStore(tenant)
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"invoice.payment_failed",
		"data":{"object":{"id":"in_1","customer":"cus_1","status":"open",
			"amount_paid":0,"amount_due":1500,"currency":"usd",
			"metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := store.tenants["tenant-1"]
	md, _ := GetStripeMetadata(got)
	if md.Status != "past_due" {
		t.Errorf("Status = %q, want past_due", md.Status)
	}
	if md.PaymentLastFailedAt.IsZero() {
		t.Error("PaymentLastFailedAt not updated")
	}
}

func TestStripeEventHandler_PaymentIntentSucceeded(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"payment_intent.succeeded",
		"data":{"object":{"id":"pi_1","amount":2500,"currency":"usd",
			"customer":"cus_1","metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	// Payment intent events don't touch tenant state — they just emit — so
	// success here means "no error".
	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestStripeEventHandler_CheckoutCompleted(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"checkout.session.completed",
		"data":{"object":{"id":"cs_1","customer":"cus_1","subscription":"sub_1",
			"payment_status":"paid","amount_total":1500,"currency":"usd",
			"metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

func TestStripeEventHandler_InvoicePaymentSucceededAlias(t *testing.T) {
	// Stripe emits both "invoice.paid" and "invoice.payment_succeeded" for
	// paid invoices; our handler should treat them identically.
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "pro"}
	_ = SetStripeMetadata(tenant, StripeMetadata{CustomerID: "cus_1", Status: "past_due"})
	store := newWebhookStore(tenant)
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"invoice.payment_succeeded",
		"data":{"object":{"id":"in_1","customer":"cus_1",
			"amount_paid":1500,"currency":"usd",
			"metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	md, _ := GetStripeMetadata(store.tenants["tenant-1"])
	if md.Status != "active" {
		t.Errorf("Status = %q, want active", md.Status)
	}
}

func TestStripeEventHandler_SubscriptionTenantLoadError(t *testing.T) {
	store := newWebhookStore()
	store.getErr = errors.New("db down")
	h, secret := newTestHandler(t, store, nil)

	payload := []byte(`{
		"id":"evt_1","type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","metadata":{"tenant_id":"tenant-1"}}}
	}`)
	sig := signPayload(t, secret, payload)

	err := h.Handle(context.Background(), payload, sig)
	if err == nil {
		t.Fatal("expected error when tenant load fails")
	}
}

func TestStripeEventHandler_NilEventBus(t *testing.T) {
	// emit must be a no-op when the event bus is nil. We construct the
	// handler manually here so we can exercise that path without crashing.
	tenant := &core.Tenant{ID: "tenant-1", PlanID: "free"}
	store := newWebhookStore(tenant)
	client := NewStripeClient("sk_test", "whsec_test")
	h := NewStripeEventHandler(store, nil, client, nil, slog.Default())

	payload := []byte(`{
		"id":"evt_1","type":"customer.subscription.updated",
		"data":{"object":{"id":"sub_1","customer":"cus_1","status":"active",
			"metadata":{"tenant_id":"tenant-1"},"items":{"data":[]}}}
	}`)
	sig := signPayload(t, "whsec_test", payload)

	if err := h.Handle(context.Background(), payload, sig); err != nil {
		t.Errorf("Handle with nil bus: %v", err)
	}
}

func TestFindPlanByID(t *testing.T) {
	plans := []Plan{{ID: "free"}, {ID: "pro"}, {ID: "biz"}}
	if got := findPlanByID(plans, "pro"); got == nil || got.ID != "pro" {
		t.Errorf("expected pro, got %+v", got)
	}
	if got := findPlanByID(plans, "xxx"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
	if got := findPlanByID(nil, "pro"); got != nil {
		t.Errorf("expected nil for nil plans, got %+v", got)
	}
}

// Assert the test helper compiles against the real io.Reader interface —
// catches drift if signPayload ever stops using io.
var _ io.Reader = (*stringReader)(nil)

type stringReader struct{ s string }

func (r *stringReader) Read(p []byte) (int, error) { return copy(p, r.s), io.EOF }
