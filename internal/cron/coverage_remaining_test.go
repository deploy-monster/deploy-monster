package cron

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestInitClosure covers the init() function's factory closure (line 19).
// init() registers a factory via core.RegisterModule; calling core.NewApp
// triggers registerAllModules which invokes every registered factory,
// covering the return-New() path inside the closure.
func TestInitClosure(t *testing.T) {
	cfg := &core.Config{
		Server: core.ServerConfig{
			SecretKey: "test-secret-key-for-cron-init-coverage",
		},
	}
	_, err := core.NewApp(cfg, core.BuildInfo{Version: "0.0.0"})
	if err != nil {
		t.Logf("NewApp returned (expected in unit test): %v", err)
	}
}
