package billing

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockContainerRuntime implements core.ContainerRuntime for testing.
type mockContainerRuntime struct {
	containers []core.ContainerInfo
	err        error
}

func (m *mockContainerRuntime) Ping() error { return nil }
func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (m *mockContainerRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *mockContainerRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockContainerRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return m.containers, m.err
}

func (m *mockContainerRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}

func (m *mockContainerRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}

func (m *mockContainerRuntime) ImagePull(_ context.Context, _ string) error { return nil }

func (m *mockContainerRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}

func (m *mockContainerRuntime) ImageRemove(_ context.Context, _ string) error { return nil }

func (m *mockContainerRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}

func (m *mockContainerRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// mockStore implements the subset of core.Store used by metering/quota.
type mockStore struct {
	core.Store
	apps  []core.Application
	total int
	err   error
}

func (m *mockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return m.apps, m.total, m.err
}

func (m *mockStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error {
	return m.err
}

func (m *mockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) {
	return nil, 0, m.err
}

func (m *mockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return m.err }
func (m *mockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, m.err
}
func (m *mockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
	return m.err
}

func (m *mockStore) UpdateTOTPEnabled(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func TestNewMeter(t *testing.T) {
	logger := slog.Default()
	runtime := &mockContainerRuntime{}
	store := &mockStore{}

	meter := NewMeter(store, runtime, logger)

	if meter == nil {
		t.Fatal("NewMeter returned nil")
	}
	if meter.store != store {
		t.Error("store not set correctly")
	}
	if meter.runtime != runtime {
		t.Error("runtime not set correctly")
	}
	if meter.logger != logger {
		t.Error("logger not set correctly")
	}
	if meter.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

func TestMeterStartStop(t *testing.T) {
	logger := slog.Default()
	runtime := &mockContainerRuntime{}
	store := &mockStore{}

	meter := NewMeter(store, runtime, logger)
	meter.Start()

	// Give the goroutine a moment to start.
	time.Sleep(10 * time.Millisecond)

	// Stop should not panic or block indefinitely.
	meter.Stop()
}

func TestMeterCollect_NilRuntime(t *testing.T) {
	logger := slog.Default()
	store := &mockStore{}

	meter := NewMeter(store, nil, logger)

	// collect with nil runtime should return immediately without panic.
	meter.collect()
}

func TestMeterCollect_RuntimeError(t *testing.T) {
	logger := slog.Default()
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		err: fmt.Errorf("docker not reachable"),
	}

	meter := NewMeter(store, runtime, logger)

	// Should not panic on runtime error.
	meter.collect()
}

func TestMeterCollect_NoContainers(t *testing.T) {
	logger := slog.Default()
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{},
	}

	meter := NewMeter(store, runtime, logger)
	meter.collect()
	// No panic, no error — just zero tenants collected.
}

func TestMeterCollect_GroupsByTenant(t *testing.T) {
	logger := slog.Default()
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "c1", Name: "app1",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-1",
					"monster.app.id": "app-1",
				},
			},
			{
				ID: "c2", Name: "app2",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-1",
					"monster.app.id": "app-2",
				},
			},
			{
				ID: "c3", Name: "app3",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-2",
					"monster.app.id": "app-3",
				},
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	meter.collect()
	// Verifies it doesn't panic. The internal aggregation produces
	// tenantUsage with 2 entries: tenant-1 (2 containers), tenant-2 (1 container).
}

func TestMeterCollect_SkipsMissingTenantLabel(t *testing.T) {
	logger := slog.Default()
	store := &mockStore{}
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "c1", Name: "app1",
				Labels: map[string]string{
					"monster.enable": "true",
					// No "monster.tenant" label — should be skipped.
					"monster.app.id": "app-1",
				},
			},
		},
	}

	meter := NewMeter(store, runtime, logger)
	meter.collect()
	// No panic; the container without tenant label is skipped.
}

func TestTenantUsage(t *testing.T) {
	usage := &TenantUsage{}

	if usage.Containers != 0 {
		t.Errorf("initial Containers = %d, want 0", usage.Containers)
	}
	if len(usage.AppIDs) != 0 {
		t.Errorf("initial AppIDs should be empty")
	}
	if usage.CPUSeconds != 0 {
		t.Errorf("initial CPUSeconds = %f, want 0", usage.CPUSeconds)
	}
	if usage.RAMMBHours != 0 {
		t.Errorf("initial RAMMBHours = %f, want 0", usage.RAMMBHours)
	}
	if usage.BandwidthMB != 0 {
		t.Errorf("initial BandwidthMB = %f, want 0", usage.BandwidthMB)
	}

	// Simulate recording usage.
	usage.Containers = 5
	usage.AppIDs = []string{"a1", "a2", "a3"}
	usage.CPUSeconds = 3600.0
	usage.RAMMBHours = 512.0
	usage.BandwidthMB = 1024.0

	if usage.Containers != 5 {
		t.Errorf("Containers = %d, want 5", usage.Containers)
	}
	if len(usage.AppIDs) != 3 {
		t.Errorf("AppIDs count = %d, want 3", len(usage.AppIDs))
	}
}

func TestQuotaCheck_WithinLimits(t *testing.T) {
	store := &mockStore{total: 2}
	plan := Plan{MaxApps: 10}

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.AppsOK {
		t.Error("expected AppsOK to be true when under limit")
	}
	if status.AppsUsed != 2 {
		t.Errorf("AppsUsed = %d, want 2", status.AppsUsed)
	}
	if status.AppsLimit != 10 {
		t.Errorf("AppsLimit = %d, want 10", status.AppsLimit)
	}
}

func TestQuotaCheck_AtLimit(t *testing.T) {
	store := &mockStore{total: 10}
	plan := Plan{MaxApps: 10}

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.AppsOK {
		t.Error("expected AppsOK to be false when at limit (total >= MaxApps)")
	}
}

func TestQuotaCheck_OverLimit(t *testing.T) {
	store := &mockStore{total: 15}
	plan := Plan{MaxApps: 10}

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.AppsOK {
		t.Error("expected AppsOK to be false when over limit")
	}
}

func TestQuotaCheck_UnlimitedPlan(t *testing.T) {
	store := &mockStore{total: 9999}
	plan := Plan{MaxApps: -1} // -1 = unlimited

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.AppsOK {
		t.Error("expected AppsOK to be true for unlimited plan (MaxApps = -1)")
	}
	if status.AppsLimit != -1 {
		t.Errorf("AppsLimit = %d, want -1", status.AppsLimit)
	}
}

func TestQuotaCheck_ZeroApps(t *testing.T) {
	store := &mockStore{total: 0}
	plan := Plan{MaxApps: 5}

	status, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !status.AppsOK {
		t.Error("expected AppsOK to be true when zero apps used")
	}
	if status.AppsUsed != 0 {
		t.Errorf("AppsUsed = %d, want 0", status.AppsUsed)
	}
}

func TestQuotaCheck_StoreError(t *testing.T) {
	store := &mockStore{err: fmt.Errorf("database unavailable")}
	plan := Plan{MaxApps: 10}

	_, err := QuotaCheckCtx(context.Background(), store, "tenant-1", plan)
	if err == nil {
		t.Fatal("expected error from store")
	}
}

func TestQuotaCheck_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		maxApps int
		wantOK  bool
	}{
		{"zero of 5", 0, 5, true},
		{"1 of 5", 1, 5, true},
		{"4 of 5", 4, 5, true},
		{"5 of 5 (at limit)", 5, 5, false},
		{"6 of 5 (over limit)", 6, 5, false},
		{"100 of unlimited", 100, -1, true},
		{"0 of unlimited", 0, -1, true},
		{"1 of 1", 1, 1, false},
		{"0 of 1", 0, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &mockStore{total: tt.total}
			plan := Plan{MaxApps: tt.maxApps}

			status, err := QuotaCheckCtx(context.Background(), store, "test-tenant", plan)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status.AppsOK != tt.wantOK {
				t.Errorf("AppsOK = %v, want %v (used=%d, limit=%d)",
					status.AppsOK, tt.wantOK, tt.total, tt.maxApps)
			}
		})
	}
}

func TestQuotaCheck_BuiltinPlans(t *testing.T) {
	// Test QuotaCheck against each builtin plan with known usage.
	tests := []struct {
		planID   string
		plan     Plan
		appCount int
		wantOK   bool
	}{
		{"free_under", BuiltinPlans[0], 1, true},
		{"free_at_limit", BuiltinPlans[0], BuiltinPlans[0].MaxApps, false},
		{"pro_under", BuiltinPlans[1], 10, true},
		{"pro_at_limit", BuiltinPlans[1], BuiltinPlans[1].MaxApps, false},
		{"business_under", BuiltinPlans[2], 50, true},
		{"enterprise_high", BuiltinPlans[3], 10000, true}, // unlimited
	}

	for _, tt := range tests {
		t.Run(tt.planID, func(t *testing.T) {
			store := &mockStore{total: tt.appCount}
			status, err := QuotaCheckCtx(context.Background(), store, "t1", tt.plan)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status.AppsOK != tt.wantOK {
				t.Errorf("AppsOK = %v, want %v", status.AppsOK, tt.wantOK)
			}
		})
	}
}
