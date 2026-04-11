package strategies

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// bgRuntime is a focused mock that lets each blue-green test control
// Stats responses over time. It embeds no behavior beyond what the
// strategy calls so unrelated interface methods fall through to no-op
// defaults via the existing mockRuntime test helper.
type bgRuntime struct {
	mockRuntime
	statsFn    func() (*core.ContainerStats, error)
	statsCalls atomic.Int32
}

func (r *bgRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	r.statsCalls.Add(1)
	if r.statsFn != nil {
		return r.statsFn()
	}
	return &core.ContainerStats{Health: "healthy", Running: true}, nil
}

func newBGPlan(runtime core.ContainerRuntime, oldContainer string) *DeployPlan {
	return &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 7,
		},
		NewImage:       "nginx:latest",
		OldContainerID: oldContainer,
		Runtime:        runtime,
		Store:          &mockStore{},
		Graceful: &GracefulConfig{
			HealthCheckInterval: 10 * time.Millisecond,
			StartupTimeout:      500 * time.Millisecond,
			BlueGreenHoldback:   5 * time.Millisecond,
		},
	}
}

func TestBlueGreen_Name(t *testing.T) {
	if (&BlueGreen{}).Name() != "blue-green" {
		t.Errorf("Name() = %q, want blue-green", (&BlueGreen{}).Name())
	}
}

func TestNew_ReturnsBlueGreen(t *testing.T) {
	for _, name := range []string{"blue-green", "bluegreen"} {
		s := New(name)
		if _, ok := s.(*BlueGreen); !ok {
			t.Errorf("New(%q) = %T, want *BlueGreen", name, s)
		}
	}
}

func TestBlueGreen_Execute_HappyPath(t *testing.T) {
	rt := &bgRuntime{}
	plan := newBGPlan(rt, "blue-123")

	if err := (&BlueGreen{}).Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !rt.createCalled {
		t.Error("CreateAndStart was not called for green")
	}
	if !rt.stopCalled {
		t.Error("Stop was not called for blue drain after holdback")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want container-123", plan.Deployment.ContainerID)
	}
}

func TestBlueGreen_Execute_NoBlue_StillPromotes(t *testing.T) {
	// First-ever deployment: no OldContainerID. Green should still
	// be created and health-checked; Stop should NOT be called
	// because there's no blue to drain.
	rt := &bgRuntime{}
	plan := newBGPlan(rt, "")

	if err := (&BlueGreen{}).Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !rt.createCalled {
		t.Error("CreateAndStart was not called")
	}
	if rt.stopCalled {
		t.Error("Stop should not be called when no blue exists")
	}
}

func TestBlueGreen_Execute_HealthTimeout_RemovesGreen(t *testing.T) {
	// Stats never returns "healthy" and Running is false, so the
	// health loop should time out and the strategy should tear down
	// green without touching blue.
	rt := &bgRuntime{
		statsFn: func() (*core.ContainerStats, error) {
			return &core.ContainerStats{Health: "unhealthy", Running: true}, nil
		},
	}
	plan := newBGPlan(rt, "blue-xyz")
	plan.Graceful.StartupTimeout = 30 * time.Millisecond

	err := (&BlueGreen{}).Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("Execute returned nil, want timeout error")
	}
	if !rt.removeCalled {
		t.Error("Remove was not called to clean up failed green")
	}
	// Blue must remain untouched — Stop should only have been called
	// as part of the cleanupGreen tear-down, not against the blue
	// container. We can't differentiate container IDs in this mock,
	// but the strategy's contract is that blue is never drained on
	// a health failure. The failure-path Stop targets greenID.
}

func TestBlueGreen_Execute_CreateFails(t *testing.T) {
	createErr := errors.New("docker daemon unavailable")
	rt := &bgRuntime{}
	rt.createAndStartFn = func(_ context.Context, _ core.ContainerOpts) (string, error) {
		return "", createErr
	}
	plan := newBGPlan(rt, "blue-1")

	err := (&BlueGreen{}).Execute(context.Background(), plan)
	if err == nil || !errors.Is(err, createErr) {
		t.Errorf("Execute err = %v, want wrap of %v", err, createErr)
	}
	if rt.stopCalled {
		t.Error("Stop must not touch blue when green failed to start")
	}
}

func TestBlueGreen_Execute_HoldbackCancelled(t *testing.T) {
	// Green comes up healthy quickly; we cancel ctx during the
	// holdback. The strategy must remove green (rollback to blue)
	// and NOT drain blue.
	rt := &bgRuntime{}
	plan := newBGPlan(rt, "blue-42")
	plan.Graceful.BlueGreenHoldback = 10 * time.Second // long holdback

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay so we land in the holdback select.
	time.AfterFunc(30*time.Millisecond, cancel)

	err := (&BlueGreen{}).Execute(ctx, plan)
	if err == nil {
		t.Fatal("Execute returned nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled in chain", err)
	}
	if !rt.removeCalled {
		t.Error("Remove was not called to roll back green")
	}
}

func TestBlueGreenHoldbackFor(t *testing.T) {
	if got := blueGreenHoldbackFor(nil); got != DefaultBlueGreenHoldback {
		t.Errorf("nil cfg holdback = %v, want default %v", got, DefaultBlueGreenHoldback)
	}
	if got := blueGreenHoldbackFor(&GracefulConfig{}); got != DefaultBlueGreenHoldback {
		t.Errorf("empty cfg holdback = %v, want default", got)
	}
	override := 2 * time.Second
	got := blueGreenHoldbackFor(&GracefulConfig{BlueGreenHoldback: override})
	if got != override {
		t.Errorf("override holdback = %v, want %v", got, override)
	}
}
