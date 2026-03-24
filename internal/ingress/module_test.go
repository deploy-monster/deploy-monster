package ingress

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_New(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("expected non-nil Module")
	}
}

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "ingress" {
		t.Errorf("expected ID 'ingress', got %q", m.ID())
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "Ingress Gateway" {
		t.Errorf("expected Name 'Ingress Gateway', got %q", m.Name())
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", m.Version())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	if deps[0] != "core.db" {
		t.Errorf("expected first dependency 'core.db', got %q", deps[0])
	}
	if deps[1] != "deploy" {
		t.Errorf("expected second dependency 'deploy', got %q", deps[1])
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	routes := m.Routes()
	if routes != nil {
		t.Error("expected nil routes")
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	events := m.Events()
	if events != nil {
		t.Error("expected nil events")
	}
}

func TestModule_Health_Uninitialized(t *testing.T) {
	m := New()
	// router is nil before Init
	if m.Health() != core.HealthDown {
		t.Errorf("expected HealthDown for uninitialized module, got %v", m.Health())
	}
}

func TestModule_Health_WithRouter(t *testing.T) {
	m := New()
	m.router = NewRouteTable()

	if m.Health() != core.HealthOK {
		t.Errorf("expected HealthOK when router is set, got %v", m.Health())
	}
}

func TestModule_Router(t *testing.T) {
	m := New()
	// Before init, Router() returns nil
	if m.Router() != nil {
		t.Error("expected nil router before Init")
	}

	rt := NewRouteTable()
	m.router = rt

	if m.Router() != rt {
		t.Error("expected Router() to return the route table")
	}
}

func TestModule_Stop_NilServers(t *testing.T) {
	m := New()
	// Stop with nil servers should not panic
	err := m.Stop(context.TODO())
	if err != nil {
		t.Errorf("expected no error from Stop with nil servers, got %v", err)
	}
}
