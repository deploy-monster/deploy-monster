package database

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// init() — covers module.go:10 (RegisterModule factory call)
// The init() function runs on package import and registers a module factory.
// We verify the factory produces a valid Module.
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m

	if m.ID() != "database" {
		t.Errorf("ID() = %q, want database", m.ID())
	}
	if m.Name() != "Database Manager" {
		t.Errorf("Name() = %q, want Database Manager", m.Name())
	}
}

func TestModule_FullLifecycle(t *testing.T) {
	c := &core.Core{
		Logger: testLogger(),
	}

	m := New()

	// Init
	if err := m.Init(t.Context(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if m.core != c {
		t.Error("core not set after Init")
	}
	if m.logger == nil {
		t.Error("logger not set after Init")
	}

	// Start
	if err := m.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Health — no container runtime configured, should be degraded
	if h := m.Health(); h != core.HealthDegraded {
		t.Errorf("Health() = %v, want HealthDegraded (no container runtime)", h)
	}

	// Stop
	if err := m.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
