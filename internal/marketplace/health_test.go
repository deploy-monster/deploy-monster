package marketplace

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_Health_NilRegistry(t *testing.T) {
	m := &Module{}
	// Before Init, Health should report OK (not yet started)
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK before Init (registry nil)")
	}
}

func TestModule_Health_EmptyRegistry(t *testing.T) {
	m := &Module{
		registry: NewTemplateRegistry(),
	}
	if m.Health() != core.HealthDegraded {
		t.Error("expected HealthDegraded when registry has no templates")
	}
}

func TestModule_Health_WithTemplates(t *testing.T) {
	reg := NewTemplateRegistry()
	reg.LoadBuiltins()

	m := &Module{
		registry: reg,
	}
	if m.Health() != core.HealthOK {
		t.Errorf("expected HealthOK with loaded templates, got %v", m.Health())
	}
}
