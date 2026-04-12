package resource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// fakeRuntime is a slim core.ContainerRuntime fixture that only
// implements ListByLabels + Stats. The rest of the interface returns
// zero values so a test accidentally calling an unused method fails
// loudly rather than returning plausible-looking garbage.
type fakeRuntime struct {
	containers []core.ContainerInfo
	stats      map[string]*core.ContainerStats
	listErr    error
	statsErr   map[string]error
}

func (f *fakeRuntime) Ping() error { return nil }
func (f *fakeRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (f *fakeRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (f *fakeRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (f *fakeRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (f *fakeRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (f *fakeRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.containers, nil
}
func (f *fakeRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (f *fakeRuntime) Stats(_ context.Context, id string) (*core.ContainerStats, error) {
	if err, ok := f.statsErr[id]; ok {
		return nil, err
	}
	if s, ok := f.stats[id]; ok {
		return s, nil
	}
	return &core.ContainerStats{}, nil
}
func (f *fakeRuntime) ImagePull(_ context.Context, _ string) error           { return nil }
func (f *fakeRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (f *fakeRuntime) ImageRemove(_ context.Context, _ string) error         { return nil }
func (f *fakeRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

var _ core.ContainerRuntime = (*fakeRuntime)(nil)

// mbToBytes is a helper that matches how ContainerStats reports memory
// (raw bytes, to be divided by 1024*1024 in the aggregator).
func mbToBytes(mb int64) int64 { return mb * 1024 * 1024 }

// TestAggregateTenantUsage_GroupsByTenantLabel is the headline test
// for Phase 3.3.6 resource aggregation. Five containers spread across
// two tenants — including one system container with no tenant label —
// must produce two aggregated TenantUsage entries with correctly
// summed container counts, distinct app counts, live CPU, and live
// memory. The system container must not leak into either tenant.
func TestAggregateTenantUsage_GroupsByTenantLabel(t *testing.T) {
	rt := &fakeRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "c1", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-a",
					"monster.app.id": "app-checkout",
				},
			},
			{
				ID: "c2", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-a",
					"monster.app.id": "app-checkout", // same app, second replica
				},
			},
			{
				ID: "c3", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-a",
					"monster.app.id": "app-billing", // second distinct app
				},
			},
			{
				ID: "c4", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-b",
					"monster.app.id": "app-one",
				},
			},
			{
				// System container — no tenant label. Must not bleed into
				// either tenant's usage.
				ID: "c5", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
				},
			},
			{
				// Stopped container — must not count.
				ID: "c6", State: "exited",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-a",
					"monster.app.id": "app-checkout",
				},
			},
		},
		stats: map[string]*core.ContainerStats{
			"c1": {CPUPercent: 10.0, MemoryUsage: mbToBytes(256)},
			"c2": {CPUPercent: 15.0, MemoryUsage: mbToBytes(384)},
			"c3": {CPUPercent: 5.0, MemoryUsage: mbToBytes(128)},
			"c4": {CPUPercent: 42.0, MemoryUsage: mbToBytes(1024)},
		},
	}

	usage, err := AggregateTenantUsage(context.Background(), rt)
	if err != nil {
		t.Fatalf("AggregateTenantUsage: %v", err)
	}

	if len(usage) != 2 {
		t.Fatalf("aggregated tenants = %d, want 2", len(usage))
	}

	a := usage["tenant-a"]
	if a == nil {
		t.Fatal("tenant-a missing from usage map")
	}
	if a.Containers != 3 {
		t.Errorf("tenant-a Containers = %d, want 3", a.Containers)
	}
	if a.Apps != 2 {
		t.Errorf("tenant-a Apps = %d, want 2 (distinct apps)", a.Apps)
	}
	if a.CPUPercent != 30.0 {
		t.Errorf("tenant-a CPUPercent = %.1f, want 30.0", a.CPUPercent)
	}
	if a.MemoryMB != 256+384+128 {
		t.Errorf("tenant-a MemoryMB = %d, want %d", a.MemoryMB, 256+384+128)
	}

	b := usage["tenant-b"]
	if b == nil {
		t.Fatal("tenant-b missing from usage map")
	}
	if b.Containers != 1 {
		t.Errorf("tenant-b Containers = %d, want 1", b.Containers)
	}
	if b.Apps != 1 {
		t.Errorf("tenant-b Apps = %d, want 1", b.Apps)
	}
	if b.MemoryMB != 1024 {
		t.Errorf("tenant-b MemoryMB = %d, want 1024", b.MemoryMB)
	}
}

// TestAggregateTenantUsage_NilRuntime proves the aggregator is safe
// to call on a master that has no Docker socket (dev mode). The
// contract is "empty usage map, no error" so the deploy gate can
// still call CheckTenantQuota without a nil-guard of its own.
func TestAggregateTenantUsage_NilRuntime(t *testing.T) {
	usage, err := AggregateTenantUsage(context.Background(), nil)
	if err != nil {
		t.Fatalf("AggregateTenantUsage(nil) = %v, want nil", err)
	}
	if len(usage) != 0 {
		t.Errorf("usage len = %d, want 0", len(usage))
	}
}

// TestAggregateTenantUsage_StatsErrorDoesNotVoidTenant verifies a
// single container with a Stats error still counts toward the
// tenant's container total but contributes zero live CPU/memory. A
// bad-container cascade that dropped the entire tenant would be the
// wrong failure mode — it would make the quota gate UNDER-report and
// let the tenant through.
func TestAggregateTenantUsage_StatsErrorDoesNotVoidTenant(t *testing.T) {
	rt := &fakeRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "good", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-x",
					"monster.app.id": "app-1",
				},
			},
			{
				ID: "bad", State: "running",
				Labels: map[string]string{
					"monster.enable": "true",
					"monster.tenant": "tenant-x",
					"monster.app.id": "app-2",
				},
			},
		},
		stats: map[string]*core.ContainerStats{
			"good": {CPUPercent: 20.0, MemoryUsage: mbToBytes(512)},
		},
		statsErr: map[string]error{
			"bad": errors.New("docker stats timeout"),
		},
	}

	usage, err := AggregateTenantUsage(context.Background(), rt)
	if err != nil {
		t.Fatalf("AggregateTenantUsage: %v", err)
	}
	x := usage["tenant-x"]
	if x == nil {
		t.Fatal("tenant-x missing")
	}
	if x.Containers != 2 {
		t.Errorf("Containers = %d, want 2 (bad container should still count)", x.Containers)
	}
	if x.Apps != 2 {
		t.Errorf("Apps = %d, want 2", x.Apps)
	}
	if x.CPUPercent != 20.0 {
		t.Errorf("CPUPercent = %.1f, want 20.0 (bad=0)", x.CPUPercent)
	}
	if x.MemoryMB != 512 {
		t.Errorf("MemoryMB = %d, want 512", x.MemoryMB)
	}
}

// TestAggregateTenantUsage_ListError surfaces runtime list failures
// to the caller — unlike the per-container Stats failure, a failed
// list means we have no ground truth and must not pretend the tenant
// is at zero usage.
func TestAggregateTenantUsage_ListError(t *testing.T) {
	rt := &fakeRuntime{listErr: errors.New("docker daemon down")}
	_, err := AggregateTenantUsage(context.Background(), rt)
	if err == nil {
		t.Fatal("AggregateTenantUsage = nil, want list error")
	}
}

// TestCheckTenantQuota_TableDriven covers every dimension plus the
// "unlimited" (non-positive) fast path.
func TestCheckTenantQuota_TableDriven(t *testing.T) {
	cases := []struct {
		name      string
		usage     *TenantUsage
		quota     TenantQuota
		wantExc   bool
		wantField string // substring that must appear in error message
	}{
		{
			name:  "zero usage passes everything",
			usage: &TenantUsage{TenantID: "t"},
			quota: TenantQuota{MaxApps: 1, MaxContainers: 1, MaxMemoryMB: 1, MaxCPUPercent: 1},
		},
		{
			name:  "nil usage passes",
			usage: nil,
			quota: TenantQuota{MaxApps: 1},
		},
		{
			name:  "apps at limit passes",
			usage: &TenantUsage{Apps: 5},
			quota: TenantQuota{MaxApps: 5},
		},
		{
			name:      "apps over limit",
			usage:     &TenantUsage{Apps: 6},
			quota:     TenantQuota{MaxApps: 5},
			wantExc:   true,
			wantField: "apps",
		},
		{
			name:      "containers over limit",
			usage:     &TenantUsage{Containers: 11},
			quota:     TenantQuota{MaxContainers: 10},
			wantExc:   true,
			wantField: "containers",
		},
		{
			name:      "memory over limit",
			usage:     &TenantUsage{MemoryMB: 4097},
			quota:     TenantQuota{MaxMemoryMB: 4096},
			wantExc:   true,
			wantField: "memory",
		},
		{
			name:      "cpu over limit",
			usage:     &TenantUsage{CPUPercent: 101.0},
			quota:     TenantQuota{MaxCPUPercent: 100.0},
			wantExc:   true,
			wantField: "cpu",
		},
		{
			name:  "negative limit means unlimited",
			usage: &TenantUsage{Apps: 9999},
			quota: TenantQuota{MaxApps: -1},
		},
		{
			name:  "zero limit means unlimited",
			usage: &TenantUsage{Apps: 9999},
			quota: TenantQuota{MaxApps: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckTenantQuota(tc.usage, tc.quota)
			if tc.wantExc {
				if err == nil {
					t.Fatal("CheckTenantQuota = nil, want ErrQuotaExceeded")
				}
				if !errors.Is(err, ErrQuotaExceeded) {
					t.Errorf("errors.Is(err, ErrQuotaExceeded) = false, err = %v", err)
				}
				if tc.wantField != "" && !contains(err.Error(), tc.wantField) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantField)
				}
				return
			}
			if err != nil {
				t.Errorf("CheckTenantQuota = %v, want nil", err)
			}
		})
	}
}

// contains is a tiny strings.Contains shim so we can avoid importing
// the strings package for a single call.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// TestCheckTenantQuota_FirstFailureWins ensures the error identifies
// the dimension that tripped the gate — important for ops: "tenant
// over memory" vs "tenant over apps" leads to very different advice.
func TestCheckTenantQuota_FirstFailureWins(t *testing.T) {
	err := CheckTenantQuota(
		&TenantUsage{Apps: 10, Containers: 20, MemoryMB: 99999, CPUPercent: 500},
		TenantQuota{MaxApps: 5, MaxContainers: 10, MaxMemoryMB: 4096, MaxCPUPercent: 100},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	// First dimension checked is Apps, so that should be the field reported.
	if !contains(err.Error(), "apps") {
		t.Errorf("first-failure error = %q, want apps dimension", err.Error())
	}
}

// ─── Integration-style demonstration of the full flow ─────────────

// TestResourceQuota_FullFlow drives AggregateTenantUsage and
// CheckTenantQuota end-to-end through a synthetic Docker runtime,
// proving that a tenant at its memory limit is refused on the next
// deploy attempt while a tenant well under limit is allowed. This is
// the scenario the deploy pipeline's pre-flight gate will run.
func TestResourceQuota_FullFlow(t *testing.T) {
	rt := &fakeRuntime{
		containers: []core.ContainerInfo{
			{ID: "t1-c1", State: "running", Labels: map[string]string{
				"monster.enable": "true", "monster.tenant": "over",
				"monster.app.id": "app-a",
			}},
			{ID: "t1-c2", State: "running", Labels: map[string]string{
				"monster.enable": "true", "monster.tenant": "over",
				"monster.app.id": "app-b",
			}},
			{ID: "t2-c1", State: "running", Labels: map[string]string{
				"monster.enable": "true", "monster.tenant": "under",
				"monster.app.id": "app-small",
			}},
		},
		stats: map[string]*core.ContainerStats{
			"t1-c1": {MemoryUsage: mbToBytes(3000)},
			"t1-c2": {MemoryUsage: mbToBytes(1500)},
			"t2-c1": {MemoryUsage: mbToBytes(128)},
		},
	}

	usage, err := AggregateTenantUsage(context.Background(), rt)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	quota := TenantQuota{MaxApps: 10, MaxMemoryMB: 4096} // pro plan-ish

	if err := CheckTenantQuota(usage["over"], quota); err == nil {
		t.Error("over tenant passed quota check, want ErrQuotaExceeded")
	} else if !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("over tenant error = %v, want ErrQuotaExceeded", err)
	}

	if err := CheckTenantQuota(usage["under"], quota); err != nil {
		t.Errorf("under tenant failed quota check unexpectedly: %v", err)
	}
}

// Guard against a regression where an error from the resource package
// stops wrapping ErrQuotaExceeded — the deploy gate relies on errors.Is
// to map quota refusals to 429.
func TestCheckTenantQuota_ErrorWrapping(t *testing.T) {
	err := CheckTenantQuota(&TenantUsage{Apps: 100}, TenantQuota{MaxApps: 1})
	if err == nil {
		t.Fatal("expected error")
	}
	wrapped := fmt.Errorf("deploy gate: %w", err)
	if !errors.Is(wrapped, ErrQuotaExceeded) {
		t.Errorf("errors.Is through double-wrap failed: %v", wrapped)
	}
}
