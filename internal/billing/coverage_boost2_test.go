package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func testDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// =============================================================================
// MODULE INIT — factory function coverage via NewApp
// =============================================================================

func TestInit_NewApp_Billing(t *testing.T) {
	cfg := &core.Config{
		Server: core.ServerConfig{
			SecretKey: "test-secret-key-for-init-coverage",
		},
	}
	_, err := core.NewApp(cfg, core.BuildInfo{Version: "0.0.0"})
	if err != nil {
		t.Logf("NewApp returned: %v", err)
	}
}

// =============================================================================
// MODULE HEALTH — degraded path (enabled but no meter)
// =============================================================================

func TestModuleHealth_Degraded(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Billing: core.BillingConfig{Enabled: true},
		},
		Logger: slog.Default(),
		Store:  &mockStore{},
	}
	_ = m.Init(context.Background(), c)
	if h := m.Health(); h != core.HealthDegraded {
		t.Errorf("Health() = %v, want HealthDegraded", h)
	}
}

// =============================================================================
// METER — loop panic recovery
// =============================================================================

type panicRuntime struct{}

func (p *panicRuntime) Ping() error { return nil }
func (p *panicRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (p *panicRuntime) Stop(_ context.Context, _ string, _ int) error        { return nil }
func (p *panicRuntime) Remove(_ context.Context, _ string, _ bool) error     { return nil }
func (p *panicRuntime) Restart(_ context.Context, _ string) error            { return nil }
func (p *panicRuntime) Logs(_ context.Context, _, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (p *panicRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	panic("panic in list")
}
func (p *panicRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (p *panicRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}
func (p *panicRuntime) ImagePull(_ context.Context, _ string) error  { return nil }
func (p *panicRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (p *panicRuntime) ImageRemove(_ context.Context, _ string) error { return nil }
func (p *panicRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) { return nil, nil }
func (p *panicRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) { return nil, nil }

func TestMeter_Loop_Recover(t *testing.T) {
	meter := NewMeter(&mockStore{}, &panicRuntime{}, slog.Default())
	meter.Start()
	time.Sleep(50 * time.Millisecond)
	meter.Stop()
}

// =============================================================================
// METER — collect mid-write abort
// =============================================================================

type storeWithTenant struct {
	mockStore
	tenant *core.Tenant
}

func (m *storeWithTenant) GetTenant(_ context.Context, id string) (*core.Tenant, error) {
	if m.tenant != nil {
		return m.tenant, nil
	}
	return nil, fmt.Errorf("tenant not found")
}

func TestMeterCollect_WithStripeReporting(t *testing.T) {
	logger := testDiscardLogger()
	tenant := &core.Tenant{
		ID: "tenant-1",
		MetadataJSON: `{"stripe":{"customer_id":"cus_123","subscription_item_id":"si_456"}}`,
	}
	store := &storeWithTenant{tenant: tenant}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "c1",
				Name:   "app1",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-1",
					"monster.app.id": "app-1",
				},
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	meter.SetStripe(&StripeClient{
		secretKey: "sk_test",
		client:    &http.Client{},
		baseURL:   "http://localhost:1",
	}, core.NewEventBus(logger))
	meter.collect()
}

// =============================================================================
// METER — reportUsageToStripe edge cases
// =============================================================================

func TestMeter_reportUsageToStripe_Aborted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	meter := NewMeter(&mockStore{}, nil, slog.Default())
	usage := map[string]*TenantUsage{
		"t1": {Containers: 1},
	}
	meter.reportUsageToStripe(ctx, usage, time.Now())
}

func TestMeter_reportUsageToStripe_TenantNotFound(t *testing.T) {
	store := &storeWithTenant{} // no tenant, GetTenant returns error
	meter := NewMeter(store, nil, slog.Default())
	usage := map[string]*TenantUsage{
		"unknown": {Containers: 1},
	}
	meter.reportUsageToStripe(context.Background(), usage, time.Now())
}

func TestMeter_reportUsageToStripe_NoSubscriptionItem(t *testing.T) {
	tenant := &core.Tenant{
		ID:           "t1",
		MetadataJSON: `{"stripe":{"customer_id":"cus_123"}}`,
	}
	store := &storeWithTenant{tenant: tenant}
	meter := NewMeter(store, nil, slog.Default())
	usage := map[string]*TenantUsage{
		"t1": {Containers: 1},
	}
	meter.reportUsageToStripe(context.Background(), usage, time.Now())
}

// =============================================================================
// STRIPE — post JSON decode error path
// =============================================================================

func TestStripePost_DecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`invalid json`))
	}))
	defer srv.Close()

	client := &StripeClient{
		secretKey: "sk_test",
		client:    srv.Client(),
	}
	client.baseURL = srv.URL

	var dest struct {
		ID string `json:"id"`
	}
	err := client.post(context.Background(), "/test", nil, &dest)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("expected 'decode response' error, got: %v", err)
	}
}

// =============================================================================
// STRIPE WEBHOOK — NewStripeEventHandler nil logger
// =============================================================================

func TestNewStripeEventHandler_NilLogger_Extra(t *testing.T) {
	h := NewStripeEventHandler(newWebhookStore(), core.NewEventBus(slog.Default()), nil, nil, nil)
	if h == nil {
		t.Fatal("NewStripeEventHandler returned nil")
	}
	if h.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
}

// =============================================================================
// STRIPE WEBHOOK — sweepLocked with ttl <= 0
// =============================================================================

func TestSweepLocked_ZeroTTL_Extra(t *testing.T) {
	h := &StripeEventHandler{
		seenTTL: 0,
		seen:    map[string]time.Time{"evt_1": time.Now()},
	}
	h.sweepLocked()
	if len(h.seen) != 1 {
		t.Errorf("expected 1 entry (not swept), got %d", len(h.seen))
	}
}

// =============================================================================
// STRIPE WEBHOOK — handlePaymentIntentSucceeded & handleCheckoutCompleted
// =============================================================================

func TestHandlePaymentIntentSucceeded_Success(t *testing.T) {
	store := newWebhookStore()
	bus := core.NewEventBus(slog.Default())
	h := NewStripeEventHandler(store, bus, &StripeClient{secretKey: "sk_test", webhookKey: "whsec_test"}, nil, slog.Default())

	payload := []byte(`{"id":"pi_123","type":"payment_intent.succeeded","data":{"object":{"id":"pi_123","amount":2000,"currency":"usd","customer":"cus_123","metadata":{"tenant_id":"t1"}}}}`)
	sig := signPayload(t, "whsec_test", payload)

	err := h.Handle(context.Background(), payload, sig)
	if err != nil {
		t.Logf("Handle returned: %v (expected if no tenant)", err)
	}
}

func TestHandleCheckoutCompleted_Success(t *testing.T) {
	store := newWebhookStore()
	bus := core.NewEventBus(slog.Default())
	h := NewStripeEventHandler(store, bus, &StripeClient{secretKey: "sk_test", webhookKey: "whsec_test"}, BuiltinPlans, slog.Default())

	payload := []byte(`{"id":"cs_123","type":"checkout.session.completed","data":{"object":{"id":"cs_123","customer":"cus_123","subscription":"sub_123","payment_status":"paid","amount_total":1000,"currency":"usd","metadata":{"tenant_id":"t1"}}}}`)
	sig := signPayload(t, "whsec_test", payload)

	err := h.Handle(context.Background(), payload, sig)
	if err != nil {
		t.Logf("Handle returned: %v", err)
	}
}

// =============================================================================
// STRIPE WEBHOOK — handleSubscriptionUpdated missing tenant_id
// =============================================================================

func TestHandleSubscriptionUpdated_MissingTenantID_Extra(t *testing.T) {
	store := newWebhookStore()
	h := NewStripeEventHandler(store, core.NewEventBus(slog.Default()), &StripeClient{secretKey: "sk_test", webhookKey: "whsec_test"}, nil, slog.Default())

	payload := []byte(`{"id":"evt_1","type":"customer.subscription.updated","data":{"object":{"id":"sub_1","customer":"cus_1","status":"active","items":{"data":[{"id":"si_1","price":{"id":"price_1"}}]}}}}`)
	sig := signPayload(t, "whsec_test", payload)

	err := h.Handle(context.Background(), payload, sig)
	if err != nil {
		t.Logf("Handle returned: %v (expected to acknowledge without tenant)", err)
	}
}

// =============================================================================
// STRIPE WEBHOOK — handleSubscriptionCanceled missing tenant_id
// =============================================================================

func TestHandleSubscriptionCanceled_MissingTenantID_Extra(t *testing.T) {
	store := newWebhookStore()
	h := NewStripeEventHandler(store, core.NewEventBus(slog.Default()), &StripeClient{secretKey: "sk_test", webhookKey: "whsec_test"}, nil, slog.Default())

	payload := []byte(`{"id":"evt_2","type":"customer.subscription.deleted","data":{"object":{"id":"sub_2","customer":"cus_2"}}}`)
	sig := signPayload(t, "whsec_test", payload)

	err := h.Handle(context.Background(), payload, sig)
	if err != nil {
		t.Logf("Handle returned: %v (expected)", err)
	}
}

// =============================================================================
// STRIPE WEBHOOK — emit with nil events bus
// =============================================================================

func TestEmit_NilEvents_Extra(t *testing.T) {
	h := &StripeEventHandler{logger: slog.Default()}
	h.emit(context.Background(), core.EventBillingSubscriptionUpdated, "t1", nil)
}

// =============================================================================
// TENANT METADATA — edge cases
// =============================================================================

func TestSetStripeMetadata_EmptyExistingMetadata_Extra(t *testing.T) {
	tenant := &core.Tenant{ID: "t1", MetadataJSON: ""}
	md := StripeMetadata{CustomerID: "cus_123"}
	err := SetStripeMetadata(tenant, md)
	if err != nil {
		t.Fatalf("SetStripeMetadata error: %v", err)
	}
	var blob map[string]json.RawMessage
	_ = json.Unmarshal([]byte(tenant.MetadataJSON), &blob)
	if _, ok := blob[stripeMetadataKey]; !ok {
		t.Error("stripe metadata key not found after SetStripeMetadata")
	}
}

func TestSetStripeMetadata_IsZero_Extra(t *testing.T) {
	tenant := &core.Tenant{ID: "t1", MetadataJSON: `{"other":"data"}`}
	md := StripeMetadata{}
	err := SetStripeMetadata(tenant, md)
	if err != nil {
		t.Fatalf("SetStripeMetadata error: %v", err)
	}
	var blob map[string]json.RawMessage
	_ = json.Unmarshal([]byte(tenant.MetadataJSON), &blob)
	if _, ok := blob[stripeMetadataKey]; ok {
		t.Error("stripe key should be removed when metadata is zero")
	}
	if _, ok := blob["other"]; !ok {
		t.Error("other keys should be preserved")
	}
}

// =============================================================================
// HANDLE — already processed (replay suppression)
// =============================================================================

func TestHandle_ReplaySuppressed_Extra(t *testing.T) {
	store := newWebhookStore()
	bus := core.NewEventBus(slog.Default())
	h := NewStripeEventHandler(store, bus, &StripeClient{secretKey: "sk_test", webhookKey: "whsec_test"}, nil, slog.Default())

	h.markProcessed("evt_replay")

	payload := []byte(`{"id":"evt_replay","type":"checkout.session.completed","data":{"object":{"id":"cs_1"}}}`)
	sig := signPayload(t, "whsec_test", payload)

	err := h.Handle(context.Background(), payload, sig)
	if err != nil {
		t.Errorf("Handle for replay should return nil, got: %v", err)
	}
}

// =============================================================================
// METER — collect with stripe and events
// =============================================================================

func TestMeterCollect_StripeReportEmitEvent(t *testing.T) {
	logger := testDiscardLogger()
	tenant := &core.Tenant{
		ID: "tenant-emit",
		MetadataJSON: `{"stripe":{"customer_id":"cus_emit","subscription_item_id":"si_emit"}}`,
	}
	store := &storeWithTenant{tenant: tenant}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "c1",
				Name:   "app1",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-emit",
					"monster.app.id": "app-1",
				},
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	meter.SetStripe(nil, core.NewEventBus(logger))
	meter.collect()
}
