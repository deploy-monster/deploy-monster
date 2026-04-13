package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// METER COLLECT TESTS — running containers aggregation
// =====================================================

func TestMeterCollect_MultipleTenants(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "c1", Name: "app1",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-a",
					"monster.app.id": "app-1",
				},
			},
			{
				ID: "c2", Name: "app2",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-a",
					"monster.app.id": "app-2",
				},
			},
			{
				ID: "c3", Name: "app3",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-b",
					"monster.app.id": "app-3",
				},
			},
			{
				ID: "c4", Name: "app4",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-c",
					"monster.app.id": "app-4",
				},
			},
			{
				ID: "c5", Name: "no-tenant",
				Labels: map[string]string{
					"monster.enable": "true",
					// No tenant label — should be skipped
				},
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	// Should not panic; exercises the full grouping logic
	meter.collect()
}

func TestMeterCollect_EmptyLabels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "c1",
				Name:   "bare-container",
				Labels: map[string]string{},
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	// Container with empty labels should be handled gracefully
	meter.collect()
}

func TestMeterCollect_NilLabels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:     "c1",
				Name:   "nil-labels",
				Labels: nil,
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	// Container with nil labels map — should not panic
	meter.collect()
}

// =====================================================
// METER START/STOP LIFECYCLE
// =====================================================

func TestMeterStartStop_Idempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(nil, nil))
	runtime := &mockContainerRuntime{}
	store := &mockStore{}

	meter := NewMeter(store, runtime, logger)
	meter.Start()
	time.Sleep(5 * time.Millisecond)
	// Stop should not block or panic
	meter.Stop()
}

// =====================================================
// QUOTA CHECK — edge cases
// =====================================================

func TestQuotaCheck_ZeroLimit(t *testing.T) {
	store := &mockStore{total: 0}
	plan := Plan{MaxApps: 0}

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MaxApps=0: total(0) < 0 is false, and MaxApps < 0 is false
	// So AppsOK should be false (no apps allowed)
	if status.AppsOK {
		t.Error("expected AppsOK=false when MaxApps=0 and total=0 (0 is not < 0)")
	}
}

func TestQuotaCheck_NegativeLimit_IsUnlimited(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		maxApps int
		wantOK  bool
	}{
		{"unlimited with zero usage", 0, -1, true},
		{"unlimited with huge usage", 100000, -1, true},
		{"unlimited -2", 500, -2, true}, // any negative = unlimited
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{total: tt.total}
			plan := Plan{MaxApps: tt.maxApps}

			status, err := QuotaCheckCtx(context.Background(), store, "t1", plan)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status.AppsOK != tt.wantOK {
				t.Errorf("AppsOK = %v, want %v", status.AppsOK, tt.wantOK)
			}
		})
	}
}

func TestQuotaCheck_StatusFieldValues(t *testing.T) {
	store := &mockStore{total: 7}
	plan := Plan{MaxApps: 25}

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-x", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.AppsUsed != 7 {
		t.Errorf("AppsUsed = %d, want 7", status.AppsUsed)
	}
	if status.AppsLimit != 25 {
		t.Errorf("AppsLimit = %d, want 25", status.AppsLimit)
	}
	if !status.AppsOK {
		t.Error("expected AppsOK=true when 7 < 25")
	}
}

// =====================================================
// STRIPE CLIENT CONSTRUCTOR TESTS
// =====================================================

func TestNewStripeClient_EmptyKeys(t *testing.T) {
	client := NewStripeClient("", "")
	if client == nil {
		t.Fatal("NewStripeClient should return non-nil even with empty keys")
	}
	if client.secretKey != "" {
		t.Errorf("secretKey should be empty, got %q", client.secretKey)
	}
	if client.webhookKey != "" {
		t.Errorf("webhookKey should be empty, got %q", client.webhookKey)
	}
}

func TestNewStripeClient_HTTPClientTimeout(t *testing.T) {
	client := NewStripeClient("sk_test", "whsec_test")
	if client.client == nil {
		t.Fatal("HTTP client should be initialized")
	}
	if client.client.Timeout != 30*time.Second {
		t.Errorf("HTTP client timeout = %v, want 30s", client.client.Timeout)
	}
}

// =====================================================
// STRIPE WEBHOOK SIGNATURE — additional edge cases
// =====================================================

func TestVerifyWebhookSignature_EmptyPayload(t *testing.T) {
	webhookKey := "whsec_testkey"
	client := NewStripeClient("sk_test", webhookKey)

	payload := []byte("")
	timestamp := "1700000000"
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(webhookKey))
	mac.Write([]byte(signedPayload))
	validSig := hex.EncodeToString(mac.Sum(nil))

	sigHeader := fmt.Sprintf("t=%s,v1=%s", timestamp, validSig)
	got := client.VerifyWebhookSignature(payload, sigHeader)
	if !got {
		t.Error("empty payload with valid signature should verify")
	}
}

func TestVerifyWebhookSignature_MultipleV1Signatures(t *testing.T) {
	// Stripe can send multiple v1 signatures; our code uses the last one found
	webhookKey := "whsec_multi"
	client := NewStripeClient("sk_test", webhookKey)

	payload := []byte(`{"data":"test"}`)
	timestamp := "1600000000"
	signedPayload := timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(webhookKey))
	mac.Write([]byte(signedPayload))
	validSig := hex.EncodeToString(mac.Sum(nil))

	// Header with invalid v1 first, then valid v1 second
	sigHeader := fmt.Sprintf("t=%s,v1=invalid,v1=%s", timestamp, validSig)
	got := client.VerifyWebhookSignature(payload, sigHeader)
	// Our implementation takes the last v1 value
	if !got {
		t.Error("should accept last v1 signature")
	}
}

// =====================================================
// PLAN COMPARISON — upgrade/downgrade
// =====================================================

func TestPlanUpgradeDowngrade(t *testing.T) {
	tests := []struct {
		name      string
		fromIndex int
		toIndex   int
		isUpgrade bool
	}{
		{"free to pro", 0, 1, true},
		{"free to business", 0, 2, true},
		{"pro to business", 1, 2, true},
		{"pro to free", 1, 0, false},
		{"business to pro", 2, 1, false},
		{"business to free", 2, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := BuiltinPlans[tt.fromIndex]
			to := BuiltinPlans[tt.toIndex]

			isUpgrade := to.PriceCents > from.PriceCents
			if isUpgrade != tt.isUpgrade {
				t.Errorf("upgrade from %s to %s: got isUpgrade=%v, want %v",
					from.ID, to.ID, isUpgrade, tt.isUpgrade)
			}
		})
	}
}

func TestPlanResourceComparison(t *testing.T) {
	free := BuiltinPlans[0]
	enterprise := BuiltinPlans[3]

	// Enterprise should have strictly more (or unlimited=-1) vs free
	if enterprise.MaxApps != -1 {
		t.Errorf("enterprise MaxApps should be -1, got %d", enterprise.MaxApps)
	}
	if free.MaxApps <= 0 {
		t.Errorf("free MaxApps should be positive, got %d", free.MaxApps)
	}

	// All enterprise limits should be -1
	limits := []int{
		enterprise.MaxContainers, enterprise.MaxCPUCores, enterprise.MaxRAMMB,
		enterprise.MaxDiskGB, enterprise.MaxBandwidthGB, enterprise.MaxDomains,
		enterprise.MaxDatabases, enterprise.MaxTeamMembers, enterprise.MaxServers,
		enterprise.BuildMinutes, enterprise.BackupGB,
	}
	for i, l := range limits {
		if l != -1 {
			t.Errorf("enterprise limit[%d] = %d, want -1", i, l)
		}
	}
}

// =====================================================
// TENANT USAGE STRUCT
// =====================================================

func TestTenantUsage_Accumulation(t *testing.T) {
	usage := &TenantUsage{}

	// Simulate multiple container contributions
	for i := 0; i < 10; i++ {
		usage.Containers++
		usage.AppIDs = append(usage.AppIDs, fmt.Sprintf("app-%d", i))
	}
	usage.CPUSeconds = 7200.5
	usage.RAMMBHours = 1024.0
	usage.BandwidthMB = 2048.75

	if usage.Containers != 10 {
		t.Errorf("Containers = %d, want 10", usage.Containers)
	}
	if len(usage.AppIDs) != 10 {
		t.Errorf("AppIDs count = %d, want 10", len(usage.AppIDs))
	}
	if usage.CPUSeconds != 7200.5 {
		t.Errorf("CPUSeconds = %f, want 7200.5", usage.CPUSeconds)
	}
	if usage.RAMMBHours != 1024.0 {
		t.Errorf("RAMMBHours = %f, want 1024.0", usage.RAMMBHours)
	}
	if usage.BandwidthMB != 2048.75 {
		t.Errorf("BandwidthMB = %f, want 2048.75", usage.BandwidthMB)
	}
}

// =====================================================
// MODULE LIFECYCLE — additional tests
// =====================================================

func TestModule_InitFields(t *testing.T) {
	// Test that New() returns properly initialized module
	m := New()
	if m.core != nil {
		t.Error("core should be nil before Init")
	}
	if m.store != nil {
		t.Error("store should be nil before Init")
	}
	if m.meter != nil {
		t.Error("meter should be nil before Init")
	}
	if m.logger != nil {
		t.Error("logger should be nil before Init")
	}
}
