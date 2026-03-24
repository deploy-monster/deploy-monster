package deploy

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_ID(t *testing.T) {
	m := New()
	if got := m.ID(); got != "deploy" {
		t.Errorf("Module.ID() = %q, want %q", got, "deploy")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if got := m.Name(); got != "Deploy Engine" {
		t.Errorf("Module.Name() = %q, want %q", got, "Deploy Engine")
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if got := m.Version(); got != "1.0.0" {
		t.Errorf("Module.Version() = %q, want %q", got, "1.0.0")
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}
	if deps[0] != "core.db" {
		t.Errorf("expected dependency %q, got %q", "core.db", deps[0])
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	routes := m.Routes()
	if routes != nil {
		t.Errorf("expected nil routes, got %v", routes)
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	events := m.Events()
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
}

func TestModule_Health_NoDocker(t *testing.T) {
	m := New()
	// Without Docker initialized, Health should return HealthDegraded
	health := m.Health()
	if health != core.HealthDegraded {
		t.Errorf("Module.Health() = %v, want %v (HealthDegraded)", health, core.HealthDegraded)
	}
}

func TestModule_Docker_Nil(t *testing.T) {
	m := New()
	if m.Docker() != nil {
		t.Error("Docker() should return nil when not initialized")
	}
}

func TestModule_Stop_NoDocker(t *testing.T) {
	m := New()
	err := m.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() should return nil when docker is nil, got: %v", err)
	}
}

func TestModule_ImplementsInterface(t *testing.T) {
	// Compile-time check that Module implements core.Module
	var _ core.Module = (*Module)(nil)
}

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}
