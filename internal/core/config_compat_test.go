package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

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
  secret_key: legacy-secret-key-minimum-16-chars
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
	if cfg.Server.SecretKey != "legacy-secret-key-minimum-16-chars" {
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
  secret_key: yaml-secret-key-minimum-16-chars
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

	// YAML beats defaults (no env override for secret_key)
	if cfg.Server.SecretKey != "yaml-secret-key-minimum-16-chars" {
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
