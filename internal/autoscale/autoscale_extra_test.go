package autoscale

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// evaluateAll — nil core early return
// =============================================================================

func TestEvaluateAll_NilCore(t *testing.T) {
	m := &Module{}
	// Should not panic when core is nil
	m.evaluateAll()
}

// =============================================================================
// evaluateAll — nil DB/Bolt early return
// =============================================================================

func TestEvaluateAll_NilDB(t *testing.T) {
	m := &Module{
		core:   &core.Core{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	m.evaluateAll()
}

// =============================================================================
// evaluateAll — disabled config skipped
// =============================================================================

func TestEvaluateAll_SkipsDisabledConfig(t *testing.T) {
	bolt := newTestBolt()
	if err := bolt.Set("autoscale", "app-1", autoscaleConfig{Enabled: false, MinReplicas: 1, MaxReplicas: 3, CPUTarget: 80}, 0); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	bolt.setErr = errors.New("should not be called") // Set should not be called for disabled config
	m := testModule(&autoscaleStore{}, &autoscaleRuntime{}, bolt)

	m.evaluateAll()
	// No decision should be persisted for disabled config
	_, ok := m.lastDecision("app-1")
	if ok {
		t.Error("expected no decision for disabled config")
	}
}

// =============================================================================
// evaluateAll — list error logs warning (non-KVNotFound)
// =============================================================================

func TestEvaluateAll_ListErrorLogsWarning(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bolt := &testBolt{
		data:    make(map[string]map[string][]byte),
		listErr: errors.New("permission denied"),
	}
	m := testModule(&autoscaleStore{}, &autoscaleRuntime{}, bolt)
	m.logger = logger

	m.evaluateAll()

	if !bytes.Contains(logs.Bytes(), []byte("autoscale: list config bucket failed")) {
		t.Fatalf("expected warning log, got: %q", logs.String())
	}
}

// =============================================================================
// evaluate — scale down path (CPU below release threshold, replicas > min)
// =============================================================================

func TestEvaluate_ScaleDown(t *testing.T) {
	bolt := newTestBolt()
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 3}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}},
		stats:      map[string]*core.ContainerStats{"ctr-1": {CPUPercent: 10, MemoryPercent: 10}},
	}
	m := testModule(store, runtime, bolt)

	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 5, CPUTarget: 80, RAMTarget: 90})

	if store.updated == nil || store.updated.Replicas != 2 {
		t.Fatalf("updated replicas = %#v, want 2", store.updated)
	}
	got, ok := m.lastDecision("app-1")
	if !ok {
		t.Fatal("expected persisted decision")
	}
	if got.Action != "scale_down" {
		t.Fatalf("decision action = %q, want scale_down", got.Action)
	}
	if got.DesiredReps != 2 {
		t.Fatalf("desired replicas = %d, want 2", got.DesiredReps)
	}
}

// =============================================================================
// evaluate — hold path (CPU within thresholds, replicas at boundaries)
// =============================================================================

func TestEvaluate_Hold(t *testing.T) {
	bolt := newTestBolt()
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 2}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}},
		stats:      map[string]*core.ContainerStats{"ctr-1": {CPUPercent: 50, MemoryPercent: 40}},
	}
	m := testModule(store, runtime, bolt)

	// CPUTarget=80 → cpuTrigger=80, cpuRelease=56; RAMTarget=90 → memTrigger=90, memRelease=63
	// CPU=50 < 80 and < 56; MEM=40 < 90 and < 63; Replicas=2 == MinReplicas=2 → hold
	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 5, CPUTarget: 80, RAMTarget: 90})

	if store.updated != nil {
		t.Fatalf("app should not be updated during hold: %#v", store.updated)
	}
	got, ok := m.lastDecision("app-1")
	if !ok {
		t.Fatal("expected persisted decision")
	}
	if got.Action != "hold" {
		t.Fatalf("decision action = %q, want hold", got.Action)
	}
}

// =============================================================================
// evaluate — scale_down cooldown
// =============================================================================

func TestEvaluate_ScaleDownCooldown(t *testing.T) {
	bolt := newTestBolt()
	previous := decision{AppID: "app-1", Action: "scale_down", EvaluatedAt: time.Now().UTC()}
	if err := bolt.Set(decisionBucket, "app-1", previous, decisionTTL); err != nil {
		t.Fatalf("seed previous decision: %v", err)
	}
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 3}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}},
		stats:      map[string]*core.ContainerStats{"ctr-1": {CPUPercent: 10, MemoryPercent: 10}},
	}
	m := testModule(store, runtime, bolt)

	// ScaleDownDelay=60 → still in cooldown since previous decision was just made
	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 5, CPUTarget: 80, RAMTarget: 90, ScaleDownDelay: 60})

	if store.updated != nil {
		t.Fatalf("app updated during scale-down cooldown: %#v", store.updated)
	}
	got, ok := m.lastDecision("app-1")
	if !ok {
		t.Fatal("expected persisted decision")
	}
	if got.Action != "cooldown" || got.Reason != "scale-down cooldown" {
		t.Fatalf("decision = %#v, want cooldown", got)
	}
}

// =============================================================================
// evaluate — UpdateApp error path
// =============================================================================

func TestEvaluate_UpdateAppError(t *testing.T) {
	bolt := newTestBolt()
	store := &autoscaleStore{
		app:       &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 1},
		updateErr: errors.New("db failure"),
	}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}, {ID: "ctr-2"}},
		stats: map[string]*core.ContainerStats{
			"ctr-1": {CPUPercent: 95, MemoryPercent: 40},
			"ctr-2": {CPUPercent: 90, MemoryPercent: 50},
		},
	}
	m := testModule(store, runtime, bolt)

	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 1, MaxReplicas: 3, CPUTarget: 80, RAMTarget: 90})

	got, ok := m.lastDecision("app-1")
	if !ok {
		t.Fatal("expected persisted decision")
	}
	if got.Action != "skip" {
		t.Fatalf("decision action = %q, want skip on update error", got.Action)
	}
	if !bytes.Contains([]byte(got.Reason), []byte("replica update failed")) {
		t.Fatalf("decision reason should mention update failure, got: %q", got.Reason)
	}
}

// =============================================================================
// evaluateAll — with enabled config (exercises m.evaluate call path)
// =============================================================================

func TestEvaluateAll_WithEnabledConfig(t *testing.T) {
	bolt := newTestBolt()
	if err := bolt.Set("autoscale", "app-1", autoscaleConfig{Enabled: true, MinReplicas: 1, MaxReplicas: 3, CPUTarget: 80, RAMTarget: 90}, 0); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 1}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}},
		stats:      map[string]*core.ContainerStats{"ctr-1": {CPUPercent: 95, MemoryPercent: 40}},
	}
	m := testModule(store, runtime, bolt)

	m.evaluateAll()

	// Should have a decision persisted since config is enabled and CPU is high
	got, ok := m.lastDecision("app-1")
	if !ok {
		t.Fatal("expected decision for enabled config")
	}
	// With CPU=95% and CPUTarget=80 → should scale up
	if got.Action != "scale_up" {
		t.Fatalf("expected scale_up for high CPU, got: %+v", got)
	}
}

// =============================================================================
// persist — nil core early return
// =============================================================================

func TestPersist_NilCore(t *testing.T) {
	m := &Module{}
	// Should not panic when core is nil
	m.persist(decision{AppID: "test"})
}

// =============================================================================
// lastDecision — nil core early return
// =============================================================================

func TestLastDecision_NilCore(t *testing.T) {
	m := &Module{}
	d, ok := m.lastDecision("test")
	if ok {
		t.Error("expected ok=false for nil core")
	}
	_ = d
}

// =============================================================================
// evaluateAll — config get error (non-KVNotFound) logs warning
// =============================================================================

func TestEvaluateAll_GetConfigErrorLogsWarning(t *testing.T) {
	bolt := newTestBolt()
	// Seed with an app entry in the autoscale bucket but make Get fail
	if err := bolt.Set("autoscale", "app-1", autoscaleConfig{Enabled: true}, 0); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	bolt.getErr = fmt.Errorf("decode failed: %w", core.ErrKVNotFound) // This triggers the inner err check
	// Actually, we want a non-KVNotFound error
	bolt2 := newTestBolt()
	bolt2.getErr = errors.New("bolt db error")
	bolt2.Set("autoscale", "app-1", autoscaleConfig{Enabled: true}, 0) // ignored due to getErr
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Replicas: 1}}

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	m := testModule(store, &autoscaleRuntime{}, bolt2)
	m.logger = logger

	m.evaluateAll()

	if !bytes.Contains(logs.Bytes(), []byte("autoscale: get config entry failed")) {
		t.Fatalf("expected config get warning, got logs: %q", logs.String())
	}
}


