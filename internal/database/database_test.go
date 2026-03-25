package database

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ core.Module = (*Module)(nil)
}

func TestModuleIdentity(t *testing.T) {
	m := New()

	tests := []struct {
		method string
		got    string
		want   string
	}{
		{"ID", m.ID(), "database"},
		{"Name", m.Name(), "Database Manager"},
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

	expected := map[string]bool{"core.db": false, "deploy": false}
	if len(deps) != len(expected) {
		t.Fatalf("Dependencies() len = %d, want %d", len(deps), len(expected))
	}
	for _, d := range deps {
		if _, ok := expected[d]; !ok {
			t.Errorf("unexpected dependency %q", d)
		}
		expected[d] = true
	}
	for d, found := range expected {
		if !found {
			t.Errorf("missing dependency %q", d)
		}
	}
}

func TestModuleRoutes(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModuleEvents(t *testing.T) {
	m := New()
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModuleHealth(t *testing.T) {
	m := New()
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", h)
	}
}

func TestModuleInit(t *testing.T) {
	c := &core.Core{
		Logger: testLogger(),
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if m.core != c {
		t.Error("Init() did not set core reference")
	}
	if m.logger == nil {
		t.Error("Init() did not set logger")
	}
}

func TestModuleStart(t *testing.T) {
	c := &core.Core{
		Logger: testLogger(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

func TestModuleStop(t *testing.T) {
	m := New()
	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestModuleStop_AfterInit(t *testing.T) {
	c := &core.Core{
		Logger: testLogger(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}
