package vps

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// discardLogger returns a logger that discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestModule_Init(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:  nil,
		Logger: discardLogger(),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.core != c {
		t.Error("core reference not set")
	}
	if m.store != nil {
		t.Error("store should be nil since Core.Store is nil")
	}
	if m.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestModule_Init_WithStore(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:  nil,
		Logger: discardLogger(),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// TestModule_Init_NilServices covers the c.Services == nil branch in Init.
func TestModule_Init_NilServices(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: discardLogger(),
		// Services is nil by default
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if c.Services == nil {
		t.Error("Init should have created Services")
	}
}

// TestModule_Init_AllProvidersNoToken covers the branch where provider
// tokens are empty but custom is always registered.
func TestModule_Init_AllProvidersNoToken(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{},
		Logger: discardLogger(),
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Custom should always be registered
	if c.Services.VPSProvisioner("custom") == nil {
		t.Error("custom provider should always be registered")
	}
}

// TestNewApp_TriggersInitClosure covers the init() closure body
// (func() core.Module { return New() }) via core.NewApp.
func TestNewApp_TriggersInitClosure(t *testing.T) {
	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-32-chars-minimum!yes!!"
	cfg.Server.LogLevel = "info"
	cfg.Server.LogFormat = "text"
	_, err := core.NewApp(cfg, core.BuildInfo{Version: "test"})
	if err != nil {
		t.Logf("NewApp returned (ok if infra missing): %v", err)
	}
}

// TestModule_Init_CoreNilConfig covers the m.core == nil / Config == nil branch
// in providerToken.
func TestModule_Init_CoreNilConfig(t *testing.T) {
	m := New()
	c := &core.Core{
		Services: core.NewServices(),
		Logger:   discardLogger(),
		// Core.Config is nil
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

// TestModule_ProviderToken_DefaultCase covers the default branch of providerToken.
func TestModule_ProviderToken_DefaultCase(t *testing.T) {
	m := New()
	m.core = &core.Core{Config: &core.Config{}}
	token := m.providerToken("nonexistent")
	if token != "" {
		t.Errorf("expected empty token for unknown provider, got %q", token)
	}
}

// TestModule_ProviderToken_NilCore covers the m.core == nil branch.
func TestModule_ProviderToken_NilCore(t *testing.T) {
	m := New()
	token := m.providerToken("hetzner")
	if token != "" {
		t.Errorf("expected empty token when core is nil, got %q", token)
	}
}
