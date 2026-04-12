package gitsources

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// init() — covers module.go:11 (RegisterModule factory call)
// The init() runs on package import and registers a module factory.
// We verify the produced module satisfies the interface.
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m

	if m.ID() != "gitsources" {
		t.Errorf("ID() = %q, want gitsources", m.ID())
	}
}

// TestModuleInit_RegistersAllProviders verifies that Init populates all four
// git providers in the Services registry.
func TestModuleInit_RegistersAllProviders(t *testing.T) {
	m := New()
	c := newTestCore(t)

	if err := m.Init(t.Context(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	expected := []string{"github", "gitlab", "gitea", "bitbucket"}
	for _, name := range expected {
		if p := c.Services.GitProvider(name); p == nil {
			t.Errorf("missing registered provider %q", name)
		}
	}
}
