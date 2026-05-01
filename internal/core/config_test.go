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
	if cfg.Server.RateLimitPerMinute != 120 {
		t.Errorf("default rate limit = %d, want 120", cfg.Server.RateLimitPerMinute)
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

func TestAuditSecrets_NoWarningsWhenClean(t *testing.T) {
	cfg := validTestConfig()
	warnings := cfg.AuditSecrets()
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for clean config, got %d: %v", len(warnings), warnings)
	}
}

func TestAuditSecrets_WarnsOnPlaintextToken(t *testing.T) {
	cfg := validTestConfig()
	cfg.DNS.CloudflareToken = "cf-secret-token-value"
	cfg.Billing.StripeSecretKey = "sk_live_abc123"

	warnings := cfg.AuditSecrets()
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}

	found := map[string]bool{}
	for _, w := range warnings {
		if strings.Contains(w, "dns.cloudflare_token") {
			found["cloudflare"] = true
		}
		if strings.Contains(w, "billing.stripe_secret_key") {
			found["stripe"] = true
		}
	}
	if !found["cloudflare"] {
		t.Error("expected warning about dns.cloudflare_token")
	}
	if !found["stripe"] {
		t.Error("expected warning about billing.stripe_secret_key")
	}
}

func TestAuditSecrets_NoWarningWhenEnvSet(t *testing.T) {
	cfg := validTestConfig()
	cfg.DNS.CloudflareToken = "cf-token"
	t.Setenv("MONSTER_CLOUDFLARE_TOKEN", "cf-token")

	warnings := cfg.AuditSecrets()
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings when env var is set, got %d: %v", len(warnings), warnings)
	}
}

// ─── Validate: new rules (Tier 28) ──────────────────────────────────────────

func TestConfigValidate_LogLevel(t *testing.T) {
	for _, lvl := range []string{"debug", "info", "warn", "error", ""} {
		cfg := validTestConfig()
		cfg.Server.LogLevel = lvl
		if err := cfg.Validate(); err != nil {
			t.Errorf("LogLevel %q should be valid, got: %v", lvl, err)
		}
	}
	cfg := validTestConfig()
	cfg.Server.LogLevel = "trace"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "log_level") {
		t.Fatalf("expected log_level error for 'trace', got %v", err)
	}
}

func TestConfigValidate_LogFormat(t *testing.T) {
	for _, fmt := range []string{"text", "json", ""} {
		cfg := validTestConfig()
		cfg.Server.LogFormat = fmt
		if err := cfg.Validate(); err != nil {
			t.Errorf("LogFormat %q should be valid, got: %v", fmt, err)
		}
	}
	cfg := validTestConfig()
	cfg.Server.LogFormat = "xml"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "log_format") {
		t.Fatalf("expected log_format error for 'xml', got %v", err)
	}
}

func TestConfigValidate_ACMEEmail(t *testing.T) {
	cfg := validTestConfig()
	cfg.ACME.Email = "bad-email"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "acme.email") {
		t.Fatalf("expected acme.email error, got %v", err)
	}

	cfg.ACME.Email = "user@example.com"
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid ACME email rejected: %v", err)
	}

	cfg.ACME.Email = "" // empty is fine (optional)
	if err := cfg.Validate(); err != nil {
		t.Errorf("empty ACME email rejected: %v", err)
	}
}

func TestConfigValidate_ACMEProvider(t *testing.T) {
	for _, prov := range []string{"http-01", "dns-01", ""} {
		cfg := validTestConfig()
		cfg.ACME.Provider = prov
		if err := cfg.Validate(); err != nil {
			t.Errorf("ACME provider %q should be valid, got: %v", prov, err)
		}
	}
	cfg := validTestConfig()
	cfg.ACME.Provider = "tls-alpn-01"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "acme.provider") {
		t.Fatalf("expected acme.provider error, got %v", err)
	}
}

func TestConfigValidate_DNSProvider(t *testing.T) {
	for _, prov := range []string{"cloudflare", "route53", "manual"} {
		cfg := validTestConfig()
		cfg.DNS.Provider = prov
		if prov == "cloudflare" {
			cfg.DNS.CloudflareToken = "tok"
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("DNS provider %q should be valid, got: %v", prov, err)
		}
	}
	cfg := validTestConfig()
	cfg.DNS.Provider = "godaddy"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "dns.provider") {
		t.Fatalf("expected dns.provider error, got %v", err)
	}
}

func TestConfigValidate_CloudflareTokenRequired(t *testing.T) {
	cfg := validTestConfig()
	cfg.DNS.Provider = "cloudflare"
	cfg.DNS.CloudflareToken = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cloudflare_token") {
		t.Fatalf("expected cloudflare_token error, got %v", err)
	}
}

func TestConfigValidate_DockerNonNegative(t *testing.T) {
	cfg := validTestConfig()
	cfg.Docker.DefaultCPUQuota = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "cpu_quota") {
		t.Fatalf("expected cpu_quota error, got %v", err)
	}

	cfg = validTestConfig()
	cfg.Docker.DefaultMemoryMB = -100
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "memory_mb") {
		t.Fatalf("expected memory_mb error, got %v", err)
	}

	// Zero is fine
	cfg = validTestConfig()
	cfg.Docker.DefaultCPUQuota = 0
	cfg.Docker.DefaultMemoryMB = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("zero Docker values rejected: %v", err)
	}
}

func TestConfigValidate_BackupRetention(t *testing.T) {
	cfg := validTestConfig()
	cfg.Backup.RetentionDays = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "retention_days") {
		t.Fatalf("expected retention_days error, got %v", err)
	}
}

func TestConfigValidate_S3Bucket(t *testing.T) {
	cfg := validTestConfig()
	cfg.Backup.S3.Bucket = "ab" // too short
	cfg.Backup.S3.Region = "us-east-1"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatalf("expected bucket length error, got %v", err)
	}

	cfg = validTestConfig()
	cfg.Backup.S3.Bucket = strings.Repeat("a", 64) // too long
	cfg.Backup.S3.Region = "us-east-1"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "bucket") {
		t.Fatalf("expected bucket length error for 64 chars, got %v", err)
	}

	cfg = validTestConfig()
	cfg.Backup.S3.Bucket = "valid-bucket"
	cfg.Backup.S3.Region = "" // region required when bucket set
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("expected region error, got %v", err)
	}

	cfg = validTestConfig()
	cfg.Backup.S3.Bucket = "valid-bucket"
	cfg.Backup.S3.Region = "us-east-1"
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid S3 config rejected: %v", err)
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
