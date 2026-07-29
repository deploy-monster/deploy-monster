package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// === merged from config_boost_test.go ===

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

// === merged from config_compat_test.go ===

// TestConfig_LegacyMinimalYAML verifies that a config file from an earlier
// release — containing only the bare minimum fields a v1.0 user would have
// written — still loads cleanly into the current Config struct. This is the
// upgrade contract: old monster.yaml files must continue to work after a
// version bump.
func TestConfig_LegacyMinimalYAML(t *testing.T) {
	// This is what a very old monster.yaml looked like before we added
	// log_level, log_format, rate_limit_per_minute, cors_origins, docker
	// resource defaults, backup.s3, and the various new provider configs.
	legacy := `
server:
  host: 0.0.0.0
  port: 8443
  secret_key: legacy-secret-key-minimum-32-chars
database:
  driver: sqlite
  path: legacy.db
ingress:
  http_port: 80
  https_port: 443
  enable_https: true
registration:
  mode: open
limits:
  max_apps_per_tenant: 50
  max_concurrent_builds: 3
`
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.yaml")
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	// Isolate from ambient MONSTER_* env vars in the test runner.
	clearMonsterEnv(t)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig(legacy): %v", err)
	}

	// Explicit legacy values preserved
	if cfg.Server.Port != 8443 {
		t.Errorf("server.port: got %d, want 8443", cfg.Server.Port)
	}
	if cfg.Server.SecretKey != "legacy-secret-key-minimum-32-chars" {
		t.Errorf("server.secret_key not preserved")
	}
	if cfg.Database.Path != "legacy.db" {
		t.Errorf("database.path: got %q, want legacy.db", cfg.Database.Path)
	}
	if cfg.Limits.MaxAppsPerTenant != 50 {
		t.Errorf("limits.max_apps_per_tenant: got %d, want 50", cfg.Limits.MaxAppsPerTenant)
	}

	// New fields filled in with defaults by applyDefaults — upgrade contract:
	// a pre-v1.x YAML must not crash the loader or produce a Validate error
	// just because it omitted fields we added in newer releases.
	if cfg.Docker.DefaultCPUQuota == 0 {
		t.Error("docker.default_cpu_quota: expected default, got 0")
	}
	if cfg.Docker.DefaultMemoryMB == 0 {
		t.Error("docker.default_memory_mb: expected default, got 0")
	}
	if cfg.Docker.Host == "" {
		t.Error("docker.host: expected default, got empty")
	}
	if cfg.Backup.RetentionDays == 0 {
		t.Error("backup.retention_days: expected default, got 0")
	}
	if cfg.Backup.StoragePath == "" {
		t.Error("backup.storage_path: expected default, got empty")
	}
	if !cfg.Marketplace.Enabled {
		t.Error("marketplace.enabled: expected default true")
	}
}

// TestConfig_RoundTripMarshalUnmarshal catches fields that were added to the
// Config struct but forgot a yaml tag — or yaml tags that disagree with the
// struct field name such that marshal → unmarshal drops the value silently.
// This is the defensive check we run every release to prevent silently
// losing user configuration across a version bump.
func TestConfig_RoundTripMarshalUnmarshal(t *testing.T) {
	orig := &Config{
		Server: ServerConfig{
			Host:               "127.0.0.1",
			Port:               9443,
			Domain:             "deploy.example.com",
			SecretKey:          "round-trip-secret-key-0123456789",
			PreviousSecretKeys: []string{"old-key-1", "old-key-2"},
			CORSOrigins:        "https://app.example.com",
			EnablePprof:        true,
			LogLevel:           "debug",
			LogFormat:          "json",
			RateLimitPerMinute: 240,
			AllowedCIDRs:       []string{"10.0.0.0/8", "192.168.1.0/24"},
		},
		Database: DatabaseConfig{
			Driver:          "postgres",
			Path:            "unused-for-pg.db",
			URL:             "postgres://user:pw@localhost/db",
			QueryTimeoutSec: 10,
		},
		Ingress: IngressConfig{
			HTTPPort:    8080,
			HTTPSPort:   8443,
			EnableHTTPS: true,
		},
		ACME: ACMEConfig{
			Email:    "admin@example.com",
			Staging:  true,
			CertDir:  "/var/certs",
			Provider: "dns-01",
		},
		DNS: DNSConfig{
			Provider:        "cloudflare",
			CloudflareToken: "cf-token-value",
			AutoSubdomain:   "deploy.example.com",
		},
		Docker: DockerConfig{
			Host:            "tcp://docker:2376",
			APIVersion:      "1.43",
			TLSVerify:       true,
			DefaultCPUQuota: 200000,
			DefaultMemoryMB: 1024,
		},
		Backup: BackupConfig{
			Schedule:      "0 2 * * *",
			RetentionDays: 14,
			StoragePath:   "/backups",
			Encryption:    true,
			S3: BackupS3Config{
				Bucket:    "deploymonster-backups",
				Region:    "us-east-1",
				Endpoint:  "https://s3.example.com",
				AccessKey: "AKIA...",
				SecretKey: "secret-access-value",
				PathStyle: true,
			},
		},
		Notifications: NotificationConfig{
			SlackWebhook:   "https://hooks.slack.com/services/T/B/X",
			DiscordWebhook: "https://discord.com/api/webhooks/id/tok",
			TelegramToken:  "bot:token",
			TelegramChatID: "-100123",
		},
		Swarm: SwarmConfig{
			Enabled:   true,
			ManagerIP: "10.0.0.1",
			JoinToken: "SWMTKN-1-xxx",
		},
		VPSProviders: VPSProvidersConfig{Enabled: true},
		GitSources: GitSourcesConfig{
			GitHubClientID:     "gh-client",
			GitHubClientSecret: "gh-secret",
			GitLabClientID:     "gl-client",
			GitLabClientSecret: "gl-secret",
		},
		Marketplace: MarketplaceConfig{
			Enabled:       true,
			TemplatesDir:  "marketplace/templates",
			CommunitySync: true,
		},
		Registration: RegistrationConfig{Mode: "invite_only"},
		Secrets:      SecretsConfig{EncryptionKey: "encryption-key-value-0123456789ab"},
		Billing: BillingConfig{
			Enabled:          true,
			StripeSecretKey:  "sk_test_xxx",
			StripeWebhookKey: "whsec_xxx",
		},
		Limits: LimitsConfig{
			MaxAppsPerTenant:    200,
			MaxBuildMinutes:     45,
			MaxConcurrentBuilds: 10,
		},
		Enterprise: EnterpriseConfig{
			Enabled:    true,
			LicenseKey: "lic-key-abc",
		},
	}

	data, err := yaml.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var round Config
	if err := yaml.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(orig, &round) {
		t.Errorf("round-trip mismatch")
		t.Logf("yaml:\n%s", data)
		// Compare top-level sections to narrow down the offender.
		compareSection(t, "Server", orig.Server, round.Server)
		compareSection(t, "Database", orig.Database, round.Database)
		compareSection(t, "Ingress", orig.Ingress, round.Ingress)
		compareSection(t, "ACME", orig.ACME, round.ACME)
		compareSection(t, "DNS", orig.DNS, round.DNS)
		compareSection(t, "Docker", orig.Docker, round.Docker)
		compareSection(t, "Backup", orig.Backup, round.Backup)
		compareSection(t, "Notifications", orig.Notifications, round.Notifications)
		compareSection(t, "Swarm", orig.Swarm, round.Swarm)
		compareSection(t, "VPSProviders", orig.VPSProviders, round.VPSProviders)
		compareSection(t, "GitSources", orig.GitSources, round.GitSources)
		compareSection(t, "Marketplace", orig.Marketplace, round.Marketplace)
		compareSection(t, "Registration", orig.Registration, round.Registration)
		compareSection(t, "Secrets", orig.Secrets, round.Secrets)
		compareSection(t, "Billing", orig.Billing, round.Billing)
		compareSection(t, "Limits", orig.Limits, round.Limits)
		compareSection(t, "Enterprise", orig.Enterprise, round.Enterprise)
	}
}

func compareSection(t *testing.T, name string, a, b any) {
	t.Helper()
	if !reflect.DeepEqual(a, b) {
		t.Errorf("section %s differs:\n  before: %+v\n  after:  %+v", name, a, b)
	}
}

// TestConfig_YAMLTagsPresentForAllFields verifies that every field of every
// config sub-struct has an explicit yaml tag. A missing tag means yaml would
// use the Go field name lowercased — which is invisible churn and breaks
// backward compatibility if we rename a field. Catch it at test time.
func TestConfig_YAMLTagsPresentForAllFields(t *testing.T) {
	cfgT := reflect.TypeOf(Config{})
	for i := 0; i < cfgT.NumField(); i++ {
		section := cfgT.Field(i)
		if section.Tag.Get("yaml") == "" {
			t.Errorf("Config.%s missing yaml tag", section.Name)
		}
		// Recurse into struct-typed sections (all our sub-configs are structs).
		if section.Type.Kind() != reflect.Struct {
			continue
		}
		for j := 0; j < section.Type.NumField(); j++ {
			f := section.Type.Field(j)
			if f.Tag.Get("yaml") == "" {
				t.Errorf("%s.%s missing yaml tag", section.Name, f.Name)
			}
		}
	}
}

// TestConfig_EnvVarPrecedence verifies that env vars override YAML values
// (but YAML still wins against defaults). This is the tri-level priority
// contract: env > yaml > defaults.
func TestConfig_EnvVarPrecedence(t *testing.T) {
	yamlContent := `
server:
  host: from-yaml
  port: 7443
  secret_key: yaml-secret-key-minimum-32-chars
database:
  driver: sqlite
  path: yaml.db
ingress:
  http_port: 80
  https_port: 443
  enable_https: true
registration:
  mode: open
limits:
  max_apps_per_tenant: 10
  max_concurrent_builds: 2
`
	dir := t.TempDir()
	path := filepath.Join(dir, "pri.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	clearMonsterEnv(t)
	t.Setenv("MONSTER_HOST", "from-env")
	t.Setenv("MONSTER_PORT", "9999")
	t.Setenv("MONSTER_REGISTRATION_MODE", "invite_only")
	t.Setenv("MONSTER_RATE_LIMIT_PER_MINUTE", "0")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Env beats YAML
	if cfg.Server.Host != "from-env" {
		t.Errorf("server.host: env should override yaml, got %q", cfg.Server.Host)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("server.port: env should override yaml, got %d", cfg.Server.Port)
	}
	if cfg.Registration.Mode != "invite_only" {
		t.Errorf("registration.mode: env should override yaml, got %q", cfg.Registration.Mode)
	}
	if cfg.Server.RateLimitPerMinute != 0 {
		t.Errorf("server.rate_limit_per_minute: env should override default, got %d", cfg.Server.RateLimitPerMinute)
	}

	// YAML beats defaults (no env override for secret_key)
	if cfg.Server.SecretKey != "yaml-secret-key-minimum-32-chars" {
		t.Errorf("server.secret_key: yaml should override default, got %q", cfg.Server.SecretKey)
	}
	if cfg.Database.Path != "yaml.db" {
		t.Errorf("database.path: yaml should override default, got %q", cfg.Database.Path)
	}
}

// TestConfig_UnknownFieldsDoNotBreakLoad verifies that a YAML containing
// fields the current code has *removed* still loads without error. This is
// the downgrade-tolerance contract: users who hand-edit their monster.yaml
// and accidentally include a stale field should not get a crash.
func TestConfig_UnknownFieldsDoNotBreakLoad(t *testing.T) {
	withUnknown := `
server:
  host: 0.0.0.0
  port: 8443
  secret_key: unknown-fields-secret-0123456789
  # A field we pretend was removed in a later release:
  legacy_allow_http: true
database:
  driver: sqlite
  path: test.db
  # Another pretend-removed field:
  unused_mode: replica
ingress:
  http_port: 80
  https_port: 443
  enable_https: true
registration:
  mode: open
limits:
  max_apps_per_tenant: 10
  max_concurrent_builds: 2
`
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.yaml")
	if err := os.WriteFile(path, []byte(withUnknown), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	clearMonsterEnv(t)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig with unknown fields should not fail: %v", err)
	}
	if cfg.Server.Port != 8443 {
		t.Errorf("server.port: got %d, want 8443", cfg.Server.Port)
	}
}

// clearMonsterEnv removes every MONSTER_* env var for the duration of the
// test, so that ambient env does not leak into test expectations.
// t.Setenv("", "") cannot unset — but it can temporarily replace with empty,
// which is what our override logic checks (`if v := ... ; v != ""`).
func clearMonsterEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "MONSTER_") {
			eq := strings.IndexByte(kv, '=')
			if eq < 0 {
				continue
			}
			t.Setenv(kv[:eq], "")
		}
	}
}

// === merged from config_extra_test.go ===

func TestApplyEnvOverrides(t *testing.T) {
	tests := []struct {
		name   string
		envKey string
		envVal string
		check  func(t *testing.T, cfg *Config)
	}{
		{
			name:   "MONSTER_HOST overrides server host",
			envKey: "MONSTER_HOST",
			envVal: "127.0.0.1",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Host != "127.0.0.1" {
					t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
				}
			},
		},
		{
			name:   "MONSTER_PORT overrides server port",
			envKey: "MONSTER_PORT",
			envVal: "9090",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 9090 {
					t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 9090)
				}
			},
		},
		{
			name:   "MONSTER_PORT invalid value keeps default",
			envKey: "MONSTER_PORT",
			envVal: "not-a-number",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Port != 8443 {
					t.Errorf("Server.Port = %d, want default 8443 for invalid value", cfg.Server.Port)
				}
			},
		},
		{
			name:   "MONSTER_DOMAIN overrides server domain",
			envKey: "MONSTER_DOMAIN",
			envVal: "example.com",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.Domain != "example.com" {
					t.Errorf("Server.Domain = %q, want %q", cfg.Server.Domain, "example.com")
				}
			},
		},
		{
			name:   "MONSTER_SECRET overrides secret key",
			envKey: "MONSTER_SECRET",
			envVal: "my-secret-key",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Server.SecretKey != "my-secret-key" {
					t.Errorf("Server.SecretKey = %q, want %q", cfg.Server.SecretKey, "my-secret-key")
				}
			},
		},
		{
			name:   "MONSTER_DB_PATH overrides database path",
			envKey: "MONSTER_DB_PATH",
			envVal: "/tmp/test.db",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Database.Path != "/tmp/test.db" {
					t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, "/tmp/test.db")
				}
			},
		},
		{
			name:   "MONSTER_DB_URL overrides database URL and switches driver to postgres",
			envKey: "MONSTER_DB_URL",
			envVal: "postgres://localhost:5432/dm",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Database.URL != "postgres://localhost:5432/dm" {
					t.Errorf("Database.URL = %q, want %q", cfg.Database.URL, "postgres://localhost:5432/dm")
				}
				if cfg.Database.Driver != "postgres" {
					t.Errorf("Database.Driver = %q, want %q when DB_URL is set", cfg.Database.Driver, "postgres")
				}
			},
		},
		{
			name:   "MONSTER_DOCKER_HOST overrides docker host",
			envKey: "MONSTER_DOCKER_HOST",
			envVal: "tcp://192.168.1.100:2376",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Docker.Host != "tcp://192.168.1.100:2376" {
					t.Errorf("Docker.Host = %q, want %q", cfg.Docker.Host, "tcp://192.168.1.100:2376")
				}
			},
		},
		{
			name:   "MONSTER_ACME_EMAIL overrides ACME email",
			envKey: "MONSTER_ACME_EMAIL",
			envVal: "admin@example.com",
			check: func(t *testing.T, cfg *Config) {
				if cfg.ACME.Email != "admin@example.com" {
					t.Errorf("ACME.Email = %q, want %q", cfg.ACME.Email, "admin@example.com")
				}
			},
		},
		{
			name:   "MONSTER_REGISTRATION_MODE overrides registration mode",
			envKey: "MONSTER_REGISTRATION_MODE",
			envVal: "invite_only",
			check: func(t *testing.T, cfg *Config) {
				if cfg.Registration.Mode != "invite_only" {
					t.Errorf("Registration.Mode = %q, want %q", cfg.Registration.Mode, "invite_only")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			applyDefaults(cfg)

			t.Setenv(tt.envKey, tt.envVal)
			applyEnvOverrides(cfg)

			tt.check(t, cfg)
		})
	}
}

func TestApplyEnvOverrides_MultipleEnvVars(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_HOST", "10.0.0.1")
	t.Setenv("MONSTER_PORT", "3000")
	t.Setenv("MONSTER_DOMAIN", "deploy.monster")

	applyEnvOverrides(cfg)

	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("Server.Host = %q, want %q", cfg.Server.Host, "10.0.0.1")
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want %d", cfg.Server.Port, 3000)
	}
	if cfg.Server.Domain != "deploy.monster" {
		t.Errorf("Server.Domain = %q, want %q", cfg.Server.Domain, "deploy.monster")
	}
}

func TestApplyDefaults_AllFields(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Server.Host = %q, got %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8443 {
		t.Errorf("Server.Port = %d, got %d", cfg.Server.Port, 8443)
	}
	if cfg.Server.Domain != "" {
		t.Errorf("Server.Domain = %q, got %q", cfg.Server.Domain, "")
	}
	if cfg.Database.Path != "deploymonster.db" {
		t.Errorf("Database.Path = %q, got %q", cfg.Database.Path, "deploymonster.db")
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Database.Driver = %q, got %q", cfg.Database.Driver, "sqlite")
	}
	if cfg.Database.URL != "" {
		t.Errorf("Database.URL = %q, got %q", cfg.Database.URL, "")
	}
	if len(cfg.Server.SecretKey) < 32 {
		t.Errorf("Server.SecretKey should be auto-generated (>= 32 chars), got %d chars", len(cfg.Server.SecretKey))
	}
	if cfg.Docker.Host != "unix:///var/run/docker.sock" {
		t.Errorf("Docker.Host = %q, got %q", cfg.Docker.Host, "unix:///var/run/docker.sock")
	}
}

// === merged from core_cov_100_test.go ===

// =============================================================================
// Registry.StopAll — error from module.Stop
// =============================================================================

type covStopErrorMod struct {
	IDValue    string
	NameValue  string
	VersionVal string
	StopError  error
}

func (m *covStopErrorMod) Init(_ context.Context, _ *Core) error { return nil }
func (m *covStopErrorMod) Start(_ context.Context) error         { return nil }
func (m *covStopErrorMod) Stop(_ context.Context) error          { return m.StopError }
func (m *covStopErrorMod) Health() HealthStatus                  { return HealthOK }
func (m *covStopErrorMod) Routes() []Route                       { return nil }
func (m *covStopErrorMod) Events() []EventHandler                { return nil }
func (m *covStopErrorMod) ID() string                            { return m.IDValue }
func (m *covStopErrorMod) Name() string                          { return m.NameValue }
func (m *covStopErrorMod) Version() string                       { return m.VersionVal }
func (m *covStopErrorMod) Dependencies() []string                { return nil }

func TestCov_RegistryStopAllError(t *testing.T) {
	r := NewRegistry()
	r.Register(&covStopErrorMod{IDValue: "stop-err-mod", StopError: errors.New("stop err")})
	r.Register(&stubModule{id: "m2"})
	r.Resolve()
	err := r.StopAll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

// =============================================================================
// EventBus — Unsubscribe found/notfound
// =============================================================================

func TestCov_EventBusUnsubscribeFound(t *testing.T) {
	eb := NewEventBus(slog.Default())
	id := eb.Subscribe("e", func(ctx context.Context, event Event) error { return nil })
	if !eb.Unsubscribe(id) {
		t.Error("expected true")
	}
	if eb.Unsubscribe(id) {
		t.Error("expected false for second call")
	}
}

// =============================================================================
// EventBus — Publish with wildcard prefix match
// =============================================================================

func TestCov_EventBusWildcard(t *testing.T) {
	eb := NewEventBus(slog.Default())
	called := false
	eb.Subscribe("app.*", func(ctx context.Context, event Event) error {
		called = true
		return nil
	})
	eb.Publish(context.Background(), Event{Type: "app.deployed"})
	if !called {
		t.Error("wildcard handler not called")
	}
}

// =============================================================================
// EventBus — PublishAsync basic
// =============================================================================

func TestCov_EventBusPublishAsyncBasic(t *testing.T) {
	eb := NewEventBus(slog.Default())
	called := make(chan struct{}, 1)
	eb.Subscribe("e", func(ctx context.Context, event Event) error {
		called <- struct{}{}
		return nil
	})
	eb.PublishAsync(context.Background(), Event{Type: "e"})
	<-called
	eb.Drain()
}

// =============================================================================
// Role.HasPermission — invalid JSON and prefix matching
// =============================================================================

func TestCov_HasPermissionInvalidJSON(t *testing.T) {
	r := &Role{PermissionsJSON: "not-json"}
	if r.HasPermission("x") {
		t.Error("expected false")
	}
}

func TestCov_HasPermissionPrefix(t *testing.T) {
	r := &Role{PermissionsJSON: `["app.*"]`}
	if !r.HasPermission("app.read") {
		t.Error("expected true for prefix match")
	}
	if r.HasPermission("other") {
		t.Error("expected false for non-matching")
	}
}

// =============================================================================
// ValidateConfig — remaining uncovered paths
// =============================================================================

func TestCov_ValidateConfigPortConflictHTTP(t *testing.T) {
	err := ValidateConfig(&Config{
		Server:       ServerConfig{Port: 80, SecretKey: "key-with-at-least-32-char!!"},
		Ingress:      IngressConfig{HTTPPort: 80, HTTPSPort: 443},
		Database:     DatabaseConfig{Driver: "sqlite", Path: "/tmp/db"},
		Registration: RegistrationConfig{Mode: "open"},
		Limits:       LimitsConfig{MaxConcurrentBuilds: 5},
	})
	if err == nil {
		t.Error("expected error for port conflict")
	}
}

func TestCov_ValidateConfigPortConflictHTTPS(t *testing.T) {
	err := ValidateConfig(&Config{
		Server:       ServerConfig{Port: 443, SecretKey: "key-with-at-least-32-char!!"},
		Ingress:      IngressConfig{HTTPPort: 80, HTTPSPort: 443},
		Database:     DatabaseConfig{Driver: "sqlite", Path: "/tmp/db"},
		Registration: RegistrationConfig{Mode: "open"},
		Limits:       LimitsConfig{MaxConcurrentBuilds: 5},
	})
	if err == nil {
		t.Error("expected error for port conflict")
	}
}

func TestCov_ValidateConfigMaxBuildsZero(t *testing.T) {
	err := ValidateConfig(&Config{
		Server:       ServerConfig{Port: 8080, SecretKey: "key-with-at-least-32-char!!"},
		Ingress:      IngressConfig{HTTPPort: 80, HTTPSPort: 443},
		Database:     DatabaseConfig{Driver: "sqlite", Path: "/tmp/db"},
		Registration: RegistrationConfig{Mode: "open"},
		Limits:       LimitsConfig{MaxConcurrentBuilds: 0},
	})
	if err == nil {
		t.Error("expected error")
	}
}

// =============================================================================
// Scheduler — loop stop, calcNextRun fallback
// =============================================================================

func TestCov_SchedulerStopTwice(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.Start()
	s.Stop()
	s.Stop() // second stop should be safe
}

func TestCov_CalcNextRunFallback(t *testing.T) {
	s := NewScheduler(slog.Default())
	next := s.calcNextRun("invalid")
	if next.IsZero() {
		t.Error("should return non-zero time")
	}
}

// =============================================================================
// CircuitBreaker — open state
// =============================================================================

func TestCov_CircuitBreakerOpen(t *testing.T) {
	cb := NewCircuitBreaker("t", DefaultCircuitBreakerConfig())
	cb.mu.Lock()
	cb.state = CircuitOpen
	cb.lastFailure = cb.now()
	cb.mu.Unlock()
	err := cb.Execute(func() error { return nil })
	if err == nil {
		t.Error("expected error for open circuit")
	}
}

// =============================================================================
// applyEnvOverrides — all env vars
// =============================================================================

func TestCov_ApplyEnvOverridesFull(t *testing.T) {
	os.Clearenv()
	defer os.Clearenv()

	vars := map[string]string{
		"MONSTER_HOST":                    "h",
		"MONSTER_PORT":                    "9090",
		"MONSTER_DOMAIN":                  "d",
		"MONSTER_SECRET":                  "key-with-at-least-32-bytes-for-test!",
		"MONSTER_PREVIOUS_SECRET_KEYS":    "k1,k2",
		"MONSTER_DB_PATH":                 "/p",
		"MONSTER_DB_SSL_MODE":             "require",
		"MONSTER_DOCKER_HOST":             "tcp://d:2375",
		"MONSTER_BUILD_IMAGE_REGISTRY":    "reg.io/",
		"MONSTER_BUILD_IMAGE_PUSH":        "true",
		"MONSTER_BUILD_REGISTRY_USERNAME": "u",
		"MONSTER_BUILD_REGISTRY_PASSWORD": "p",
		"MONSTER_DOCKER_CPU_QUOTA":        "50000",
		"MONSTER_DOCKER_MEMORY_MB":        "512",
		"MONSTER_LOG_LEVEL":               "debug",
		"MONSTER_LOG_FORMAT":              "json",
		"MONSTER_ACME_EMAIL":              "a@b.com",
		"MONSTER_REGISTRATION_MODE":       "invite_only",
		"MONSTER_JOIN_TOKEN":              "tok",
		"MONSTER_AGENT_CERT_FILE":         "/c",
		"MONSTER_AGENT_KEY_FILE":          "/k",
		"MONSTER_AGENT_CA_FILE":           "/ca",
		"MONSTER_CORS_ORIGINS":            "https://o",
		"MONSTER_RATE_LIMIT_PER_MINUTE":   "100",
		"MONSTER_ENABLE_PPROF":            "true",
		"MONSTER_CLOUDFLARE_TOKEN":        "cf",
		"MONSTER_GITHUB_CLIENT_SECRET":    "gh",
		"MONSTER_GITLAB_CLIENT_SECRET":    "gl",
		"MONSTER_ENCRYPTION_KEY":          "ek",
		"MONSTER_STRIPE_SECRET_KEY":       "ssk",
		"MONSTER_STRIPE_WEBHOOK_KEY":      "swk",
	}
	for k, v := range vars {
		os.Setenv(k, v)
	}

	cfg := &Config{}
	applyDefaults(cfg)
	applyEnvOverrides(cfg)

	if cfg.Server.Host != "h" {
		t.Error("host")
	}
	if cfg.Server.Port != 9090 {
		t.Error("port")
	}
	if cfg.Docker.BuildImageRegistry != "reg.io" {
		t.Error("registry not trimmed")
	}
	if !cfg.Docker.BuildImagePush {
		t.Error("push not set")
	}
	if cfg.Server.RateLimitPerMinute != 100 {
		t.Error("rate limit")
	}
}

func TestCov_ApplyEnvOverridesInvalidPort(t *testing.T) {
	os.Clearenv()
	defer os.Clearenv()
	os.Setenv("MONSTER_PORT", "bad")
	cfg := &Config{}
	applyDefaults(cfg)
	applyEnvOverrides(cfg)
	// Should not panic; port retains default
}

// =============================================================================
// AuditSecrets — plaintext warnings
// =============================================================================

func TestCov_AuditSecretsPlaintext(t *testing.T) {
	cfg := &Config{}
	cfg.DNS.CloudflareToken = "plaintext"
	cfg.Secrets.EncryptionKey = "plaintext"
	w := cfg.AuditSecrets()
	if len(w) == 0 {
		t.Error("expected warnings")
	}
}

// =============================================================================
// ValidateVolumePaths — remaining error paths
// =============================================================================

func TestCov_ValidateVolumePathsAll(t *testing.T) {
	check := func(name string, opts ContainerOpts, wantErr bool) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			err := opts.ValidateVolumePaths()
			if (err != nil) != wantErr {
				t.Errorf("error = %v, wantErr %v", err, wantErr)
			}
		})
	}

	check("null byte", ContainerOpts{Volumes: map[string]string{"/x\x00:/y": "/c"}}, true)
	check("traversal", ContainerOpts{Volumes: map[string]string{"/data/../etc": "/c"}}, true)
	check("root blocked", ContainerOpts{Volumes: map[string]string{"/": "/c"}}, true)
	check("docker socket blocked", ContainerOpts{Volumes: map[string]string{"/var/run/docker.sock": "/c"}}, true)
	check("docker socket allowed", ContainerOpts{AllowDockerSocket: true, Volumes: map[string]string{"/var/run/docker.sock": "/c"}}, false)
	check("valid path", ContainerOpts{Volumes: map[string]string{"/data": "/c"}}, false)
}

// =============================================================================
// ContainerOpts — ApplyResourceDefaults
// =============================================================================

func TestCov_ApplyResourceDefaultsNoOverwrite(t *testing.T) {
	opts := ContainerOpts{CPUQuota: 100000, MemoryMB: 512}
	opts.ApplyResourceDefaults(50000, 256)
	if opts.CPUQuota != 100000 {
		t.Error("should not overwrite existing CPU quota")
	}
	if opts.MemoryMB != 512 {
		t.Error("should not overwrite existing memory")
	}
}

func TestCov_ApplyResourceDefaultsSetsDefaults(t *testing.T) {
	opts := ContainerOpts{}
	opts.ApplyResourceDefaults(50000, 256)
	if opts.CPUQuota != 50000 {
		t.Error("should set default CPU quota")
	}
	if opts.MemoryMB != 256 {
		t.Error("should set default memory")
	}
}

// === merged from core_final_test.go ===

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// failingModule is a module that can fail on Init or Start for testing.
type failingModule struct {
	id     string
	failOn string // "init" or "start"
}

func (f *failingModule) ID() string             { return f.id }
func (f *failingModule) Name() string           { return f.id }
func (f *failingModule) Version() string        { return "1.0.0" }
func (f *failingModule) Dependencies() []string { return nil }
func (f *failingModule) Health() HealthStatus   { return HealthOK }
func (f *failingModule) Routes() []Route        { return nil }
func (f *failingModule) Events() []EventHandler { return nil }

func (f *failingModule) Init(_ context.Context, _ *Core) error {
	if f.failOn == "init" {
		return fmt.Errorf("init failed for %s", f.id)
	}
	return nil
}
func (f *failingModule) Start(_ context.Context) error {
	if f.failOn == "start" {
		return fmt.Errorf("start failed for %s", f.id)
	}
	return nil
}
func (f *failingModule) Stop(_ context.Context) error { return nil }

// ═══════════════════════════════════════════════════════════════════════════════
// NewApp — covers app.go:47
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewApp_ReturnsCore(t *testing.T) {
	// Save and restore global module factories
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	build := BuildInfo{Version: "1.0.0", Commit: "abc", Date: "2024-01-01"}
	c, err := NewApp(cfg, build)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if c == nil {
		t.Fatal("NewApp returned nil")
	}
	if c.Config != cfg {
		t.Error("Config not set")
	}
	if c.Build.Version != "1.0.0" {
		t.Errorf("Build.Version = %q, want 1.0.0", c.Build.Version)
	}
	if c.Registry == nil {
		t.Error("Registry should not be nil")
	}
	if c.Events == nil {
		t.Error("Events should not be nil")
	}
	if c.Scheduler == nil {
		t.Error("Scheduler should not be nil")
	}
	if c.Services == nil {
		t.Error("Services should not be nil")
	}
	if c.Logger == nil {
		t.Error("Logger should not be nil")
	}
	if c.Router == nil {
		t.Error("Router should not be nil")
	}
}

func TestNewApp_WithModuleFactories(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()

	moduleFactories = moduleRegistry{}
	RegisterModule(func() Module { return newStub("factory-mod-1") })
	RegisterModule(func() Module { return newStub("factory-mod-2") })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	c, err := NewApp(cfg, BuildInfo{Version: "0.1.0"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	// Modules should be registered in the registry
	if c.Registry.Get("factory-mod-1") == nil {
		t.Error("factory-mod-1 should be registered")
	}
	if c.Registry.Get("factory-mod-2") == nil {
		t.Error("factory-mod-2 should be registered")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// registerAllModules — covers app.go:117 including error logging path
// ═══════════════════════════════════════════════════════════════════════════════

func TestRegisterAllModules_DuplicateModule(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()

	moduleFactories = moduleRegistry{}
	// Register two factories that produce modules with the same ID
	RegisterModule(func() Module { return newStub("dup") })
	RegisterModule(func() Module { return newStub("dup") })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	// NewApp calls registerAllModules; the second registration of "dup"
	// should trigger the error log branch but not crash
	c, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	// Only one module with ID "dup" should be registered
	if c.Registry.Get("dup") == nil {
		t.Error("first 'dup' module should be registered")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Run — covers app.go:67 (all branches)
// ═══════════════════════════════════════════════════════════════════════════════

func TestRun_HappyPath(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	RegisterModule(func() Module { return newStub("mod-a") })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	c, err := NewApp(cfg, BuildInfo{Version: "1.0.0", Commit: "abc"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a brief delay to unblock <-ctx.Done() in Run
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err = c.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRun_ResolveError(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	// Create a module with a dependency that doesn't exist
	RegisterModule(func() Module { return newStub("orphan", "nonexistent") })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	c, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = c.Run(ctx)
	if err == nil {
		t.Fatal("Run should fail when dependency resolution fails")
	}
}

func TestRun_InitError(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	RegisterModule(func() Module { return &failingModule{id: "fail-init", failOn: "init"} })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	c, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = c.Run(ctx)
	if err == nil {
		t.Fatal("Run should fail when module init fails")
	}
}

func TestRun_StartError(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	RegisterModule(func() Module { return &failingModule{id: "fail-start", failOn: "start"} })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret"

	c, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = c.Run(ctx)
	if err == nil {
		t.Fatal("Run should fail when module start fails")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LoadConfig — covers config.go:158 YAML parsing error branch (75% → 100%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestLoadConfig_InvalidYAML(t *testing.T) {
	// Create a temp directory and write an invalid monster.yaml
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	os.WriteFile(yamlPath, []byte("invalid: yaml: [broken"), 0644)

	// Change to the temp dir so LoadConfig finds monster.yaml
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("LoadConfig should fail with invalid YAML")
	}
}

func TestLoadConfig_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	os.WriteFile(yamlPath, []byte("server:\n  port: 9999\n  host: 127.0.0.1\n"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Server.Port != 9999 {
		t.Errorf("port = %d, want 9999", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", cfg.Server.Host)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler.Start — covers scheduler.go:62 (the ticker/stopCh select, 87.5% → 100%)
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// ReloadConfig — covers app.go ReloadConfig hot-reload behavior
// ═══════════════════════════════════════════════════════════════════════════════

func TestReloadConfig_AppliesSafeFields(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")

	// Initial config
	os.WriteFile(yamlPath, []byte("server:\n  port: 8443\n  host: 0.0.0.0\n  log_level: info\n"), 0644)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.Config.Server.LogLevel = "info"
	c.Config.Server.LogFormat = "text"
	c.ConfigPath = yamlPath

	// Modify the YAML file
	os.WriteFile(yamlPath, []byte("server:\n  port: 8443\n  host: 0.0.0.0\n  log_level: debug\n  log_format: json\n"), 0644)

	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}

	if c.Config.Server.LogLevel != "debug" {
		t.Errorf("log_level = %q, want debug", c.Config.Server.LogLevel)
	}
	if c.Config.Server.LogFormat != "json" {
		t.Errorf("log_format = %q, want json", c.Config.Server.LogFormat)
	}
}

func TestReloadConfig_AppliesSafeFields_NilEventBus(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")

	os.WriteFile(yamlPath, []byte("server:\n  port: 8443\n  host: 0.0.0.0\n  log_level: info\n"), 0644)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
	}
	applyDefaults(c.Config)
	c.Config.Server.LogLevel = "info"
	c.ConfigPath = yamlPath

	os.WriteFile(yamlPath, []byte("server:\n  port: 8443\n  host: 0.0.0.0\n  log_level: debug\n"), 0644)

	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
	if c.Config.Server.LogLevel != "debug" {
		t.Errorf("log_level = %q, want debug", c.Config.Server.LogLevel)
	}
}

func TestReloadConfig_NoChanges(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	os.WriteFile(yamlPath, []byte("server:\n  port: 8443\n  host: 0.0.0.0\n"), 0644)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.ConfigPath = yamlPath

	// No changes — should succeed with "no changes detected"
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
}

func TestReloadConfig_InvalidFile(t *testing.T) {
	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.ConfigPath = "/nonexistent/monster.yaml"

	err := c.ReloadConfig()
	if err == nil {
		t.Fatal("ReloadConfig should fail with invalid file path")
	}
}

func TestCore_Run_NilEventBusAndScheduler(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := &Core{
		Config:   cfg,
		Build:    BuildInfo{Version: "test"},
		Registry: NewRegistry(),
		Logger:   discardLogger(),
	}

	if err := c.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestScheduler_Start_StopImmediately(t *testing.T) {
	s := NewScheduler(discardLogger())

	s.Add(&CronJob{
		ID:       "start-stop-job",
		Name:     "quick",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			return nil
		},
	})

	s.Start()
	// Stop immediately to exercise the stopCh branch in the goroutine
	time.Sleep(10 * time.Millisecond)
	s.Stop()
}

func TestScheduler_Start_TickerFires(t *testing.T) {
	// This test exercises the ticker branch inside Start's goroutine.
	// We cannot easily make the 30s ticker fire quickly, but we can
	// verify Start and Stop work without panic when the scheduler
	// has jobs registered.
	s := NewScheduler(discardLogger())

	called := false
	s.Add(&CronJob{
		ID:       "ticker-job",
		Name:     "ticker",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			called = true
			return nil
		},
	})

	s.Start()

	// Force a tick manually to cover the tick() path inside the goroutine
	s.mu.Lock()
	s.jobs["ticker-job"].NextRun = time.Now().Add(-time.Second)
	s.mu.Unlock()

	s.tick()
	time.Sleep(100 * time.Millisecond)

	s.Stop()

	if !called {
		t.Error("handler should have been called via manual tick")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Role.HasPermission — covers wildcard and prefix matching in store.go
// ═══════════════════════════════════════════════════════════════════════════════

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		name       string
		permsJSON  string
		permission string
		want       bool
	}{
		{"exact match", `["app.delete"]`, "app.delete", true},
		{"no match", `["app.view"]`, "app.delete", false},
		{"star wildcard", `["*"]`, "anything.goes", true},
		{"prefix wildcard app", `["app.*"]`, "app.delete", true},
		{"prefix wildcard tenant", `["tenant.*"]`, "tenant.billing", true},
		{"prefix wildcard no dot", `["app.*"]`, "app", false},
		{"prefix mismatch", `["app.*"]`, "tenant.delete", false},
		{"empty perms", `[]`, "app.delete", false},
		{"invalid json", `{bad`, "app.delete", false},
		{"multiple with wildcard", `["app.view","app.*"]`, "app.deploy", true},
		{"exact before wildcard", `["app.delete","app.*"]`, "app.delete", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Role{PermissionsJSON: tt.permsJSON}
			if got := r.HasPermission(tt.permission); got != tt.want {
				t.Errorf("HasPermission(%q) = %v, want %v", tt.permission, got, tt.want)
			}
		})
	}
}

// === merged from coverage_boost_test.go ===

func TestIsDraining(t *testing.T) {
	c := &Core{}
	if c.IsDraining() {
		t.Error("expected not draining initially")
	}
	c.SetDraining()
	if !c.IsDraining() {
		t.Error("expected draining after SetDraining")
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus(nil)
	called := false
	handler := func(_ context.Context, _ Event) error {
		called = true
		return nil
	}

	subID := eb.Subscribe("test.event", handler)
	eb.Publish(context.Background(), Event{Type: "test.event"})
	if !called {
		t.Error("handler should be called before unsubscribe")
	}

	called = false
	eb.Unsubscribe(subID)
	eb.Publish(context.Background(), Event{Type: "test.event"})
	if called {
		t.Error("handler should NOT be called after unsubscribe")
	}
}

func TestWithCorrelationID(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "corr-123")
	if CorrelationIDFromContext(ctx) != "corr-123" {
		t.Errorf("expected correlation ID corr-123, got %s", CorrelationIDFromContext(ctx))
	}
}

func TestCorrelationIDFromContext_Empty(t *testing.T) {
	if CorrelationIDFromContext(context.Background()) != "" {
		t.Error("expected empty correlation ID from background context")
	}
}

func TestContainerOpts_ApplyResourceDefaults(t *testing.T) {
	co := &ContainerOpts{}
	co.ApplyResourceDefaults(512, 512)
	if co.CPUQuota != 512 {
		t.Errorf("CPUQuota = %d, want 512", co.CPUQuota)
	}
	if co.MemoryMB != 512 {
		t.Errorf("MemoryMB = %d, want 512", co.MemoryMB)
	}

	// When values are already set, defaults should not override
	co2 := &ContainerOpts{CPUQuota: 1000, MemoryMB: 2048}
	co2.ApplyResourceDefaults(512, 512)
	if co2.CPUQuota != 1000 {
		t.Errorf("CPUQuota = %d, want 1000", co2.CPUQuota)
	}
	if co2.MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", co2.MemoryMB)
	}

	// When defaults are 0, existing zero values stay zero
	co3 := &ContainerOpts{}
	co3.ApplyResourceDefaults(0, 0)
	if co3.CPUQuota != 0 {
		t.Errorf("CPUQuota = %d, want 0", co3.CPUQuota)
	}
	if co3.MemoryMB != 0 {
		t.Errorf("MemoryMB = %d, want 0", co3.MemoryMB)
	}
}

// === merged from events_extra_test.go ===

func TestEventBus_OnError(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var captured struct {
		mu    sync.Mutex
		event Event
		sub   *Subscription
		err   error
	}

	eb.OnError(func(event Event, sub *Subscription, err error) {
		captured.mu.Lock()
		defer captured.mu.Unlock()
		captured.event = event
		captured.sub = sub
		captured.err = err
	})

	handlerErr := errors.New("handler exploded")
	eb.SubscribeNamed("test.error", "failing-handler", false, func(_ context.Context, _ Event) error {
		return handlerErr
	})

	err := eb.Publish(context.Background(), Event{Type: "test.error", Source: "test"})
	if err == nil {
		t.Fatal("expected error from sync handler, got nil")
	}

	captured.mu.Lock()
	defer captured.mu.Unlock()

	if captured.err == nil {
		t.Fatal("OnError callback was not called")
	}
	if !errors.Is(captured.err, handlerErr) {
		t.Errorf("OnError got error %v, want %v", captured.err, handlerErr)
	}
	if captured.event.Type != "test.error" {
		t.Errorf("OnError got event type %q, want %q", captured.event.Type, "test.error")
	}
	if captured.sub.Name != "failing-handler" {
		t.Errorf("OnError got handler name %q, want %q", captured.sub.Name, "failing-handler")
	}

	// Verify error count is incremented
	stats := eb.Stats()
	if stats.ErrorCount != 1 {
		t.Errorf("expected ErrorCount 1, got %d", stats.ErrorCount)
	}
}

func TestEventBus_OnError_AsyncHandler(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var errorCalled atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	eb.OnError(func(_ Event, _ *Subscription, _ error) {
		errorCalled.Store(true)
		wg.Done()
	})

	eb.SubscribeAsync("test.async.error", func(_ context.Context, _ Event) error {
		return errors.New("async failure")
	})

	// PublishAsync should not block and should not return the async error
	err := eb.Publish(context.Background(), Event{Type: "test.async.error", Source: "test"})
	if err != nil {
		t.Fatalf("async handler error should not propagate, got: %v", err)
	}

	// Wait for async handler to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async error callback")
	}

	if !errorCalled.Load() {
		t.Error("OnError should be called for async handler failures")
	}
}

func TestEventBus_PublishAsync(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var received atomic.Bool

	eb.Subscribe("test.async.publish", func(_ context.Context, _ Event) error {
		received.Store(true)
		return nil
	})

	// PublishAsync should not block
	start := time.Now()
	eb.PublishAsync(context.Background(), Event{Type: "test.async.publish", Source: "test"})
	elapsed := time.Since(start)

	// The call should return almost immediately (not block on handler)
	if elapsed > 500*time.Millisecond {
		t.Errorf("PublishAsync took %v, expected near-instant return", elapsed)
	}

	// Wait for the async goroutine to finish
	time.Sleep(200 * time.Millisecond)

	if !received.Load() {
		t.Error("handler should eventually be called by PublishAsync")
	}

	stats := eb.Stats()
	if stats.PublishCount != 1 {
		t.Errorf("expected PublishCount 1, got %d", stats.PublishCount)
	}
}

func TestEventBus_EmitWithTenant(t *testing.T) {
	tests := []struct {
		name     string
		tenantID string
		userID   string
		data     any
	}{
		{
			name:     "with tenant and user",
			tenantID: "tenant-abc",
			userID:   "user-123",
			data:     map[string]string{"key": "value"},
		},
		{
			name:     "system event with empty tenant",
			tenantID: "",
			userID:   "",
			data:     nil,
		},
		{
			name:     "with tenant only",
			tenantID: "tenant-xyz",
			userID:   "",
			data:     "simple-payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eb := NewEventBus(slog.Default())

			var received Event
			eb.Subscribe("tenant.test", func(_ context.Context, e Event) error {
				received = e
				return nil
			})

			err := eb.EmitWithTenant(
				context.Background(),
				"tenant.test", "source-mod",
				tt.tenantID, tt.userID, tt.data,
			)
			if err != nil {
				t.Fatalf("EmitWithTenant returned error: %v", err)
			}

			if received.Type != "tenant.test" {
				t.Errorf("expected type %q, got %q", "tenant.test", received.Type)
			}
			if received.Source != "source-mod" {
				t.Errorf("expected source %q, got %q", "source-mod", received.Source)
			}
			if received.TenantID != tt.tenantID {
				t.Errorf("expected tenantID %q, got %q", tt.tenantID, received.TenantID)
			}
			if received.UserID != tt.userID {
				t.Errorf("expected userID %q, got %q", tt.userID, received.UserID)
			}
		})
	}
}

func TestEvent_DebugString(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantSub []string // substrings expected in the output
	}{
		{
			name: "full event with all fields",
			event: Event{
				ID:       "abcdef1234567890",
				Type:     "app.deployed",
				Source:   "deploy-module",
				TenantID: "tenant-1",
				UserID:   "user-42",
			},
			wantSub: []string{
				"abcdef12",      // first 8 chars of ID
				"app.deployed",  // event type
				"deploy-module", // source
				"tenant=tenant-1",
				"user=user-42",
			},
		},
		{
			name: "system event with empty tenant and user",
			event: Event{
				ID:       "1234567890abcdef",
				Type:     "system.started",
				Source:   "core",
				TenantID: "",
				UserID:   "",
			},
			wantSub: []string{
				"12345678",
				"system.started",
				"core",
				"tenant=",
				"user=",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.event.DebugString()
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("DebugString() = %q, missing substring %q", got, sub)
				}
			}
		})
	}

	// Verify format matches expected pattern
	e := Event{
		ID:       "abcdef1234567890",
		Type:     "app.created",
		Source:   "api",
		TenantID: "t1",
		UserID:   "u1",
	}
	expected := fmt.Sprintf("[%s] %s from %s (tenant=%s user=%s)",
		e.ID[:8], e.Type, e.Source, e.TenantID, e.UserID)
	if got := e.DebugString(); got != expected {
		t.Errorf("DebugString() = %q, want %q", got, expected)
	}
}

func TestNewEventBus_NilLogger(t *testing.T) {
	// Passing nil logger should use slog.Default()
	eb := NewEventBus(nil)
	if eb == nil {
		t.Fatal("NewEventBus(nil) returned nil")
	}
	if eb.logger == nil {
		t.Error("logger should be set to default, not nil")
	}
}

func TestEventBus_PublishAsync_WithSyncError(t *testing.T) {
	// Test that PublishAsync logs errors from synchronous handlers
	eb := NewEventBus(slog.Default())

	var wg sync.WaitGroup
	wg.Add(1)

	eb.SubscribeNamed("test.sync.error", "failing-sync", false, func(_ context.Context, _ Event) error {
		defer wg.Done()
		return errors.New("sync handler error in async publish")
	})

	// PublishAsync should not block on error
	eb.PublishAsync(context.Background(), Event{Type: "test.sync.error", Source: "test"})

	// Wait for the handler to be called
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Good - handler was called
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for handler")
	}

	// Verify error count was incremented
	stats := eb.Stats()
	if stats.ErrorCount != 1 {
		t.Errorf("expected ErrorCount 1, got %d", stats.ErrorCount)
	}
}

func TestEventBus_PublishAsync_WithPresetID(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var receivedID string
	var wg sync.WaitGroup
	wg.Add(1)

	eb.Subscribe("test.preset.id", func(_ context.Context, e Event) error {
		receivedID = e.ID
		wg.Done()
		return nil
	})

	presetID := "preset-id-12345"
	eb.PublishAsync(context.Background(), Event{
		ID:   presetID,
		Type: "test.preset.id",
	})

	wg.Wait()

	// Should use the preset ID, not generate a new one
	if receivedID != presetID {
		t.Errorf("ID = %q, want %q", receivedID, presetID)
	}
}

func TestEventBus_PublishAsync_WithPresetTimestamp(t *testing.T) {
	eb := NewEventBus(slog.Default())

	presetTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	var receivedTime time.Time
	var wg sync.WaitGroup
	wg.Add(1)

	eb.Subscribe("test.preset.time", func(_ context.Context, e Event) error {
		receivedTime = e.Timestamp
		wg.Done()
		return nil
	})

	eb.PublishAsync(context.Background(), Event{
		Type:      "test.preset.time",
		Timestamp: presetTime,
	})

	wg.Wait()

	// Should use the preset timestamp, not generate a new one
	if !receivedTime.Equal(presetTime) {
		t.Errorf("Timestamp = %v, want %v", receivedTime, presetTime)
	}
}

// === merged from reload_integration_test.go ===

// writeYAML is a tiny helper so test bodies stay short — each case
// only needs to vary a handful of fields.
func writeYAML(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// simulatedDeploy mimics the Config-read pattern of a deploy pipeline
// in flight: every iteration snapshots a handful of fields that a
// real Build/Deploy step would inspect (worker-pool caps, CORS origins,
// registration gating, log level, etc.). The test spawns several of
// these alongside ReloadConfig callers to drive the concurrent path.
//
// Returns the number of iterations the goroutine completed before
// ctx was canceled. Any visible corruption (nil-pointer panic, a
// reload mid-field that leaves a struct half-mutated) would surface
// as a recover() catching a panic; the test asserts the recovered
// count stays at zero.
func simulatedDeploy(ctx context.Context, c *Core, panics *atomic.Int64) int {
	iterations := 0
	defer func() {
		if r := recover(); r != nil {
			panics.Add(1)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return iterations
		default:
		}
		// Read every field ReloadConfig is allowed to mutate. We
		// intentionally consume each value so the compiler cannot
		// hoist the read out of the loop. Reads are bracketed by the
		// Core.configMu RLock so the race detector sees the same
		// synchronization that ReloadConfig uses on the write side.
		c.ConfigRLock()
		_ = c.Config.Server.LogLevel
		_ = c.Config.Server.LogFormat
		_ = c.Config.Server.CORSOrigins
		_ = c.Config.Registration.Mode
		_ = c.Config.Backup.Schedule
		_ = c.Config.Limits.MaxAppsPerTenant
		_ = c.Config.Limits.MaxConcurrentBuilds
		c.ConfigRUnlock()
		iterations++
		// A brief yield so the scheduler actually interleaves this
		// loop with the reloader goroutine; without a yield the Go
		// runtime tends to let one goroutine run to completion on a
		// lightly loaded machine.
		if iterations%100 == 0 {
			time.Sleep(time.Microsecond)
		}
	}
}

// TestReloadConfig_ConcurrentWithInFlightDeploy exercises the
// Roadmap 3.2.4 scenario: SIGHUP / ReloadConfig is fired while a
// deploy is already reading Config fields, and the deploy has to
// keep running without corruption.
//
// The test cannot assert "zero races" without the race detector
// (which requires cgo on Windows) — what it CAN assert is:
//
//  1. No reader panics. A torn read that happened to land on a nil
//     pointer or out-of-range index would recover() here.
//  2. After every reload completes, Core.Config matches the file
//     that was last written. No stale snapshot survives.
//  3. The EventConfigReloaded event fires once per applied reload.
//  4. The concurrent readers together complete far more iterations
//     than the number of reloads, proving the reload path did not
//     block the deploy pipeline.
func TestReloadConfig_ConcurrentWithInFlightDeploy(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")

	// Initial config: low traffic settings.
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
  log_format: text
  cors_origins: "https://app.example.com"
registration:
  mode: open
backup:
  schedule: "02:00"
limits:
  max_apps_per_tenant: 50
  max_concurrent_builds: 3
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	// Seed the in-memory config with the values from the YAML so
	// the first ReloadConfig sees a non-trivial delta.
	c.Config.Server.LogLevel = "info"
	c.Config.Server.LogFormat = "text"
	c.Config.Server.CORSOrigins = "https://app.example.com"
	c.Config.Registration.Mode = "open"
	c.Config.Backup.Schedule = "02:00"
	c.Config.Limits.MaxAppsPerTenant = 50
	c.Config.Limits.MaxConcurrentBuilds = 3
	c.ConfigPath = yamlPath

	// Count EventConfigReloaded so the test can assert the event
	// fired exactly once per applied reload. Sync subscriber — the
	// ReloadConfig caller uses PublishAsync which spawns a goroutine,
	// so we wait for Events.Drain at the end.
	var reloadEvents atomic.Int64
	c.Events.Subscribe(EventConfigReloaded, func(_ context.Context, _ Event) error {
		reloadEvents.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	var panics atomic.Int64
	var readerWG sync.WaitGroup
	var totalIters atomic.Int64

	// Launch 5 concurrent "in-flight deploys". Five is enough to get
	// real interleaving without being flaky on slow CI.
	for i := 0; i < 5; i++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			totalIters.Add(int64(simulatedDeploy(ctx, c, &panics)))
		}()
	}

	// Run a series of reloads while the deploys are churning. Each
	// iteration rewrites the YAML with a different value so
	// ReloadConfig is guaranteed to see a delta.
	const reloadRounds = 10
	for i := 0; i < reloadRounds; i++ {
		maxBuilds := 4 + i
		writeYAML(t, yamlPath, fmt.Sprintf(`server:
  port: 8443
  host: 0.0.0.0
  log_level: debug
  log_format: json
  cors_origins: "https://app-%d.example.com"
registration:
  mode: invite_only
backup:
  schedule: "03:%02d"
limits:
  max_apps_per_tenant: %d
  max_concurrent_builds: %d
`, i, i, 100+i, maxBuilds))
		if err := c.ReloadConfig(); err != nil {
			t.Fatalf("reload %d: %v", i, err)
		}
		// Yield so the readers observe the new state before the next
		// rewrite.
		time.Sleep(2 * time.Millisecond)
	}

	cancel()
	readerWG.Wait()
	c.Events.Drain()

	if panics.Load() != 0 {
		t.Errorf("simulatedDeploy panicked %d times — torn read from ReloadConfig",
			panics.Load())
	}

	// Sanity: readers did real work. If the reload path had somehow
	// deadlocked the deploy goroutines, totalIters would be close to
	// zero.
	if totalIters.Load() < int64(reloadRounds*100) {
		t.Errorf("simulatedDeploy iterations = %d, want >> %d (reload should not block readers)",
			totalIters.Load(), reloadRounds*100)
	}

	// Final state: the newest YAML should be the active config.
	wantMaxBuilds := 4 + (reloadRounds - 1)
	if c.Config.Limits.MaxConcurrentBuilds != wantMaxBuilds {
		t.Errorf("final MaxConcurrentBuilds = %d, want %d (last reload not applied)",
			c.Config.Limits.MaxConcurrentBuilds, wantMaxBuilds)
	}
	if c.Config.Server.LogLevel != "debug" {
		t.Errorf("final LogLevel = %q, want %q", c.Config.Server.LogLevel, "debug")
	}
	if c.Config.Registration.Mode != "invite_only" {
		t.Errorf("final Registration.Mode = %q, want %q",
			c.Config.Registration.Mode, "invite_only")
	}

	// Event count: one ConfigReloaded event per reload round since
	// every round mutates at least one field.
	if got := reloadEvents.Load(); got != int64(reloadRounds) {
		t.Errorf("EventConfigReloaded fired %d times, want %d", got, reloadRounds)
	}
}

// TestReloadConfig_NoChangesSkipsEvent verifies the "no changes"
// fast path does not publish a reload event. This is the correctness
// check the SIGHUP handler in main.go relies on to avoid spamming
// EventConfigReloaded when an operator SIGHUPs a config file they
// haven't actually edited.
func TestReloadConfig_NoChangesSkipsEvent(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
  log_format: text
registration:
  mode: open
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.Config.Server.LogLevel = "info"
	c.Config.Server.LogFormat = "text"
	c.Config.Registration.Mode = "open"
	c.ConfigPath = yamlPath

	var reloadEvents atomic.Int64
	c.Events.Subscribe(EventConfigReloaded, func(_ context.Context, _ Event) error {
		reloadEvents.Add(1)
		return nil
	})

	// Reload twice with no file changes between calls.
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("first reload: %v", err)
	}
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("second reload: %v", err)
	}
	c.Events.Drain()

	if got := reloadEvents.Load(); got != 0 {
		t.Errorf("no-change reload fired %d EventConfigReloaded events, want 0", got)
	}
}

// TestReloadConfig_InFlightDeployOnlyReadsAtomicStructs is a
// regression guard: ReloadConfig MUST mutate Core.Config in-place
// (not swap a pointer) because modules that grabbed a reference to
// *Config at Init time still hold that pointer. Swapping the pointer
// would orphan all live references and break hot-reload silently.
//
// The test grabs *Config before any reload, then compares it to
// *Core.Config after reload — they must still be the same pointer.
func TestReloadConfig_InFlightDeployOnlyReadsAtomicStructs(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: info
`)

	c := &Core{
		Config: &Config{},
		Logger: discardLogger(),
		Events: NewEventBus(discardLogger()),
	}
	applyDefaults(c.Config)
	c.ConfigPath = yamlPath

	// Snapshot the pointer a freshly-initialized module would keep.
	modulePtr := c.Config

	writeYAML(t, yamlPath, `server:
  port: 8443
  host: 0.0.0.0
  log_level: warn
`)
	if err := c.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}

	if c.Config != modulePtr {
		t.Fatal("ReloadConfig swapped Core.Config pointer — existing module references would observe stale fields")
	}
	if modulePtr.Server.LogLevel != "warn" {
		t.Errorf("module's config pointer LogLevel = %q, want %q (reload did not mutate in place)",
			modulePtr.Server.LogLevel, "warn")
	}
}

// === merged from scheduler_extra_test.go ===

func TestScheduler_Stop(t *testing.T) {
	s := NewScheduler(slog.Default())

	var tickCount atomic.Int32
	s.Add(&CronJob{
		Name:     "counter",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			tickCount.Add(1)
			return nil
		},
	})

	s.Start()

	// Let the scheduler run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop should not panic and should halt the scheduler loop
	s.Stop()

	// Give the goroutine time to exit
	time.Sleep(100 * time.Millisecond)

	// Record the count after stop
	countAfterStop := tickCount.Load()

	// Wait again to verify no more ticks happen
	time.Sleep(200 * time.Millisecond)
	countLater := tickCount.Load()

	if countLater != countAfterStop {
		t.Errorf("scheduler continued ticking after Stop: count went from %d to %d",
			countAfterStop, countLater)
	}
}

func TestScheduler_StartAndStop_NoJobs(t *testing.T) {
	s := NewScheduler(slog.Default())

	// Start with no jobs should not panic
	s.Start()

	time.Sleep(50 * time.Millisecond)

	// Stop with no jobs should not panic
	s.Stop()
}

func TestScheduler_Stop_Idempotent(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.Start()

	time.Sleep(50 * time.Millisecond)

	// First stop should work
	s.Stop()

	// Second stop on a closed channel would panic if not handled,
	// but the current implementation uses close() which will panic.
	// This test documents the behavior: calling Stop() twice panics.
	// If the implementation is changed to be idempotent, update this test.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop() panicked on second call: %v", r)
		}
	}()

	// This should be safe if the implementation is idempotent
	s.Stop()
}

func TestScheduler_CalcNextRun_EveryInterval(t *testing.T) {
	s := NewScheduler(slog.Default())

	tests := []struct {
		schedule string
		minDur   time.Duration
		maxDur   time.Duration
	}{
		{"@every 5m", 4*time.Minute + 59*time.Second, 5*time.Minute + 1*time.Second},
		{"@every 1h", 59*time.Minute + 59*time.Second, 1*time.Hour + 1*time.Second},
		{"@every 30s", 29 * time.Second, 31 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			before := time.Now()
			next := s.calcNextRun(tt.schedule)
			diff := next.Sub(before)

			if diff < tt.minDur || diff > tt.maxDur {
				t.Errorf("calcNextRun(%q) = %v from now, expected between %v and %v",
					tt.schedule, diff, tt.minDur, tt.maxDur)
			}
		})
	}
}

func TestScheduler_CalcNextRun_HHMMFormat(t *testing.T) {
	s := NewScheduler(slog.Default())

	// Use 05:00 to avoid DST transition issues (DST in Turkey is at 03:00 on March 30)
	next := s.calcNextRun("05:00")

	// The result should be a valid time in the future or at most 24h from now
	now := time.Now()
	if next.Before(now.Add(-time.Second)) {
		t.Error("calcNextRun('05:00') returned a time in the past")
	}
	diff := next.Sub(now)
	if diff > 25*time.Hour {
		t.Errorf("calcNextRun('05:00') returned time %v in the future, expected <= 24h", diff)
	}

	// Verify it targets 05:00
	if next.Hour() != 5 || next.Minute() != 0 {
		t.Errorf("calcNextRun('05:00') = %02d:%02d, want 05:00", next.Hour(), next.Minute())
	}
}

// === merged from tier70_hardening_test.go ===

// Tier 70 — core scheduler lifecycle + ctx plumbing tests.
//
// These cover the regressions fixed in Tier 70:
//   - NewScheduler nil-logger guard
//   - Stop idempotency (stopOnce-guarded double close)
//   - Stop waits for loop AND in-flight handlers (wg.Wait)
//   - Start idempotency (startOnce prevents duplicate goroutines)
//   - Stop without Start does not deadlock on wg.Wait
//   - Cancellable stopCtx plumbed to every handler
//   - Handler panic recovery keeps the scheduler alive
//   - Per-job Timeout override
//   - parseHHMM rejects garbage input
//   - calcNextRun falls back safely on bad schedules
//   - runCtx nil fallback for struct-literal construction

// ─── NewScheduler nil-logger guard ─────────────────────────────────────────

func TestTier70_NewScheduler_NilLogger(t *testing.T) {
	s := NewScheduler(nil)
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if s.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if s.stopCtx == nil || s.stopCancel == nil {
		t.Error("stopCtx/stopCancel should be initialized")
	}
	if s.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier70_Scheduler_Stop_Idempotent(t *testing.T) {
	s := NewScheduler(tier70Logger())
	s.Start()

	// Double-Stop must not panic. Before Tier 70 the second call
	// panicked with "close of closed channel" because there was no
	// stopOnce guard.
	s.Stop()
	s.Stop()
}

func TestTier70_Scheduler_Stop_WithoutStart_Safe(t *testing.T) {
	s := NewScheduler(tier70Logger())
	// Must not deadlock on wg.Wait — nothing was added to the group.
	s.Stop()
	s.Stop()
}

// ─── Start idempotency ─────────────────────────────────────────────────────

func TestTier70_Scheduler_Start_Idempotent(t *testing.T) {
	s := NewScheduler(tier70Logger())

	// Starting twice must not double-count wg. If it did, Stop would
	// block forever waiting for a phantom second goroutine.
	s.Start()
	s.Start()

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked — startOnce/wg balance is wrong")
	}
}

// ─── Stop waits for the loop goroutine ─────────────────────────────────────

func TestTier70_Scheduler_Stop_WaitsForLoop(t *testing.T) {
	s := NewScheduler(tier70Logger())
	s.Start()

	// Give the goroutine a moment to enter its select.
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — wg.Wait missing or deadlock")
	}
}

// ─── Stop waits for in-flight handlers ─────────────────────────────────────

// TestTier70_Scheduler_Stop_WaitsForInFlightHandler proves that Stop
// actually blocks until dispatched handler goroutines drain. Before
// Tier 70 the handler goroutine was not tracked by wg, so a slow job
// could keep running long after Stop returned.
func TestTier70_Scheduler_Stop_WaitsForInFlightHandler(t *testing.T) {
	s := NewScheduler(tier70Logger())

	started := make(chan struct{})
	finished := atomic.Bool{}
	s.Add(&CronJob{
		Name:     "slow",
		Schedule: "@every 1h",
		Handler: func(ctx context.Context) error {
			close(started)
			// Sleep briefly but longer than the test's wait-before-Stop.
			select {
			case <-ctx.Done():
			case <-time.After(200 * time.Millisecond):
			}
			finished.Store(true)
			return nil
		},
	})

	// Force the job to be due immediately and dispatch it through tick.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
	s.tick()

	// Wait for the handler to enter.
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler never started")
	}

	// Stop must block until the handler returns.
	s.Stop()
	if !finished.Load() {
		t.Error("Stop returned before in-flight handler finished")
	}
}

// ─── Stop cancels in-flight handler via ctx ────────────────────────────────

// TestTier70_Scheduler_Stop_CancelsInFlightHandler proves the handler
// context is derived from stopCtx so Stop can abort a long-running job
// at its next context checkpoint.
func TestTier70_Scheduler_Stop_CancelsInFlightHandler(t *testing.T) {
	s := NewScheduler(tier70Logger())

	started := make(chan struct{})
	var observedCancel atomic.Bool
	s.Add(&CronJob{
		Name:     "blocker",
		Schedule: "@every 1h",
		Handler: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			observedCancel.Store(true)
			return ctx.Err()
		},
	})

	// Make it due and dispatch.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
	s.tick()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocker handler never started")
	}

	// Stop cancels the parent ctx; the handler must observe cancellation.
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — handler ctx cancellation is not plumbed")
	}

	if !observedCancel.Load() {
		t.Error("handler did not observe ctx cancellation")
	}
}

// ─── Handler panic recovery ────────────────────────────────────────────────

// TestTier70_Scheduler_HandlerPanic_Recovered proves a panicking
// handler does not tear the whole process down. Before Tier 70 the
// dispatch goroutine had no defer/recover so one bad job would take
// the scheduler with it.
func TestTier70_Scheduler_HandlerPanic_Recovered(t *testing.T) {
	s := NewScheduler(tier70Logger())

	s.Add(&CronJob{
		Name:     "kaboom",
		Schedule: "@every 1h",
		Handler: func(_ context.Context) error {
			panic("boom")
		},
	})

	// Force the job to be due.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()

	// tick must not panic. If the recover is missing, this test
	// crashes the whole test binary.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped dispatch goroutine: %v", r)
		}
	}()
	s.tick()

	// Give the goroutine a moment to run + recover.
	time.Sleep(100 * time.Millisecond)

	// After a panic, Running should be reset so the job can run again.
	s.mu.RLock()
	var stillRunning bool
	for _, j := range s.jobs {
		if j.Running {
			stillRunning = true
		}
	}
	s.mu.RUnlock()
	if stillRunning {
		t.Error("job.Running was not reset after panic recovery")
	}

	// Stop must also still work cleanly.
	s.Stop()
}

// ─── Per-job Timeout override ──────────────────────────────────────────────

// TestTier70_Scheduler_Job_TimeoutOverride proves that CronJob.Timeout,
// when set, bounds a single handler invocation. We set a 30ms timeout
// and block in the handler until the ctx fires.
func TestTier70_Scheduler_Job_TimeoutOverride(t *testing.T) {
	s := NewScheduler(tier70Logger())
	defer s.Stop()

	observed := make(chan time.Duration, 1)
	s.Add(&CronJob{
		Name:     "bounded",
		Schedule: "@every 1h",
		Timeout:  30 * time.Millisecond,
		Handler: func(ctx context.Context) error {
			start := time.Now()
			<-ctx.Done()
			observed <- time.Since(start)
			return ctx.Err()
		},
	})

	// Force the job to be due.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
	s.tick()

	select {
	case d := <-observed:
		if d > 500*time.Millisecond {
			t.Errorf("handler ctx fired after %v — Timeout override ignored", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler never observed ctx cancellation — Timeout override not plumbed")
	}
}

// ─── parseHHMM validation ──────────────────────────────────────────────────

func TestTier70_ParseHHMM_InvalidInput(t *testing.T) {
	bad := []string{
		"",         // empty
		"abc",      // no colon
		"99:00",    // hour out of range
		"12:99",    // minute out of range
		"-1:00",    // negative hour
		"12:-1",    // negative minute
		"ab:cd",    // non-numeric
		"12",       // missing colon
		"12:00:00", // too many parts — third field treated as minute
		"  :  ",    // only a colon
	}
	for _, in := range bad {
		if _, err := parseHHMM(in); err == nil {
			t.Errorf("parseHHMM(%q) = nil error, expected failure", in)
		}
	}
}

func TestTier70_ParseHHMM_ValidInput(t *testing.T) {
	cases := []struct {
		in  string
		out int
	}{
		{"00:00", 0},
		{"12:34", 12*60 + 34},
		{"23:59", 23*60 + 59},
		{"  7:05  ", 7*60 + 5}, // TrimSpace handling
	}
	for _, c := range cases {
		got, err := parseHHMM(c.in)
		if err != nil {
			t.Errorf("parseHHMM(%q) errored: %v", c.in, err)
			continue
		}
		if got != c.out {
			t.Errorf("parseHHMM(%q) = %d, want %d", c.in, got, c.out)
		}
	}
}

// ─── calcNextRun fallback on bad schedules ─────────────────────────────────

func TestTier70_CalcNextRun_InvalidSchedule_FallsBackTo1h(t *testing.T) {
	s := NewScheduler(tier70Logger())

	inputs := []string{
		"@every garbage",
		"not-a-schedule",
		"99:99",
	}
	for _, in := range inputs {
		before := time.Now()
		next := s.calcNextRun(in)
		diff := next.Sub(before)
		// Should fall back to ~1h, never wedge or panic.
		if diff < 55*time.Minute || diff > 65*time.Minute {
			t.Errorf("calcNextRun(%q) = %v from now, expected ~1h fallback", in, diff)
		}
	}
}

// ─── runCtx nil fallback ──────────────────────────────────────────────────

func TestTier70_Scheduler_RunCtx_NilFallback(t *testing.T) {
	// Bare struct literal — no NewScheduler, so stopCtx is nil.
	s := &Scheduler{logger: tier70Logger()}
	ctx := s.runCtx()
	if ctx == nil {
		t.Fatal("runCtx must not return nil")
	}
	if ctx.Err() != nil {
		t.Errorf("fallback background context should not be canceled: %v", ctx.Err())
	}
}

// ─── Concurrent Start+Stop storm ───────────────────────────────────────────

// TestTier70_Scheduler_ConcurrentStartStop exercises the startOnce /
// stopOnce guards under concurrent pressure. Before Tier 70 the
// concurrent double-close would race with a close-of-closed-channel
// panic.
func TestTier70_Scheduler_ConcurrentStartStop(t *testing.T) {
	s := NewScheduler(tier70Logger())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Start() }()
		go func() { defer wg.Done(); s.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	s.Stop()
}

// ─── Disabled jobs are skipped ─────────────────────────────────────────────

func TestTier70_Scheduler_DisabledJob_NotDispatched(t *testing.T) {
	s := NewScheduler(tier70Logger())
	defer s.Stop()

	var called atomic.Bool
	s.Add(&CronJob{
		Name:     "disabled",
		Schedule: "@every 1h",
		Handler: func(_ context.Context) error {
			called.Store(true)
			return nil
		},
	})

	// Disable it and make it due.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.Enabled = false
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()

	s.tick()
	time.Sleep(50 * time.Millisecond)

	if called.Load() {
		t.Error("disabled job was dispatched")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier70Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
