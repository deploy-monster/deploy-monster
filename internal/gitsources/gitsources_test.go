package gitsources

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// stubStore implements core.Store with no-op methods — only what gitsources needs.
type stubStore struct{ core.Store }

func newTestCore(t *testing.T) *core.Core {
	t.Helper()
	c := &core.Core{
		Store:    &stubStore{},
		Services: core.NewServices(),
		Logger:   discardLogger(),
		Events:   core.NewEventBus(discardLogger()),
	}
	return c
}

// ---------------------------------------------------------------------------
// Module identity
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModuleID(t *testing.T) {
	m := New()
	if got := m.ID(); got != "gitsources" {
		t.Errorf("ID() = %q, want %q", got, "gitsources")
	}
}

func TestModuleName(t *testing.T) {
	m := New()
	if got := m.Name(); got != "Git Source Manager" {
		t.Errorf("Name() = %q, want %q", got, "Git Source Manager")
	}
}

func TestModuleVersion(t *testing.T) {
	m := New()
	if got := m.Version(); got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}
}

func TestModuleDependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies() = %v, want [core.db]", deps)
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
	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() = %v, want %v", got, core.HealthOK)
	}
}

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

func TestModuleInit(t *testing.T) {
	m := New()
	c := newTestCore(t)

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Verify that core was stored
	if m.core != c {
		t.Error("Init() did not set core reference")
	}
	if m.store == nil {
		t.Error("Init() did not set store reference")
	}
	if m.logger == nil {
		t.Error("Init() did not set logger")
	}
}

func TestModuleInitRegistersProviders(t *testing.T) {
	m := New()
	c := newTestCore(t)

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Verify all 4 providers were registered in Services
	providerNames := c.Services.GitProviders()
	if len(providerNames) < 4 {
		t.Errorf("expected at least 4 registered git providers, got %d: %v", len(providerNames), providerNames)
	}

	for _, name := range []string{"github", "gitlab", "gitea", "bitbucket"} {
		if p := c.Services.GitProvider(name); p == nil {
			t.Errorf("expected git provider %q to be registered", name)
		}
	}
}

func TestModuleStart(t *testing.T) {
	m := New()
	c := newTestCore(t)

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Errorf("Start() error = %v", err)
	}
}

func TestModuleStop(t *testing.T) {
	m := New()
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Full lifecycle
// ---------------------------------------------------------------------------

func TestModuleFullLifecycle(t *testing.T) {
	m := New()
	c := newTestCore(t)
	ctx := context.Background()

	// Init
	if err := m.Init(ctx, c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Health before start
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() before Start = %v, want %v", h, core.HealthOK)
	}

	// Start
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Health after start
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() after Start = %v, want %v", h, core.HealthOK)
	}

	// Stop
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// Module satisfies interface
// ---------------------------------------------------------------------------

func TestModuleImplementsCoreModule(t *testing.T) {
	var _ core.Module = (*Module)(nil)
}
