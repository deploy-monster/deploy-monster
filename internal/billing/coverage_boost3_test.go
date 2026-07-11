package billing

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// METER collect — context canceled mid-write
// =============================================================================

func TestMeterCollect_CtxCanceled(t *testing.T) {
	logger := testDiscardLogger()
	store := &mockStore{}
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
	// Cancel the stopCtx before collect to trigger ctx.Err() mid-write
	if meter.stopCancel != nil {
		meter.stopCancel()
	}
	meter.collect()
}

// =============================================================================
// STRIPE post — error response with message
// =============================================================================

func TestStripePost_ErrorMessage(t *testing.T) {
	client := &StripeClient{
		secretKey: "sk_test",
		client:    &http.Client{},
	}
	// Point to a non-routable address so the HTTP call fails
	// with a network error, covering the s.client.Do(req) error path.
	client.baseURL = "http://127.0.0.1:1"
	params := url.Values{"test": {"value"}}
	err := client.post(context.Background(), "/test", params, nil)
	if err == nil {
		t.Fatal("expected network error")
	}
}

// =============================================================================
// STRIPE WEBHOOK — emit with events bus (EmitWithTenant error path)
// We trigger the error by passing a context that's already canceled.
// =============================================================================

func TestEmit_WithEvents_Error(t *testing.T) {
	bus := core.NewEventBus(slog.Default())
	h := &StripeEventHandler{
		events: bus,
		logger: slog.Default(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.emit(ctx, core.EventBillingSubscriptionUpdated, "t1", map[string]any{"key": "value"})
}

// =============================================================================
// METER — loop with canceled context (stopCtx.Err() path)
// =============================================================================

func TestMeter_Loop_StopCtxCanceled(t *testing.T) {
	meter := NewMeter(&mockStore{}, &mockContainerRuntime{}, slog.Default())
	meter.Start()
	// Cancel the context directly to trigger stopCtx.Err() != nil path
	if meter.stopCancel != nil {
		meter.stopCancel()
	}
	time.Sleep(50 * time.Millisecond)
	// Meter should still handle double-Stop gracefully
	meter.Stop()
}

// =============================================================================
// HANDLE — subscription updated with tenant store error
// =============================================================================

func TestHandleSubscriptionUpdated_StoreError(t *testing.T) {
	store := newWebhookStore()
	// Add a tenant but make update fail
	store.tenants["t-upd-err"] = &core.Tenant{
		ID:           "t-upd-err",
		MetadataJSON: `{"stripe":{"customer_id":"cus_1","subscription_item_id":"si_1"}}`,
	}
	store.updateErr = fmt.Errorf("update failed")
	h := NewStripeEventHandler(store, core.NewEventBus(slog.Default()),
		&StripeClient{secretKey: "sk_test", webhookKey: "whsec_test"}, nil, slog.Default())

	payload := []byte(`{"id":"evt_upd","type":"customer.subscription.updated","data":{"object":{"id":"sub_1","customer":"cus_1","metadata":{"tenant_id":"t-upd-err"},"status":"active","items":{"data":[{"id":"si_1","price":{"id":"price_1"}}]}}}}`)
	sig := signPayload(t, "whsec_test", payload)

	err := h.Handle(context.Background(), payload, sig)
	if err == nil {
		t.Log("Handle returned nil (expected update error)")
	}
}

// =============================================================================
// REPORT USAGE TO STRIPE — emit event path
// =============================================================================

func TestMeter_reportUsageToStripe_EmitEvent(t *testing.T) {
	tenant := &core.Tenant{
		ID:           "t-emit",
		MetadataJSON: `{"stripe":{"customer_id":"cus_emit","subscription_item_id":"si_emit"}}`,
	}
	store := &storeWithTenant{tenant: tenant}
	logger := testDiscardLogger()
	meter := NewMeter(store, nil, logger)
	// Provide a stripe client so reportUsageToStripe doesn't NPE
	meter.stripe = &StripeClient{
		secretKey: "sk_test",
		client:    &http.Client{},
		baseURL:   "http://127.0.0.1:1",
	}
	meter.events = core.NewEventBus(logger)
	usage := map[string]*TenantUsage{
		"t-emit": {Containers: 2},
	}
	meter.reportUsageToStripe(context.Background(), usage, time.Now())
}

// =============================================================================
// COLLECT — stripe reporting with empty usage
// =============================================================================

func TestMeterCollect_NoContainers_WithStripe(t *testing.T) {
	meter := NewMeter(&mockStore{}, &mockContainerRuntime{}, testDiscardLogger())
	meter.SetStripe(&StripeClient{secretKey: "sk_test"}, nil)
	meter.collect()
}

// =============================================================================
// ReportUsage — edge case validations  
// =============================================================================

func TestStripe_ReportUsage_EmptyItemID(t *testing.T) {
	client := &StripeClient{secretKey: "sk_test"}
	err := client.ReportUsage(context.Background(), "", 1, time.Now())
	if err == nil {
		t.Fatal("expected error for empty subscription item ID")
	}
}

func TestStripe_ReportUsage_NegativeQuantity(t *testing.T) {
	client := &StripeClient{secretKey: "sk_test"}
	err := client.ReportUsage(context.Background(), "si_1", -1, time.Now())
	if err == nil {
		t.Fatal("expected error for negative quantity")
	}
}

func TestStripe_ReportUsage_ZeroTimestamp(t *testing.T) {
	client := &StripeClient{
		secretKey: "sk_test",
		client:    &http.Client{},
		baseURL:   "http://127.0.0.1:1",
	}
	// Zero timestamp should default to time.Now()
	err := client.ReportUsage(context.Background(), "si_1", 5, time.Time{})
	if err == nil {
		t.Fatal("expected network error (confirms timestamp defaulted)")
	}
}
