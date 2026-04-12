package strategies

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// fakeCanaryController records every AdjustWeight / Finalize call so
// tests can assert the strategy walked the phase schedule in order.
type fakeCanaryController struct {
	mu          sync.Mutex
	calls       []int // percents, in call order
	finalized   atomic.Bool
	adjustErr   error // if non-nil returned from AdjustWeight
	failAtPhase int   // percent value that should fail; 0 disables
}

func (f *fakeCanaryController) AdjustWeight(_ context.Context, _ *core.Application, _, _ string, percent int) error {
	f.mu.Lock()
	f.calls = append(f.calls, percent)
	f.mu.Unlock()
	if f.failAtPhase != 0 && percent == f.failAtPhase {
		return f.adjustErr
	}
	return nil
}

func (f *fakeCanaryController) Finalize(_ context.Context, _ *core.Application, _, _ string) error {
	f.finalized.Store(true)
	return nil
}

func (f *fakeCanaryController) Calls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.calls))
	copy(out, f.calls)
	return out
}

func newCanaryPlan(runtime core.ContainerRuntime, oldContainer string, controller CanaryController) *DeployPlan {
	return &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment:     &core.Deployment{Version: 11},
		NewImage:       "nginx:latest",
		OldContainerID: oldContainer,
		Runtime:        runtime,
		Store:          &mockStore{},
		Graceful: &GracefulConfig{
			HealthCheckInterval: 5 * time.Millisecond,
			StartupTimeout:      500 * time.Millisecond,
			CanaryController:    controller,
			// Fast phases so tests run in milliseconds.
			CanaryPlan: []CanaryWeight{
				{Percent: 10, Dwell: 5 * time.Millisecond},
				{Percent: 50, Dwell: 5 * time.Millisecond},
				{Percent: 100, Dwell: 0},
			},
		},
	}
}

func TestCanary_Name(t *testing.T) {
	if (&Canary{}).Name() != "canary" {
		t.Errorf("Name() = %q, want canary", (&Canary{}).Name())
	}
}

func TestNew_ReturnsCanary(t *testing.T) {
	s := New("canary")
	if _, ok := s.(*Canary); !ok {
		t.Errorf("New(\"canary\") = %T, want *Canary", s)
	}
}

func TestCanary_Execute_HappyPath_PhasesInOrder(t *testing.T) {
	rt := &bgRuntime{}
	ctrl := &fakeCanaryController{}
	plan := newCanaryPlan(rt, "stable-1", ctrl)

	if err := (&Canary{}).Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := ctrl.Calls()
	want := []int{10, 50, 100}
	if len(got) != len(want) {
		t.Fatalf("AdjustWeight calls = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("phase %d percent = %d, want %d", i, got[i], want[i])
		}
	}
	if !ctrl.finalized.Load() {
		t.Error("Finalize was not called after 100%% phase")
	}
	if !rt.stopCalled {
		t.Error("stable container was not drained at end of rollout")
	}
}

func TestCanary_Execute_NoController_StillPromotes(t *testing.T) {
	// A nil controller is valid — the strategy advances through
	// phases without touching any route table and still promotes
	// the new container at the end.
	rt := &bgRuntime{}
	plan := newCanaryPlan(rt, "stable-1", nil)

	if err := (&Canary{}).Execute(context.Background(), plan); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !rt.createCalled {
		t.Error("CreateAndStart was not called")
	}
	if !rt.stopCalled {
		t.Error("stable container was not drained")
	}
}

func TestCanary_Execute_HealthTimeout(t *testing.T) {
	rt := &bgRuntime{
		statsFn: func() (*core.ContainerStats, error) {
			return &core.ContainerStats{Health: "unhealthy", Running: true}, nil
		},
	}
	ctrl := &fakeCanaryController{}
	plan := newCanaryPlan(rt, "stable-1", ctrl)
	plan.Graceful.StartupTimeout = 30 * time.Millisecond

	err := (&Canary{}).Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("Execute returned nil, want health timeout error")
	}
	// On health failure the strategy runs its defensive rollback,
	// which calls AdjustWeight(0) once to reset any stray state.
	// No other phase percents should appear.
	calls := ctrl.Calls()
	if len(calls) != 1 || calls[0] != 0 {
		t.Errorf("calls = %v, want [0] (rollback reset only)", calls)
	}
	if ctrl.finalized.Load() {
		t.Error("Finalize ran on health failure")
	}
	if !rt.removeCalled {
		t.Error("failed canary was not removed")
	}
}

func TestCanary_Execute_PhaseFailure_RollsBack(t *testing.T) {
	rt := &bgRuntime{}
	adjustErr := errors.New("weight adjust rejected by router")
	ctrl := &fakeCanaryController{
		failAtPhase: 50,
		adjustErr:   adjustErr,
	}
	plan := newCanaryPlan(rt, "stable-1", ctrl)

	err := (&Canary{}).Execute(context.Background(), plan)
	if err == nil || !errors.Is(err, adjustErr) {
		t.Errorf("err = %v, want wrap of %v", err, adjustErr)
	}

	// Phase 1 (10%) succeeds, phase 2 (50%) fails, then rollback
	// calls AdjustWeight(0). Finalize must NOT run.
	calls := ctrl.Calls()
	if len(calls) < 3 || calls[0] != 10 || calls[1] != 50 || calls[len(calls)-1] != 0 {
		t.Errorf("calls = %v, want sequence starting 10,50 and ending with 0", calls)
	}
	if ctrl.finalized.Load() {
		t.Error("Finalize ran despite phase failure")
	}
	if !rt.removeCalled {
		t.Error("canary container was not removed on rollback")
	}
}

func TestCanary_Execute_CancelledDuringDwell(t *testing.T) {
	rt := &bgRuntime{}
	ctrl := &fakeCanaryController{}
	plan := newCanaryPlan(rt, "stable-1", ctrl)
	// Make phase 1 dwell long enough for the cancel to land inside it.
	plan.Graceful.CanaryPlan = []CanaryWeight{
		{Percent: 10, Dwell: 5 * time.Second},
		{Percent: 100, Dwell: 0},
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)

	err := (&Canary{}).Execute(ctx, plan)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled in chain", err)
	}
	calls := ctrl.Calls()
	if len(calls) == 0 || calls[0] != 10 {
		t.Errorf("first call = %v, want 10%% phase to run before cancel", calls)
	}
	// Rollback should have called AdjustWeight(0) as the last step.
	if calls[len(calls)-1] != 0 {
		t.Errorf("last call = %d, want 0 (rollback reset)", calls[len(calls)-1])
	}
	if !rt.removeCalled {
		t.Error("canary container was not removed after cancel")
	}
}

func TestCanary_Execute_CreateFails(t *testing.T) {
	createErr := errors.New("image pull failed")
	rt := &bgRuntime{}
	rt.createAndStartFn = func(_ context.Context, _ core.ContainerOpts) (string, error) {
		return "", createErr
	}
	plan := newCanaryPlan(rt, "stable-1", &fakeCanaryController{})

	err := (&Canary{}).Execute(context.Background(), plan)
	if err == nil || !errors.Is(err, createErr) {
		t.Errorf("err = %v, want wrap of %v", err, createErr)
	}
	if rt.stopCalled {
		t.Error("stable container was touched despite canary start failure")
	}
}

func TestDefaultCanaryPlan_Shape(t *testing.T) {
	// Sanity check the spec-defined schedule so a regression to
	// something like [10, 100] gets caught.
	want := []int{10, 50, 100}
	if len(DefaultCanaryPlan) != 3 {
		t.Fatalf("len(DefaultCanaryPlan) = %d, want 3", len(DefaultCanaryPlan))
	}
	for i, phase := range DefaultCanaryPlan {
		if phase.Percent != want[i] {
			t.Errorf("phase %d percent = %d, want %d", i, phase.Percent, want[i])
		}
	}
}
