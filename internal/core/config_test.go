package core

import (
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Server.Port != 8443 {
		t.Errorf("default port = %d, want 8443", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("default host = %q, want 0.0.0.0", cfg.Server.Host)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("default driver = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.Ingress.HTTPPort != 80 {
		t.Errorf("default HTTP port = %d, want 80", cfg.Ingress.HTTPPort)
	}
	if cfg.Ingress.HTTPSPort != 443 {
		t.Errorf("default HTTPS port = %d, want 443", cfg.Ingress.HTTPSPort)
	}
	if cfg.Registration.Mode != "open" {
		t.Errorf("default registration = %q, want open", cfg.Registration.Mode)
	}
	if cfg.Limits.MaxConcurrentBuilds != 5 {
		t.Errorf("default concurrent builds = %d, want 5", cfg.Limits.MaxConcurrentBuilds)
	}
	if cfg.Server.SecretKey == "" {
		t.Error("secret key should be auto-generated")
	}
}

func TestConfigValidate_Valid(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}
}

func TestConfigValidate_BadPort(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.Port = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("expected port error, got %v", err)
	}
}

func TestConfigValidate_ShortSecret(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.SecretKey = "short"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "secret_key") {
		t.Fatalf("expected secret_key error, got %v", err)
	}
}

func TestConfigValidate_BadDriver(t *testing.T) {
	cfg := validTestConfig()
	cfg.Database.Driver = "mysql"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "driver") {
		t.Fatalf("expected driver error, got %v", err)
	}
}

func TestConfigValidate_BadRegistrationMode(t *testing.T) {
	cfg := validTestConfig()
	cfg.Registration.Mode = "unknown"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "registration.mode") {
		t.Fatalf("expected mode error, got %v", err)
	}
}

func validTestConfig() *Config {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "this-is-a-valid-secret-key-32chars"
	return cfg
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Marketplace.Enabled != true {
		t.Error("marketplace should be enabled by default")
	}
	if cfg.Backup.Encryption != true {
		t.Error("backup encryption should be enabled by default")
	}
	if cfg.Backup.RetentionDays != 30 {
		t.Errorf("backup retention = %d, want 30", cfg.Backup.RetentionDays)
	}
}
