package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_cleanOrphanContainers(t *testing.T) {
	store := newMockStore()
	store.apps["orphan-app"] = &core.Application{
		ID:   "orphan-app",
		Name: "orphan",
	}

	// Container for a valid app
	store.apps["valid-app"] = &core.Application{
		ID:   "valid-app",
		Name: "valid",
	}

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{
					ID:   "container-orphan",
					Name: "orphan-c",
					Labels: map[string]string{
						"monster.managed": "true",
						"monster.app.id":  "deleted-app",
					},
				},
				{
					ID:   "container-valid",
					Name: "valid-c",
					Labels: map[string]string{
						"monster.managed": "true",
						"monster.app.id":  "valid-app",
					},
				},
				{
					ID:     "container-no-label",
					Name:   "no-label-c",
					Labels: map[string]string{"monster.managed": "true"},
				},
			}, nil
		},
	}

	m := &Module{
		store:  store,
		docker: runtime,
		logger: slog.Default(),
	}

	m.cleanOrphanContainers(context.Background())

	if !runtime.removeCalled {
		t.Error("expected Remove to be called for orphan container")
	}
}

func TestModule_cleanOrphanContainers_ListError(t *testing.T) {
	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, fmt.Errorf("docker unavailable")
		},
	}

	m := &Module{
		store:  newMockStore(),
		docker: runtime,
		logger: slog.Default(),
	}

	// Should not panic
	m.cleanOrphanContainers(context.Background())
}

func TestModule_cleanOrphanContainers_RemoveError(t *testing.T) {
	store := newMockStore()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{
					ID:   "container-orphan-123",
					Name: "orphan",
					Labels: map[string]string{
						"monster.managed": "true",
						"monster.app.id":  "missing",
					},
				},
			}, nil
		},
		removeFn: func(_ context.Context, _ string, _ bool) error {
			return fmt.Errorf("remove failed")
		},
	}

	m := &Module{
		store:  store,
		docker: runtime,
		logger: slog.Default(),
	}

	// Should not panic
	m.cleanOrphanContainers(context.Background())
}

func TestModule_Health_PingError(t *testing.T) {
	runtime := &mockRuntime{pingErr: fmt.Errorf("docker down")}
	m := &Module{docker: runtime}

	if got := m.Health(); got != core.HealthDown {
		t.Errorf("Health() = %v, want HealthDown", got)
	}
}

func TestModule_Health_PingOK(t *testing.T) {
	runtime := &mockRuntime{}
	m := &Module{docker: runtime}

	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", got)
	}
}
