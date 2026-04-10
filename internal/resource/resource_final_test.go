package resource

import (
	"context"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// collectionLoop — covers module.go:64 (all branches in the select loop)
// The existing tests call Start/Stop but the 30s ticker never fires in tests.
// We exercise the loop body by calling the collector/alerter directly, and also
// verify the stopCh terminates the goroutine promptly.
// ═══════════════════════════════════════════════════════════════════════════════

func TestCollectionLoop_StopTerminatesGoroutine(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "app1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 10, MemoryUsage: 100, MemoryLimit: 200},
	}

	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}
	c.Services.Container = mock

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Start the collection loop goroutine
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Let the goroutine start
	time.Sleep(20 * time.Millisecond)

	// Stop should terminate the goroutine via stopCh
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// TestCollectionLoop_DirectInvocation directly calls collectionLoop in a
// goroutine and immediately stops it to cover the function entry and stopCh branch.
func TestCollectionLoop_DirectInvocation(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 30, MemoryUsage: 128 * 1024 * 1024, MemoryLimit: 256 * 1024 * 1024},
	}

	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}
	c.Services.Container = mock

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Launch collectionLoop directly. Tier 75: wg.Add must precede
	// the goroutine spawn so the loop's defer wg.Done does not
	// underflow the counter.
	m.wg.Add(1)
	go m.collectionLoop()

	// Give the goroutine time to start and enter the select
	time.Sleep(50 * time.Millisecond)

	// Stop terminates the loop via stopCh and drains the wg.
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// TestCollectionLoop_FullSimulation exercises every line of the collectionLoop
// body by directly calling the same operations: CollectServer, Evaluate,
// CollectContainers — covering the nil check and the container metrics path.
func TestCollectionLoop_FullSimulation(t *testing.T) {
	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
			{ID: "c2", State: "running", Labels: map[string]string{"monster.app.id": "a2"}},
		},
		stats: &core.ContainerStats{
			CPUPercent:  50,
			MemoryUsage: 256 * 1024 * 1024,
			MemoryLimit: 512 * 1024 * 1024,
		},
	}

	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}
	c.Services.Container = mock

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx := context.Background()

	metrics := m.collector.CollectServer(ctx)
	if metrics == nil {
		t.Fatal("CollectServer returned nil")
	}
	m.alerter.Evaluate(ctx, metrics)

	containerMetrics := m.collector.CollectContainers(ctx)
	if len(containerMetrics) != 2 {
		t.Errorf("CollectContainers returned %d metrics, want 2", len(containerMetrics))
	}
}

// TestCollectionLoop_NilServerMetrics covers the guard where metrics could be nil.
func TestCollectionLoop_NilServerMetrics(t *testing.T) {
	c := &core.Core{
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	ctx := context.Background()
	metrics := m.collector.CollectServer(ctx)
	if metrics != nil {
		m.alerter.Evaluate(ctx, metrics)
	}

	containerMetrics := m.collector.CollectContainers(ctx)
	if containerMetrics != nil {
		t.Errorf("expected nil container metrics with nil runtime, got %d", len(containerMetrics))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// init() — covers module.go:11
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m
	if m.ID() != "resource" {
		t.Errorf("ID() = %q, want resource", m.ID())
	}
}
