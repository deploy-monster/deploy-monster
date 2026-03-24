package deploy

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestNewAutoRestarter(t *testing.T) {
	events := core.NewEventBus(nil)
	logger := slog.Default()

	t.Run("creates non-nil restarter", func(t *testing.T) {
		ar := NewAutoRestarter(nil, nil, events, logger)
		if ar == nil {
			t.Fatal("NewAutoRestarter returned nil")
		}
	})

	t.Run("fields are set", func(t *testing.T) {
		store := newMockStore()
		runtime := &mockRuntime{}
		ar := NewAutoRestarter(runtime, store, events, logger)
		if ar.runtime != runtime {
			t.Error("runtime field not set")
		}
		if ar.store != store {
			t.Error("store field not set")
		}
		if ar.events != events {
			t.Error("events field not set")
		}
		if ar.logger != logger {
			t.Error("logger field not set")
		}
		if ar.maxRetries != 5 {
			t.Errorf("maxRetries = %d, want 5", ar.maxRetries)
		}
	})
}

func TestAutoRestarter_CheckCrashed_NilRuntime(t *testing.T) {
	events := core.NewEventBus(nil)
	logger := slog.Default()
	ar := NewAutoRestarter(nil, nil, events, logger)

	// Should not panic with nil runtime
	ar.checkCrashed()
}

func TestAutoRestarter_CheckCrashed_NoContainers(t *testing.T) {
	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{}, nil
		},
	}
	events := core.NewEventBus(nil)
	logger := slog.Default()
	store := newMockStore()
	ar := NewAutoRestarter(runtime, store, events, logger)

	// Should not panic with empty container list
	ar.checkCrashed()
}

func TestAutoRestarter_CheckCrashed_ListError(t *testing.T) {
	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, context.DeadlineExceeded
		},
	}
	events := core.NewEventBus(nil)
	logger := slog.Default()
	store := newMockStore()
	ar := NewAutoRestarter(runtime, store, events, logger)

	// Should not panic when ListByLabels fails
	ar.checkCrashed()
}
