package billing

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// metering.go — loop context cancellation (loop exit via stopCh)
// =============================================================================

func TestMeter_Loop_Stop(t *testing.T) {
	m := NewMeter(nil, nil, slog.Default())
	m.Start()
	m.Stop()
}

func TestMeter_Loop_NilStopCtx(t *testing.T) {
	m := &Meter{
		stopCh: make(chan struct{}),
		logger: slog.Default(),
	}
	m.Start()
	m.Stop()
}

func TestMeter_Start_AlreadyStopped(t *testing.T) {
	m := NewMeter(nil, nil, slog.Default())
	m.Stop()
	m.Start() // Should be no-op
}

func TestMeter_Stop_Idempotent(t *testing.T) {
	m := NewMeter(nil, nil, slog.Default())
	m.Stop()
	m.Stop() // Should not panic
}

func TestMeter_SetStripe_Nil(t *testing.T) {
	m := NewMeter(nil, nil, slog.Default())
	m.SetStripe(nil, nil) // Should not panic
}

// =============================================================================
// metering.go — NewMeter with nil logger
// =============================================================================

func TestNewMeter_NilLogger(t *testing.T) {
	m := NewMeter(nil, nil, nil)
	if m == nil {
		t.Fatal("expected non-nil meter")
	}
}

// =============================================================================
// metering.go — collect with nil runtime (early return)
// =============================================================================

func TestMeter_Collect_NilRuntime(t *testing.T) {
	m := NewMeter(nil, nil, slog.Default())
	m.collect() // Should not panic, early return for nil runtime
}

// =============================================================================
// metering.go — reportUsageToStripe empty tenantUsage
// =============================================================================

func TestMeter_ReportUsageToStripe_Empty(t *testing.T) {
	m := NewMeter(nil, nil, slog.Default())
	m.reportUsageToStripe(context.Background(), map[string]*TenantUsage{}, time.Now())
}

// =============================================================================
// stripe_webhook.go — emit with nil events bus
// =============================================================================

func TestStripeEventHandler_Emit_NilEvents(t *testing.T) {
	h := &StripeEventHandler{
		events: nil,
		logger: slog.Default(),
		now:    func() time.Time { return time.Now().UTC() },
	}
	// Should not panic
	h.emit(context.Background(), core.EventBillingSubscriptionUpdated, "", nil)
}

// =============================================================================
// stripe_webhook.go — resolveTenantID edge case
// =============================================================================

func TestStripeEventHandler_ResolveTenantID_Empty(t *testing.T) {
	h := &StripeEventHandler{
		logger: slog.Default(),
		now:    func() time.Time { return time.Now().UTC() },
	}
	id := h.resolveTenantID(context.Background(), "", "", "")
	if id != "" {
		t.Errorf("expected empty, got %q", id)
	}
}

// =============================================================================
// stripe_webhook.go — findPlanByID edge cases
// =============================================================================

func TestFindPlanByID_Empty(t *testing.T) {
	plan := findPlanByID(nil, "test")
	if plan != nil {
		t.Errorf("expected nil, got %v", plan)
	}

	plan = findPlanByID([]Plan{}, "test")
	if plan != nil {
		t.Errorf("expected nil, got %v", plan)
	}
}

// =============================================================================
// stripe_webhook.go — PlanByStripePriceID edge cases
// =============================================================================

func TestPlanByStripePriceID_EmptyPriceID(t *testing.T) {
	plan := PlanByStripePriceID(nil, "")
	if plan != nil {
		t.Errorf("expected nil for empty price ID")
	}
}

func TestPlanByStripePriceID_NotFound(t *testing.T) {
	plan := PlanByStripePriceID([]Plan{
		{ID: "plan1", StripePriceID: "price_1"},
	}, "price_notfound")
	if plan != nil {
		t.Errorf("expected nil, got %v", plan)
	}
}

// =============================================================================
// stripe_webhook.go — NewStripeEventHandler with nil logger
// =============================================================================

func TestNewStripeEventHandler_NilLogger(t *testing.T) {
	h := NewStripeEventHandler(nil, nil, nil, nil, nil)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

// =============================================================================
// stripe_webhook.go — alreadyProcessed / markProcessed with empty map
// =============================================================================

func TestStripeEventHandler_Processed_Empty(t *testing.T) {
	h := NewStripeEventHandler(nil, nil, nil, nil, slog.Default())
	if h.alreadyProcessed("evt_1") {
		t.Error("expected false for non-existent event")
	}
	h.markProcessed("evt_1")
	if !h.alreadyProcessed("evt_1") {
		t.Error("expected true after marking processed")
	}
}

// =============================================================================
// stripe_webhook.go — AlreadyProcessed with nil client
// =============================================================================

func TestStripeEventHandler_Handle_NilClient(t *testing.T) {
	h := NewStripeEventHandler(nil, nil, nil, nil, slog.Default())
	// Client is nil, signature check should fail
	err := h.Handle(context.Background(), []byte(`{}`), "t=1,v1=test")
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

// =============================================================================
// stripe_webhook.go — Handle with missing event type
// =============================================================================

func TestStripeEventHandler_Handle_EmptyType(t *testing.T) {
	store := newWebhookStore()
	h, secret := newTestHandler(t, store, nil)

	sig := signPayload(t, secret, []byte(`{"id":"evt_1","type":""}`))
	err := h.Handle(context.Background(), []byte(`{"id":"evt_1","type":""}`), sig)
	if err == nil {
		t.Fatal("expected error for empty event type")
	}
}

// =============================================================================
// metering.go — runCtx fallback
// =============================================================================

func TestMeter_RunCtx_Fallback(t *testing.T) {
	m := &Meter{} // No stopCtx
	ctx := m.runCtx()
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

// =============================================================================
// stripe_webhook.go — sweepLocked edge case
// =============================================================================

func TestStripeEventHandler_SweepLocked_ZeroTTL(t *testing.T) {
	h := &StripeEventHandler{
		seenTTL: 0,
		seen:    map[string]time.Time{"evt_1": time.Now()},
		logger:  slog.Default(),
		now:     func() time.Time { return time.Now().UTC() },
	}

	// Should be a no-op when TTL is zero or negative
	h.sweepLocked()
	if len(h.seen) != 1 {
		t.Errorf("expected 1 entry after sweep with zero TTL, got %d", len(h.seen))
	}
}

// =============================================================================
// tenant_metadata.go — GetStripeMetadata/SetStripeMetadata edge cases
// =============================================================================

func TestGetStripeMetadata_NilTenant(t *testing.T) {
	md, err := GetStripeMetadata(nil)
	if err != nil {
		t.Fatalf("GetStripeMetadata nil tenant: %v", err)
	}
	if !md.IsZero() {
		t.Errorf("expected zero metadata, got %v", md)
	}
}

func TestGetStripeMetadata_InvalidJSON(t *testing.T) {
	tenant := &core.Tenant{
		MetadataJSON: "{invalid json}",
	}
	_, err := GetStripeMetadata(tenant)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSetStripeMetadata_NilTenantEdge(t *testing.T) {
	err := SetStripeMetadata(nil, StripeMetadata{})
	if err == nil {
		t.Fatal("expected error for nil tenant")
	}
}

func TestSetStripeMetadata_EmptyMetadata(t *testing.T) {
	tenant := &core.Tenant{
		MetadataJSON: "",
	}
	md := StripeMetadata{}
	if err := SetStripeMetadata(tenant, md); err != nil {
		t.Fatalf("SetStripeMetadata: %v", err)
	}
	if tenant.MetadataJSON != "" {
		t.Errorf("expected empty metadata, got %q", tenant.MetadataJSON)
	}
}

func TestSetStripeMetadata_PreservesOtherKeysEdge(t *testing.T) {
	tenant := &core.Tenant{
		MetadataJSON: `{"other_key":"value"}`,
	}
	md := StripeMetadata{
		CustomerID: "cus_123",
	}
	if err := SetStripeMetadata(tenant, md); err != nil {
		t.Fatalf("SetStripeMetadata: %v", err)
	}
	if tenant.MetadataJSON == "" {
		t.Fatal("expected non-empty metadata JSON")
	}
}
