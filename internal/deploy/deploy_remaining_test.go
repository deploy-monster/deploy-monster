package deploy

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Module — basic identity (module.go)
// =============================================================================

func TestDeployModule_ID(t *testing.T) {
	m := &Module{}
	if m.ID() == "" {
		t.Error("expected non-empty ID")
	}
	if m.Name() == "" {
		t.Error("expected non-empty Name")
	}
	if m.Version() == "" {
		t.Error("expected non-empty Version")
	}
}

// =============================================================================
// SetRegistryAuth — edge cases (docker.go:62)
// =============================================================================

func TestSetRegistryAuth_EmptyBoth(t *testing.T) {
	mgr := &DockerManager{}
	err := mgr.SetRegistryAuth("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr.registryAuth != "" {
		t.Error("expected empty registry auth")
	}
}

func TestSetRegistryAuth_MissingOne(t *testing.T) {
	mgr := &DockerManager{}
	err := mgr.SetRegistryAuth("user", "")
	if err == nil || !strings.Contains(err.Error(), "both") {
		t.Fatalf("expected 'both' error, got: %v", err)
	}
}

func TestSetRegistryAuth_Valid(t *testing.T) {
	mgr := &DockerManager{}
	err := mgr.SetRegistryAuth("user", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr.registryAuth == "" {
		t.Error("expected non-empty registry auth")
	}
}

// =============================================================================
// AutoRollbackManager — Start with nil events is safe
// =============================================================================

func TestAutoRollbackManager_StartNilEvents(t *testing.T) {
	mgr := NewAutoRollbackManager(nil, nil, nil, slog.Default())
	mgr.Start() // Should not panic
	mgr.Stop()
}

// =============================================================================
// AutoRollbackManager — Start with events, then stop
// =============================================================================

func TestAutoRollbackManager_StartCloseStop(t *testing.T) {
	eb := core.NewEventBus(slog.Default())
	mgr := NewAutoRollbackManager(nil, nil, eb, slog.Default())
	mgr.Start()
	mgr.Stop()
	mgr.Stop() // Second stop is safe
}

// =============================================================================
// AutoRestarter — Stop without Start (autorestart.go:54)
// =============================================================================

func TestAutoRestarter_StopWithoutStart(t *testing.T) {
	ar := NewAutoRestarter(nil, nil, nil, slog.Default())
	ar.Stop() // Should not panic
}

// =============================================================================
// AutoRollbackManager — Wait is safe (autorollback.go:152)
// =============================================================================

func TestAutoRollbackManager_Wait(t *testing.T) {
	mgr := NewAutoRollbackManager(nil, nil, nil, slog.Default())
	mgr.Wait() // Should not hang
}

// =============================================================================
// DockerManager — SetResourceDefaults
// =============================================================================

func TestDockerManager_SetResourceDefaultsExtra(t *testing.T) {
	mgr := &DockerManager{}
	mgr.SetResourceDefaults(50000, 256)
	if mgr.defaultCPU != 50000 {
		t.Errorf("expected CPU 50000, got %d", mgr.defaultCPU)
	}
	if mgr.defaultMemMB != 256 {
		t.Errorf("expected mem 256, got %d", mgr.defaultMemMB)
	}
}
