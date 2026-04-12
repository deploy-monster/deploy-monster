package notifications

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModuleIdentity(t *testing.T) {
	m := New()

	tests := []struct {
		method string
		got    string
		want   string
	}{
		{"ID", m.ID(), "notifications"},
		{"Name", m.Name(), "Notifications"},
		{"Version", m.Version(), "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.method, tt.got, tt.want)
			}
		})
	}
}

func TestModuleDependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()

	if len(deps) == 0 {
		t.Fatal("expected at least one dependency")
	}

	found := false
	for _, d := range deps {
		if d == "core.db" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected dependency on 'core.db', got %v", deps)
	}
}

func TestModuleHealth(t *testing.T) {
	m := New()
	health := m.Health()
	if health != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK (%v)", health, core.HealthOK)
	}
}

func TestModuleRoutes(t *testing.T) {
	m := New()
	routes := m.Routes()
	if routes != nil {
		t.Errorf("Routes() should return nil, got %v", routes)
	}
}

func TestModuleEvents(t *testing.T) {
	m := New()
	events := m.Events()
	if events != nil {
		t.Errorf("Events() should return nil, got %v", events)
	}
}

func TestModuleStop(t *testing.T) {
	m := New()
	err := m.Stop(context.TODO())
	if err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

func TestNewModuleReturnsNonNil(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModuleImplementsInterface(t *testing.T) {
	// Compile-time check that Module implements core.Module.
	var _ core.Module = (*Module)(nil)
}
