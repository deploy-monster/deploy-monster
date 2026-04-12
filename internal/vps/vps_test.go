package vps

import (
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Module Tests
// =============================================================================

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "vps" {
		t.Errorf("ID = %q, want %q", m.ID(), "vps")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "VPS Provider Manager" {
		t.Errorf("Name = %q", m.Name())
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("Version = %q", m.Version())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies = %v", deps)
	}
}

func TestModule_Routes_Nil(t *testing.T) {
	m := New()
	if m.Routes() != nil {
		t.Error("Routes should be nil")
	}
}

func TestModule_Events_Nil(t *testing.T) {
	m := New()
	if m.Events() != nil {
		t.Error("Events should be nil")
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	if m.Health() != core.HealthOK {
		t.Errorf("Health = %v, want HealthOK", m.Health())
	}
}

func TestModule_Stop(t *testing.T) {
	m := New()
	//lint:ignore SA1012 nil context triggers the error path
	if err := m.Stop(nil); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestModule_Start(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	//lint:ignore SA1012 nil context triggers the error path
	if err := m.Start(nil); err != nil {
		t.Errorf("Start: %v", err)
	}
}

// =============================================================================
