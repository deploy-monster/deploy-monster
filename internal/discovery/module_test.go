package discovery

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "discovery" {
		t.Errorf("ID() = %q, want discovery", m.ID())
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "Service Discovery" {
		t.Errorf("Name() = %q, want 'Service Discovery'", m.Name())
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want 1.0.0", m.Version())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()

	if len(deps) != 2 {
		t.Fatalf("Dependencies() length = %d, want 2", len(deps))
	}

	expected := map[string]bool{"deploy": true, "ingress": true}
	for _, dep := range deps {
		if !expected[dep] {
			t.Errorf("unexpected dependency: %q", dep)
		}
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	health := m.Health()

	if health != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", health)
	}
}

func TestModule_Health_String(t *testing.T) {
	m := New()
	if m.Health().String() != "ok" {
		t.Errorf("Health().String() = %q, want ok", m.Health().String())
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	routes := m.Routes()

	if routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	events := m.Events()

	if events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModule_ImplementsInterface(t *testing.T) {
	// Compile-time interface check.
	var _ core.Module = (*Module)(nil)
}

func TestNew_ReturnsNonNil(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModule_StopWithoutStart(t *testing.T) {
	m := New()
	// Stop should not panic when watcher is nil (module not started).
	if err := m.Stop(context.TODO()); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}
