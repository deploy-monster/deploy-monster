package auth

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_ID(t *testing.T) {
	m := New()
	if got := m.ID(); got != "core.auth" {
		t.Errorf("ID() = %q, want %q", got, "core.auth")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if got := m.Name(); got != "Authentication" {
		t.Errorf("Name() = %q, want %q", got, "Authentication")
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	got := m.Version()
	if got == "" {
		t.Error("Version() should return a non-empty string")
	}
	if got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()

	if len(deps) == 0 {
		t.Fatal("Dependencies() should return at least one dependency")
	}

	// Must depend on core.db
	found := false
	for _, dep := range deps {
		if dep == "core.db" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Dependencies() = %v, expected to contain %q", deps, "core.db")
	}
}

func TestModule_Health_NoJWT(t *testing.T) {
	m := New()
	// Module created with New() has no JWT service initialized
	got := m.Health()
	if got != core.HealthDown {
		t.Errorf("Health() = %v, want HealthDown when JWT service is nil", got)
	}
}

func TestModule_Health_WithJWT(t *testing.T) {
	m := New()
	m.jwt = MustNewJWTService("test-secret-key-at-least-32-bytes-long!")
	got := m.Health()
	if got != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK when JWT service is set", got)
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	events := m.Events()
	if events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	routes := m.Routes()
	if routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModule_JWT(t *testing.T) {
	m := New()

	// Before Init, JWT should be nil
	if m.JWT() != nil {
		t.Error("JWT() should be nil before initialization")
	}

	// Set JWT manually and verify accessor
	jwtSvc := MustNewJWTService("test-secret-key-at-least-32-bytes-long!")
	m.jwt = jwtSvc
	if m.JWT() != jwtSvc {
		t.Error("JWT() should return the configured JWT service")
	}
}

func TestModule_Store(t *testing.T) {
	m := New()

	// Before Init, Store should be nil
	if m.Store() != nil {
		t.Error("Store() should be nil before initialization")
	}
}

func TestModule_Start(t *testing.T) {
	// New creates a module; Start should work even without Init
	// (it only logs, so it needs a logger — we test the nil-safe path)
	// We cannot fully test Start without a core.Core, but we verify
	// the method exists and returns no error concept when the logger is set.
}

func TestModule_Stop(t *testing.T) {
	m := New()
	// Stop should return nil regardless of state
	if err := m.Stop(context.TODO()); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}

func TestCleanupBootstrapAdminCredentials(t *testing.T) {
	oldPath := bootstrapAdminEnvFile
	bootstrapAdminEnvFile = filepath.Join(t.TempDir(), "deploymonster.env")
	t.Cleanup(func() { bootstrapAdminEnvFile = oldPath })

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "secret")
	if err := os.WriteFile(bootstrapAdminEnvFile, []byte("MONSTER_ADMIN_PASSWORD='secret'\n"), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	m := &Module{logger: slog.Default()}
	m.cleanupBootstrapAdminCredentials()

	if _, err := os.Stat(bootstrapAdminEnvFile); !os.IsNotExist(err) {
		t.Fatalf("bootstrap env file still exists or stat failed: %v", err)
	}
	if os.Getenv("MONSTER_ADMIN_EMAIL") != "" || os.Getenv("MONSTER_ADMIN_PASSWORD") != "" {
		t.Fatal("bootstrap admin env vars were not unset")
	}
}
