package deploy

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// module.go:12-14 — init() registration
// =============================================================================

func TestCov_ModuleInitRegistration(t *testing.T) {
	m := New()
	if m.ID() != "deploy" {
		t.Errorf("ID = %q", m.ID())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version = %q", m.Version())
	}
}

// =============================================================================
// module.go:54-89 — Module.Init nil store
// =============================================================================

func TestCov_ModuleInitNilStore(t *testing.T) {
	m := New()
	err := m.Init(context.Background(), &core.Core{Logger: slog.Default()})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
}

// =============================================================================
// module.go:91-123 — Start/Stop/Health edge cases
// =============================================================================

func TestCov_ModuleStopAllNil(t *testing.T) {
	m := New()
	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCov_ModuleHealthDegraded(t *testing.T) {
	m := New()
	if m.Health() != core.HealthDegraded {
		t.Errorf("Health = %v", m.Health())
	}
}

func TestCov_ModuleHealthDown(t *testing.T) {
	m := New()
	m.docker = &mockRuntime{pingErr: errors.New("docker down")}
	if got := m.Health(); got != core.HealthDown {
		t.Errorf("Health = %v, want HealthDown", got)
	}
}

func TestCov_ModuleStartNilDocker(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	err := m.Start(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// autorestart.go:90-140 — handleCrash with nil runtime or events
// =============================================================================

func TestCov_AutoRestarterHandleCrashNilRuntime(t *testing.T) {
	ar := NewAutoRestarter(nil, newMockStore(), nil, slog.Default())
	ar.maxRetries = 1
	ar.retryDelay = func(attempt int) time.Duration { return time.Millisecond }
	ar.handleCrash(context.Background(), "app1", "c1")
}

func TestCov_AutoRestarterStartStop(t *testing.T) {
	ar := NewAutoRestarter(nil, nil, nil, nil)
	ar.Start()
	ar.Stop()
}

// =============================================================================
// autorollback.go:83-130 — Start/Stop with nil events
// =============================================================================

func TestCov_AutoRollbackStartNilEvents(t *testing.T) {
	ar := NewAutoRollbackManager(nil, nil, nil, nil)
	ar.Start()
	ar.Stop()
}

func TestCov_AutoRollbackIsClosedDefault(t *testing.T) {
	ar := NewAutoRollbackManager(nil, nil, nil, nil)
	if ar.isClosed() {
		t.Error("should not be closed")
	}
}

func TestCov_AutoRollbackIsClosedAfterStop(t *testing.T) {
	ar := NewAutoRollbackManager(nil, nil, nil, nil)
	ar.Stop()
	if !ar.isClosed() {
		t.Error("should be closed")
	}
}

func TestCov_AutoRollbackRunCtxFallback(t *testing.T) {
	ar := NewAutoRollbackManager(nil, nil, nil, nil)
	ctx := ar.runCtx(context.Background())
	if ctx == nil {
		t.Error("ctx should not be nil")
	}
}

func TestCov_AutoRollbackRunCtxNilAll(t *testing.T) {
	ar := NewAutoRollbackManager(nil, nil, nil, nil)
	ctx := ar.runCtx(nil)
	if ctx == nil {
		t.Error("ctx should not be nil")
	}
}

// =============================================================================
// docker.go:62-80 — SetRegistryAuth edge cases
// =============================================================================

func TestCov_DockerSetRegistryAuthPartial(t *testing.T) {
	d := &DockerManager{}
	if err := d.SetRegistryAuth("u", ""); err == nil {
		t.Fatal("expected error")
	}
	if err := d.SetRegistryAuth("", "p"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCov_DockerSetRegistryAuthEmpty(t *testing.T) {
	d := &DockerManager{}
	if err := d.SetRegistryAuth("", ""); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// module.go:254-258 — shortContainerID
// =============================================================================

func TestCov_ShortContainerID(t *testing.T) {
	if got := shortContainerID("abc"); got != "abc" {
		t.Errorf("got %q", got)
	}
	if got := shortContainerID("abcdef1234567890"); got != "abcdef123456" {
		t.Errorf("got %q", got)
	}
}

// =============================================================================
// rollback.go:25+ — error paths using mockStore
// =============================================================================

func TestCov_RollbackListDeploymentsError(t *testing.T) {
	s := newMockStore()
	s.listDeploymentsErr = errors.New("db err")
	eng := NewRollbackEngine(s, nil, nil)
	_, err := eng.Rollback(context.Background(), "a1", 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCov_RollbackVersionNotFound(t *testing.T) {
	s := newMockStore()
	s.deployments = []core.Deployment{{Version: 2, Image: "img:v2"}}
	eng := NewRollbackEngine(s, nil, nil)
	_, err := eng.Rollback(context.Background(), "a1", 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCov_RollbackEmptyImage(t *testing.T) {
	s := newMockStore()
	s.deployments = []core.Deployment{{Version: 1, Image: ""}}
	eng := NewRollbackEngine(s, nil, nil)
	_, err := eng.Rollback(context.Background(), "a1", 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCov_RollbackGetAppError(t *testing.T) {
	s := newMockStore()
	s.deployments = []core.Deployment{{Version: 1, Image: "img:v1"}}
	s.getAppErr = errors.New("not found")
	eng := NewRollbackEngine(s, nil, nil)
	_, err := eng.Rollback(context.Background(), "a1", 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

// =============================================================================
// module.go:136-176 — reclaimStaleDeployments list error
// =============================================================================

func TestCov_ReclaimStaleDeploymentsListError(t *testing.T) {
	s := newMockStore()
	s.listByStatusErr = errors.New("db error")
	m := New()
	m.store = s
	m.logger = slog.Default()
	m.reclaimStaleDeployments(context.Background())
}

// =============================================================================
// module.go:179-217 — cleanOrphanContainers list error
// =============================================================================

func TestCov_CleanOrphanContainersListError(t *testing.T) {
	m := New()
	m.docker = &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, errors.New("docker error")
		},
	}
	m.logger = slog.Default()
	m.cleanOrphanContainers(context.Background())
}

// =============================================================================
// autorestart.go:90-140 — handleCrash with failing restarts
// =============================================================================

func TestCov_HandleCrashMaxRetries(t *testing.T) {
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			return errors.New("fail")
		},
	}
	ar := NewAutoRestarter(runtime, newMockStore(), nil, slog.Default())
	ar.maxRetries = 2
	ar.retryDelay = func(attempt int) time.Duration { return time.Millisecond }
	ar.handleCrash(context.Background(), "app1", "c1")
}

func TestCov_HandleCrashPublishesEvent(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	ar := NewAutoRestarter(nil, newMockStore(), events, slog.Default())
	ar.maxRetries = 1
	ar.retryDelay = func(attempt int) time.Duration { return time.Millisecond }
	ar.handleCrash(context.Background(), "app1", "c1")
}

// =============================================================================
// docker.go:105-107 — CreateAndStart ValidateVolumePaths error
// =============================================================================

func TestCov_DockerCreateAndStartVolumePathError(t *testing.T) {
	d := &DockerManager{}
	_, err := d.CreateAndStart(context.Background(), core.ContainerOpts{
		Name:    "test",
		Image:   "nginx",
		Volumes: map[string]string{"/../etc": "/etc"}, // path traversal
	})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}
