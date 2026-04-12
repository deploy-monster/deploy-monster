package gitsources

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Health — covers module.go:52 (33.3% → 100%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleHealth_Degraded_HasConfigNoProviders(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:    &stubStore{},
		Services: core.NewServices(), // no git providers registered
		Logger:   discardLogger(),
		Events:   core.NewEventBus(discardLogger()),
		Config: &core.Config{
			GitSources: core.GitSourcesConfig{
				GitHubClientID: "some-id", // config present
			},
		},
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Clear the providers that Init registered to simulate misconfiguration
	// We need a core with no providers but with config
	m2 := &Module{
		core: &core.Core{
			Config: &core.Config{
				GitSources: core.GitSourcesConfig{
					GitHubClientID: "client-id",
				},
			},
			Services: core.NewServices(), // empty — no providers registered
		},
	}

	if got := m2.Health(); got != core.HealthDegraded {
		t.Errorf("Health() with config but no providers = %v, want HealthDegraded", got)
	}
}

func TestModuleHealth_OK_HasConfigAndProviders(t *testing.T) {
	c := &core.Core{
		Store:    &stubStore{},
		Services: core.NewServices(),
		Logger:   discardLogger(),
		Events:   core.NewEventBus(discardLogger()),
		Config: &core.Config{
			GitSources: core.GitSourcesConfig{
				GitHubClientID: "some-id",
			},
		},
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// After Init, providers are registered, so Health should be OK
	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() with config and providers = %v, want HealthOK", got)
	}
}

func TestModuleHealth_OK_NoConfig(t *testing.T) {
	m := &Module{
		core: &core.Core{
			Config: &core.Config{
				GitSources: core.GitSourcesConfig{}, // no config
			},
			Services: core.NewServices(),
		},
	}

	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() with no config = %v, want HealthOK", got)
	}
}

func TestModuleHealth_OK_GitLabConfigWithProviders(t *testing.T) {
	c := &core.Core{
		Store:    &stubStore{},
		Services: core.NewServices(),
		Logger:   discardLogger(),
		Events:   core.NewEventBus(discardLogger()),
		Config: &core.Config{
			GitSources: core.GitSourcesConfig{
				GitLabClientID: "gitlab-id",
			},
		},
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", got)
	}
}
