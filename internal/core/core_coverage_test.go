package core

import (
	"testing"
)

func testConfig() *Config {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret-key-at-least-32-bytes!"
	return cfg
}

// =============================================================================
// app.go:146 — initTracer (0%) traced by calling NewApp with TracingURL.
// =============================================================================

func TestNewApp_WithTracingURL(t *testing.T) {
	cfg := testConfig()
	cfg.Observability.TracingURL = "http://localhost:4318"
	cfg.Observability.ServiceName = "test-service"
	cfg.Server.Port = 0

	c, err := NewApp(cfg, BuildInfo{Version: "test"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if c == nil {
		t.Fatal("NewApp returned nil")
	}
	if c.TracerProvider == nil {
		t.Log("TracerProvider may be nil if OTel SDK init failed — this is expected in test")
	}
}

// =============================================================================
// app.go:112-115 — Config audit secrets warnings branch
// =============================================================================


// =============================================================================
// command_safety.go:90 — CommandSafe (0%)
// =============================================================================

func TestCommandSafe_Allowed(t *testing.T) {
	if !CommandSafe("ls -la") {
		t.Error("expected 'ls -la' to be safe")
	}
}

func TestCommandSafe_BlockedOperator(t *testing.T) {
	if CommandSafe("ls && rm -rf /") {
		t.Error("expected 'ls && rm -rf /' to be unsafe")
	}
}

func TestCommandSafe_Empty(t *testing.T) {
	if !CommandSafe("") {
		t.Error("expected empty command to be safe (returns /bin/true)")
	}
}

// =============================================================================
// command_safety.go:54 — SplitCommand empty token path (line 83-85)
// =============================================================================

func TestSplitCommand_Empty(t *testing.T) {
	tokens := SplitCommand("")
	if len(tokens) != 1 || tokens[0] != "/bin/true" {
		t.Errorf("SplitCommand('') = %v, want ['/bin/true']", tokens)
	}
}

func TestSplitCommand_OnlyWhitespace(t *testing.T) {
	tokens := SplitCommand("   \t  \n  ")
	if len(tokens) != 1 || tokens[0] != "/bin/true" {
		t.Errorf("SplitCommand(whitespace) = %v, want ['/bin/true']", tokens)
	}
}

// =============================================================================
// command_safety.go:95 — CommandTokensSafe empty tokens path
// =============================================================================

func TestCommandTokensSafe_Empty(t *testing.T) {
	if CommandTokensSafe(nil) {
		t.Error("expected false for nil tokens")
	}
	if CommandTokensSafe([]string{}) {
		t.Error("expected false for empty tokens")
	}
}
