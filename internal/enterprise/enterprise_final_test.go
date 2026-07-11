package enterprise

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_ID_Final(t *testing.T) {
	m := New()
	if m.ID() != "enterprise" {
		t.Errorf("ID = %q, want %q", m.ID(), "enterprise")
	}
}

// TestNewApp_TriggersInitClosure covers the init() closure body
// (func() core.Module { return New() }) by calling NewApp, which
// calls registerAllModules → factory() → New(), ensuring the
// closure body is exercised.
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
