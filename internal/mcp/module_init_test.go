package mcp

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestModuleInitClosure covers the init() registered factory closure by
// creating a minimal Core via NewApp, which calls registerAllModules →
// factory() → func() core.Module { return New() }.
func TestModuleInitClosure(t *testing.T) {
	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-key-for-init-test"
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8443

	build := core.BuildInfo{Version: "0.0.0-test", Commit: "none", Date: "today"}
	c, err := core.NewApp(cfg, build)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if c == nil {
		t.Fatal("NewApp returned nil")
	}
	// Verify the mcp module was registered via the factory closure.
	mod := c.Registry.Get("mcp")
	if mod == nil {
		t.Fatal("mcp module not registered (Get returned nil)")
	}
	if mod.ID() != "mcp" {
		t.Errorf("module ID = %q, want %q", mod.ID(), "mcp")
	}
}
