package core

import (
	"os"
	"strings"
	"testing"
)

func TestApplyEnvOverrides_Boost(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	// Set a few env vars and verify they override defaults
	t.Setenv("MONSTER_HOST", "127.0.0.1")
	t.Setenv("MONSTER_PORT", "9090")
	t.Setenv("MONSTER_DOMAIN", "example.com")
	t.Setenv("MONSTER_SECRET", "my-secret-key-32-bytes-long!!!")
	t.Setenv("MONSTER_DB_PATH", "/tmp/test.db")
	t.Setenv("MONSTER_DOCKER_HOST", "tcp://192.168.1.1:2375")
	t.Setenv("MONSTER_DOCKER_CPU_QUOTA", "200000")
	t.Setenv("MONSTER_DOCKER_MEMORY_MB", "1024")
	t.Setenv("MONSTER_LOG_LEVEL", "debug")
	t.Setenv("MONSTER_LOG_FORMAT", "json")
	t.Setenv("MONSTER_ACME_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_REGISTRATION_MODE", "invite")
	t.Setenv("MONSTER_CORS_ORIGINS", "https://app.example.com")
	t.Setenv("MONSTER_ENABLE_PPROF", "true")
	t.Setenv("MONSTER_SMTP_HOST", "smtp.example.com")
	t.Setenv("MONSTER_SMTP_PORT", "587")
	t.Setenv("MONSTER_SMTP_USERNAME", "user")
	t.Setenv("MONSTER_SMTP_PASSWORD", "pass")
	t.Setenv("MONSTER_SMTP_FROM", "noreply@example.com")
	t.Setenv("MONSTER_SMTP_FROM_NAME", "DeployMonster")
	t.Setenv("MONSTER_SMTP_USE_TLS", "true")
	t.Setenv("MONSTER_S3_ACCESS_KEY", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("MONSTER_S3_SECRET_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")

	applyEnvOverrides(cfg)

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Domain != "example.com" {
		t.Errorf("domain = %q, want example.com", cfg.Server.Domain)
	}
	if cfg.Server.SecretKey != "my-secret-key-32-bytes-long!!!" {
		t.Errorf("secret = %q", cfg.Server.SecretKey)
	}
	if cfg.Database.Path != "/tmp/test.db" {
		t.Errorf("db path = %q", cfg.Database.Path)
	}
	if cfg.Docker.Host != "tcp://192.168.1.1:2375" {
		t.Errorf("docker host = %q", cfg.Docker.Host)
	}
	if cfg.Docker.DefaultCPUQuota != 200000 {
		t.Errorf("cpu quota = %d, want 200000", cfg.Docker.DefaultCPUQuota)
	}
	if cfg.Docker.DefaultMemoryMB != 1024 {
		t.Errorf("memory = %d, want 1024", cfg.Docker.DefaultMemoryMB)
	}
	if cfg.Server.LogLevel != "debug" {
		t.Errorf("log level = %q", cfg.Server.LogLevel)
	}
	if cfg.Server.LogFormat != "json" {
		t.Errorf("log format = %q", cfg.Server.LogFormat)
	}
	if cfg.ACME.Email != "admin@example.com" {
		t.Errorf("acme email = %q", cfg.ACME.Email)
	}
	if cfg.Registration.Mode != "invite" {
		t.Errorf("registration = %q", cfg.Registration.Mode)
	}
	if cfg.Server.CORSOrigins != "https://app.example.com" {
		t.Errorf("cors = %q", cfg.Server.CORSOrigins)
	}
	if !cfg.Server.EnablePprof {
		t.Error("expected pprof enabled")
	}
	if cfg.Notifications.SMTP.Host != "smtp.example.com" {
		t.Errorf("smtp host = %q", cfg.Notifications.SMTP.Host)
	}
	if cfg.Notifications.SMTP.Port != 587 {
		t.Errorf("smtp port = %d, want 587", cfg.Notifications.SMTP.Port)
	}
	if cfg.Notifications.SMTP.Username != "user" {
		t.Errorf("smtp user = %q", cfg.Notifications.SMTP.Username)
	}
	if cfg.Notifications.SMTP.Password != "pass" {
		t.Errorf("smtp pass = %q", cfg.Notifications.SMTP.Password)
	}
	if cfg.Notifications.SMTP.From != "noreply@example.com" {
		t.Errorf("smtp from = %q", cfg.Notifications.SMTP.From)
	}
	if cfg.Notifications.SMTP.FromName != "DeployMonster" {
		t.Errorf("smtp from_name = %q", cfg.Notifications.SMTP.FromName)
	}
	if !cfg.Notifications.SMTP.UseTLS {
		t.Error("expected smtp tls enabled")
	}
	if cfg.Backup.S3.AccessKey != "AKIAIOSFODNN7EXAMPLE" {
		t.Errorf("s3 access key = %q", cfg.Backup.S3.AccessKey)
	}
	if cfg.Backup.S3.SecretKey != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Errorf("s3 secret key = %q", cfg.Backup.S3.SecretKey)
	}
}

func TestApplyEnvOverrides_InvalidPort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	// Invalid port should be ignored
	t.Setenv("MONSTER_PORT", "not-a-number")
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 8443 {
		t.Errorf("port = %d, want 8443 (default)", cfg.Server.Port)
	}
}

func TestApplyEnvOverrides_DBURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_DB_URL", "postgres://user:pass@localhost/db")
	applyEnvOverrides(cfg)

	if cfg.Database.URL != "postgres://user:pass@localhost/db" {
		t.Errorf("db url = %q", cfg.Database.URL)
	}
	if cfg.Database.Driver != "postgres" {
		t.Errorf("db driver = %q, want postgres", cfg.Database.Driver)
	}
}

func TestApplyEnvOverrides_PreviousSecretKeys(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_PREVIOUS_SECRET_KEYS", "key1,key2,key3")
	applyEnvOverrides(cfg)

	if len(cfg.Server.PreviousSecretKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(cfg.Server.PreviousSecretKeys))
	}
	if cfg.Server.PreviousSecretKeys[0] != "key1" {
		t.Errorf("first key = %q", cfg.Server.PreviousSecretKeys[0])
	}
}

func TestApplyEnvOverrides_NoEnvVars(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	// Save original values
	host := cfg.Server.Host
	port := cfg.Server.Port

	// Clear all MONSTER_ env vars for this test
	for _, e := range os.Environ() {
		if len(e) > 8 && e[:8] == "MONSTER_" {
			key := e[:strings.IndexByte(e, '=')]
			os.Unsetenv(key)
		}
	}

	applyEnvOverrides(cfg)

	if cfg.Server.Host != host {
		t.Errorf("host changed without env var")
	}
	if cfg.Server.Port != port {
		t.Errorf("port changed without env var")
	}
}
