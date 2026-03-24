package core

import "testing"

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig()
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
