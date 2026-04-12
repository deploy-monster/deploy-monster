package deploy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// GracefulConfig / DefaultGracefulConfig
// ═══════════════════════════════════════════════════════════════════════════════

func TestDefaultGracefulConfig_Values(t *testing.T) {
	cfg := DefaultGracefulConfig()
	if cfg.DrainTimeout != 30*time.Second {
		t.Errorf("DrainTimeout = %v, want 30s", cfg.DrainTimeout)
	}
	if cfg.HealthCheckInterval != 500*time.Millisecond {
		t.Errorf("HealthCheckInterval = %v, want 500ms", cfg.HealthCheckInterval)
	}
	if cfg.HealthCheckTimeout != 5*time.Second {
		t.Errorf("HealthCheckTimeout = %v, want 5s", cfg.HealthCheckTimeout)
	}
	if cfg.HealthCheckPath != "/health" {
		t.Errorf("HealthCheckPath = %q, want /health", cfg.HealthCheckPath)
	}
	if cfg.StartupTimeout != 60*time.Second {
		t.Errorf("StartupTimeout = %v, want 60s", cfg.StartupTimeout)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// NewGracefulManager
// ═══════════════════════════════════════════════════════════════════════════════

func discardLoggerDeploy() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewGracefulManager(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	if g == nil {
		t.Fatal("NewGracefulManager returned nil")
	}
	if g.draining == nil {
		t.Error("draining map should be initialized")
	}
	if g.healthCheckers == nil {
		t.Error("healthCheckers map should be initialized")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// IsDraining
// ═══════════════════════════════════════════════════════════════════════════════

func TestGracefulManager_IsDraining_NotDraining(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	if g.IsDraining("c1") {
		t.Error("IsDraining should return false for unknown container")
	}
}

func TestGracefulManager_IsDraining_Draining(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	g.draining["c1"] = &DrainState{ContainerID: "c1", Done: make(chan struct{})}
	if !g.IsDraining("c1") {
		t.Error("IsDraining should return true for draining container")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StartDrain
// ═══════════════════════════════════════════════════════════════════════════════

func TestGracefulManager_StartDrain_Completed(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())

	done := make(chan error, 1)
	go func() {
		done <- g.StartDrain(context.Background(), "c1", 5*time.Second)
	}()

	// Allow goroutine to register the drain state
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if g.IsDraining("c1") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	g.CompleteDrain("c1")

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StartDrain returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartDrain did not complete")
	}
}

func TestGracefulManager_StartDrain_Timeout(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())

	err := g.StartDrain(context.Background(), "c2", 50*time.Millisecond)
	if err == nil {
		t.Fatal("StartDrain should return timeout error")
	}
	if err.Error() != "drain timeout" {
		t.Errorf("err = %v, want 'drain timeout'", err)
	}
	// Should have been cleaned up
	if g.IsDraining("c2") {
		t.Error("container should be removed from draining map after timeout")
	}
}

func TestGracefulManager_StartDrain_ContextCancelled(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- g.StartDrain(ctx, "c3", 5*time.Second)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("StartDrain should return context error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StartDrain did not return after ctx cancelled")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CompleteDrain
// ═══════════════════════════════════════════════════════════════════════════════

func TestGracefulManager_CompleteDrain_Unknown(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	// Should not panic for unknown container
	g.CompleteDrain("does-not-exist")
}

// ═══════════════════════════════════════════════════════════════════════════════
// WaitForHealthy
// ═══════════════════════════════════════════════════════════════════════════════

type healthyOnThirdRuntime struct {
	mockRuntime
	calls int
	mu    sync.Mutex
}

func (m *healthyOnThirdRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls >= 3 {
		return &core.ContainerStats{Health: "healthy", Running: true}, nil
	}
	return &core.ContainerStats{Health: "starting", Running: true}, nil
}

func TestGracefulManager_WaitForHealthy_Success(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	rt := &healthyOnThirdRuntime{}

	cfg := GracefulConfig{
		HealthCheckInterval: 20 * time.Millisecond,
		HealthCheckTimeout:  100 * time.Millisecond,
		HealthCheckPath:     "/health",
		StartupTimeout:      2 * time.Second,
	}

	err := g.WaitForHealthy(context.Background(), rt, "c-ok", 8080, cfg)
	if err != nil {
		t.Fatalf("WaitForHealthy returned %v, want nil", err)
	}
}

type alwaysStartingRuntime struct {
	mockRuntime
}

func (m *alwaysStartingRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{Health: "starting", Running: true}, nil
}

func TestGracefulManager_WaitForHealthy_Timeout(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	rt := &alwaysStartingRuntime{}

	cfg := GracefulConfig{
		HealthCheckInterval: 20 * time.Millisecond,
		HealthCheckTimeout:  50 * time.Millisecond,
		HealthCheckPath:     "/health",
		StartupTimeout:      100 * time.Millisecond,
	}

	err := g.WaitForHealthy(context.Background(), rt, "c-timeout", 8080, cfg)
	if err == nil {
		t.Fatal("WaitForHealthy should return timeout error")
	}
}

func TestGracefulManager_WaitForHealthy_ContextCancelled(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	rt := &alwaysStartingRuntime{}

	ctx, cancel := context.WithCancel(context.Background())

	cfg := GracefulConfig{
		HealthCheckInterval: 20 * time.Millisecond,
		HealthCheckTimeout:  50 * time.Millisecond,
		HealthCheckPath:     "/health",
		StartupTimeout:      5 * time.Second,
	}

	done := make(chan error, 1)
	go func() {
		done <- g.WaitForHealthy(ctx, rt, "c-ctx", 8080, cfg)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("WaitForHealthy should return ctx error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForHealthy did not return after ctx cancelled")
	}
}

type statsErrorRuntime struct {
	mockRuntime
	calls int
	mu    sync.Mutex
}

func (m *statsErrorRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls < 3 {
		return nil, errors.New("stats error")
	}
	return &core.ContainerStats{Health: "healthy", Running: true}, nil
}

func TestGracefulManager_WaitForHealthy_TransientStatsErrors(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	rt := &statsErrorRuntime{}

	cfg := GracefulConfig{
		HealthCheckInterval: 20 * time.Millisecond,
		HealthCheckTimeout:  50 * time.Millisecond,
		HealthCheckPath:     "/health",
		StartupTimeout:      2 * time.Second,
	}

	err := g.WaitForHealthy(context.Background(), rt, "c-err", 8080, cfg)
	if err != nil {
		t.Fatalf("WaitForHealthy returned %v, want nil", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// checkHealth — direct coverage of branches
// ═══════════════════════════════════════════════════════════════════════════════

type nilStatsRuntime struct {
	mockRuntime
}

func (m *nilStatsRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}

func TestGracefulManager_checkHealth_NilStats(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	cfg := GracefulConfig{HealthCheckTimeout: 100 * time.Millisecond}
	healthy, err := g.checkHealth(context.Background(), &nilStatsRuntime{}, "c", 8080, cfg)
	if err != nil {
		t.Fatalf("checkHealth returned %v", err)
	}
	if healthy {
		t.Error("checkHealth should return false for nil stats")
	}
}

type runningNoHealthRuntime struct {
	mockRuntime
}

func (m *runningNoHealthRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{Health: "", Running: true}, nil
}

func TestGracefulManager_checkHealth_NoHealthDefined(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	cfg := GracefulConfig{HealthCheckTimeout: 100 * time.Millisecond}
	healthy, err := g.checkHealth(context.Background(), &runningNoHealthRuntime{}, "c", 8080, cfg)
	if err != nil {
		t.Fatalf("checkHealth returned %v", err)
	}
	if !healthy {
		t.Error("checkHealth should return true when running and no health check defined")
	}
}

type errorStatsRuntime struct {
	mockRuntime
}

func (m *errorStatsRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, errors.New("stats broken")
}

func TestGracefulManager_checkHealth_StatsError(t *testing.T) {
	g := NewGracefulManager(discardLoggerDeploy())
	cfg := GracefulConfig{HealthCheckTimeout: 100 * time.Millisecond}
	healthy, err := g.checkHealth(context.Background(), &errorStatsRuntime{}, "c", 8080, cfg)
	if err == nil {
		t.Fatal("checkHealth should propagate stats error")
	}
	if healthy {
		t.Error("checkHealth should return false on error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ConnectionTracker
// ═══════════════════════════════════════════════════════════════════════════════

func TestConnectionTracker_IncrementDecrement(t *testing.T) {
	ct := NewConnectionTracker()
	if got := ct.Active("c1"); got != 0 {
		t.Errorf("Active = %d, want 0", got)
	}

	ct.Increment("c1")
	ct.Increment("c1")
	ct.Increment("c1")
	if got := ct.Active("c1"); got != 3 {
		t.Errorf("Active after 3 Increments = %d, want 3", got)
	}

	ct.Decrement("c1")
	if got := ct.Active("c1"); got != 2 {
		t.Errorf("Active after Decrement = %d, want 2", got)
	}

	ct.Decrement("c1")
	ct.Decrement("c1")
	if got := ct.Active("c1"); got != 0 {
		t.Errorf("Active after all Decrements = %d, want 0", got)
	}

	// Decrement at zero should stay at zero
	ct.Decrement("c1")
	if got := ct.Active("c1"); got != 0 {
		t.Errorf("Active after decrement-below-zero = %d, want 0", got)
	}
}

func TestConnectionTracker_MultipleContainers(t *testing.T) {
	ct := NewConnectionTracker()
	ct.Increment("c1")
	ct.Increment("c2")
	ct.Increment("c2")
	ct.Increment("c3")

	if got := ct.Active("c1"); got != 1 {
		t.Errorf("c1 Active = %d, want 1", got)
	}
	if got := ct.Active("c2"); got != 2 {
		t.Errorf("c2 Active = %d, want 2", got)
	}
	if got := ct.Active("c3"); got != 1 {
		t.Errorf("c3 Active = %d, want 1", got)
	}
	if got := ct.Active("unknown"); got != 0 {
		t.Errorf("unknown Active = %d, want 0", got)
	}
}
