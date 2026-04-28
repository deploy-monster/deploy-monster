package billing

import (
	"context"
	"log/slog"
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
		{"ID", m.ID(), "billing"},
		{"Name", m.Name(), "Billing Engine"},
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

func TestModuleStop_NilMeter(t *testing.T) {
	m := New()
	// Stop with nil meter should not panic.
	err := m.Stop(context.TODO())
	if err != nil {
		t.Errorf("Stop() with nil meter returned error: %v", err)
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

func TestStripeClient_Disabled(t *testing.T) {
	m := New()
	if m.StripeClient() != nil {
		t.Error("expected StripeClient to be nil when billing disabled")
	}
}

func TestStripeClient_Enabled(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Billing: core.BillingConfig{
				Enabled:          true,
				StripeSecretKey:  "sk_test_123",
				StripeWebhookKey: "whsec_test_123",
			},
		},
		Logger: slog.Default(),
		Store:  &mockStore{},
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if m.StripeClient() == nil {
		t.Error("expected StripeClient to be non-nil when Stripe keys configured")
	}
	if m.WebhookHandler() == nil {
		t.Error("expected WebhookHandler to be non-nil when Stripe keys configured")
	}
}

func TestPlans(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{Billing: core.BillingConfig{Enabled: false}},
		Logger: slog.Default(),
		Store:  &mockStore{},
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init error: %v", err)
	}
	if len(m.Plans()) == 0 {
		t.Error("expected Plans to be non-empty")
	}
}
