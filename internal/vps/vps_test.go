package vps

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Module Tests
// =============================================================================

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "vps" {
		t.Errorf("ID = %q, want %q", m.ID(), "vps")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "VPS Provider Manager" {
		t.Errorf("Name = %q", m.Name())
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("Version = %q", m.Version())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies = %v", deps)
	}
}

func TestModule_Routes_Nil(t *testing.T) {
	m := New()
	if m.Routes() != nil {
		t.Error("Routes should be nil")
	}
}

func TestModule_Events_Nil(t *testing.T) {
	m := New()
	if m.Events() != nil {
		t.Error("Events should be nil")
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	if m.Health() != core.HealthOK {
		t.Errorf("Health = %v, want HealthOK", m.Health())
	}
}

func TestModule_Stop(t *testing.T) {
	m := New()
	if err := m.Stop(nil); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestModule_Start(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	if err := m.Start(nil); err != nil {
		t.Errorf("Start: %v", err)
	}
}

// =============================================================================
// GenerateCloudInit Tests
// =============================================================================

func TestGenerateCloudInit_ContainsCloudConfig(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "my-token", "1.5.0")

	if !strings.HasPrefix(output, "#cloud-config") {
		t.Error("output should start with #cloud-config")
	}
}

func TestGenerateCloudInit_ContainsMasterURL(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "my-token", "1.5.0")

	if !strings.Contains(output, "https://master.example.com") {
		t.Error("output should contain master URL")
	}
}

func TestGenerateCloudInit_ContainsToken(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "secret-token-xyz", "1.5.0")

	if !strings.Contains(output, "secret-token-xyz") {
		t.Error("output should contain the join token")
	}
}

func TestGenerateCloudInit_ContainsVersion(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "token", "2.3.1")

	if !strings.Contains(output, "2.3.1") {
		t.Error("output should contain the version")
	}
}

func TestGenerateCloudInit_ContainsDockerInstall(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "token", "1.0.0")

	if !strings.Contains(output, "docker") {
		t.Error("output should contain Docker install instructions")
	}
}

func TestGenerateCloudInit_ContainsSystemdService(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "token", "1.0.0")

	checks := []string{
		"deploymonster-agent.service",
		"deploymonster serve --agent",
		"systemctl daemon-reload",
		"systemctl enable",
		"systemctl start",
		"MONSTER_MASTER_URL",
		"MONSTER_JOIN_TOKEN",
	}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("cloud-init output missing %q", check)
		}
	}
}

func TestGenerateCloudInit_ContainsPackages(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "token", "1.0.0")

	if !strings.Contains(output, "package_update: true") {
		t.Error("should contain package_update")
	}
	if !strings.Contains(output, "curl") {
		t.Error("should contain curl package")
	}
	if !strings.Contains(output, "ca-certificates") {
		t.Error("should contain ca-certificates package")
	}
}

func TestGenerateCloudInit_ContainsFirewall(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "token", "1.0.0")

	if !strings.Contains(output, "ufw") {
		t.Error("should contain firewall (ufw) configuration")
	}
}

func TestGenerateCloudInit_ContainsDataDir(t *testing.T) {
	output := GenerateCloudInit("https://master.example.com", "token", "1.0.0")

	if !strings.Contains(output, "/var/lib/deploymonster") {
		t.Error("should contain data directory creation")
	}
}

// =============================================================================
// BootstrapScript Tests
// =============================================================================

func TestBootstrapScript_ContainsBashHeader(t *testing.T) {
	script := BootstrapScript("https://master.example.com", "token", "srv-1")
	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("should start with #!/bin/bash")
	}
}

func TestBootstrapScript_ContainsServerID(t *testing.T) {
	script := BootstrapScript("https://master.example.com", "token", "my-server-99")
	if !strings.Contains(script, "my-server-99") {
		t.Error("should contain server ID")
	}
}

func TestBootstrapScript_ContainsMasterAndToken(t *testing.T) {
	script := BootstrapScript("https://master.test.io", "join-token-abc", "srv")
	if !strings.Contains(script, "https://master.test.io") {
		t.Error("should contain master URL")
	}
	if !strings.Contains(script, "join-token-abc") {
		t.Error("should contain join token")
	}
}

func TestBootstrapScript_ContainsDockerInstall(t *testing.T) {
	script := BootstrapScript("https://master.example.com", "token", "srv")
	if !strings.Contains(script, "https://get.docker.com") {
		t.Error("should contain Docker install URL")
	}
}

func TestBootstrapScript_ContainsSystemdUnit(t *testing.T) {
	script := BootstrapScript("https://master.example.com", "token", "srv")
	checks := []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"deploymonster-agent.service",
		"systemctl daemon-reload",
		"systemctl enable deploymonster-agent",
		"systemctl start deploymonster-agent",
	}
	for _, check := range checks {
		if !strings.Contains(script, check) {
			t.Errorf("script missing %q", check)
		}
	}
}

// =============================================================================
// CloudInitConfig Tests (wraps BootstrapScript in cloud-init YAML)
// =============================================================================

func TestCloudInitConfig_Format(t *testing.T) {
	config := CloudInitConfig("https://master.example.com", "token-123", "srv-1")

	if !strings.HasPrefix(config, "#cloud-config") {
		t.Error("should start with #cloud-config")
	}
	if !strings.Contains(config, "package_update: true") {
		t.Error("should contain package_update")
	}
	if !strings.Contains(config, "runcmd:") {
		t.Error("should contain runcmd section")
	}
}

func TestCloudInitConfig_ContainsBootstrapScript(t *testing.T) {
	config := CloudInitConfig("https://master.example.com", "tok-xyz", "server-42")

	if !strings.Contains(config, "server-42") {
		t.Error("should contain server ID from bootstrap script")
	}
	if !strings.Contains(config, "tok-xyz") {
		t.Error("should contain token from bootstrap script")
	}
}

// =============================================================================
// SSHPool Constructor Test
// =============================================================================

func TestNewSSHPool(t *testing.T) {
	pool := NewSSHPool(slog.Default())
	if pool == nil {
		t.Fatal("NewSSHPool returned nil")
	}
	if pool.clients == nil {
		t.Error("clients map should be initialized")
	}
	// Close the pool to stop the cleanup goroutine
	pool.Close()
}

func TestSSHPool_Close_Empty(t *testing.T) {
	pool := NewSSHPool(slog.Default())
	// Should not panic on empty pool
	pool.Close()
}

func TestSSHPool_Remove_NonExistent(t *testing.T) {
	pool := NewSSHPool(slog.Default())
	defer pool.Close()
	// Should not panic when removing non-existent host
	pool.remove("nonexistent:22")
}
