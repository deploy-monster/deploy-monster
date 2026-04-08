package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all application configuration.
type Config struct {
	Server        ServerConfig       `yaml:"server"`
	Database      DatabaseConfig     `yaml:"database"`
	Ingress       IngressConfig      `yaml:"ingress"`
	ACME          ACMEConfig         `yaml:"acme"`
	DNS           DNSConfig          `yaml:"dns"`
	Docker        DockerConfig       `yaml:"docker"`
	Backup        BackupConfig       `yaml:"backup"`
	Notifications NotificationConfig `yaml:"notifications"`
	Swarm         SwarmConfig        `yaml:"swarm"`
	VPSProviders  VPSProvidersConfig `yaml:"vps_providers"`
	GitSources    GitSourcesConfig   `yaml:"git_sources"`
	Marketplace   MarketplaceConfig  `yaml:"marketplace"`
	Registration  RegistrationConfig `yaml:"registration"`
	SSO           SSOConfig          `yaml:"sso"`
	Secrets       SecretsConfig      `yaml:"secrets"`
	Billing       BillingConfig      `yaml:"billing"`
	Limits        LimitsConfig       `yaml:"limits"`
	Enterprise    EnterpriseConfig   `yaml:"enterprise"`
}

// ServerConfig holds the HTTP server configuration.
type ServerConfig struct {
	Host               string   `yaml:"host"`
	Port               int      `yaml:"port"`
	Domain             string   `yaml:"domain"`
	SecretKey          string   `yaml:"secret_key"`
	PreviousSecretKeys []string `yaml:"previous_secret_keys"` // old keys kept for graceful JWT rotation
	CORSOrigins        string   `yaml:"cors_origins"`         // comma-separated allowed origins; empty = derive from domain
	EnablePprof        bool     `yaml:"enable_pprof"`         // opt-in: expose /debug/pprof/* endpoints (auth-protected)
}

// DatabaseConfig holds database configuration.
type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	Path   string `yaml:"path"`
	URL    string `yaml:"url"`
}

// IngressConfig holds ingress gateway configuration.
type IngressConfig struct {
	HTTPPort    int  `yaml:"http_port"`
	HTTPSPort   int  `yaml:"https_port"`
	EnableHTTPS bool `yaml:"enable_https"`
}

// ACMEConfig holds ACME/Let's Encrypt configuration.
type ACMEConfig struct {
	Email    string `yaml:"email"`
	Staging  bool   `yaml:"staging"`
	CertDir  string `yaml:"cert_dir"`
	Provider string `yaml:"provider"` // http-01, dns-01
}

// DNSConfig holds DNS provider configuration.
type DNSConfig struct {
	Provider        string `yaml:"provider"` // cloudflare, route53, manual
	CloudflareToken string `yaml:"cloudflare_token"`
	AutoSubdomain   string `yaml:"auto_subdomain"` // e.g., deploy.monster
}

// DockerConfig holds Docker connection configuration.
type DockerConfig struct {
	Host       string `yaml:"host"`
	APIVersion string `yaml:"api_version"`
	TLSVerify  bool   `yaml:"tls_verify"`
}

// BackupConfig holds backup configuration.
type BackupConfig struct {
	Schedule      string `yaml:"schedule"`
	RetentionDays int    `yaml:"retention_days"`
	StoragePath   string `yaml:"storage_path"`
	Encryption    bool   `yaml:"encryption"`
}

// NotificationConfig holds notification channel configuration.
type NotificationConfig struct {
	EmailSMTP      string `yaml:"email_smtp"`
	SlackWebhook   string `yaml:"slack_webhook"`
	DiscordWebhook string `yaml:"discord_webhook"`
	TelegramToken  string `yaml:"telegram_token"`
}

// SwarmConfig holds Docker Swarm configuration.
type SwarmConfig struct {
	Enabled   bool   `yaml:"enabled"`
	ManagerIP string `yaml:"manager_ip"`
	JoinToken string `yaml:"join_token"`
}

// VPSProvidersConfig holds VPS provider configuration.
type VPSProvidersConfig struct {
	Enabled bool `yaml:"enabled"`
}

// GitSourcesConfig holds git source configuration.
type GitSourcesConfig struct {
	GitHubClientID     string `yaml:"github_client_id"`
	GitHubClientSecret string `yaml:"github_client_secret"`
	GitLabClientID     string `yaml:"gitlab_client_id"`
	GitLabClientSecret string `yaml:"gitlab_client_secret"`
}

// MarketplaceConfig holds marketplace configuration.
type MarketplaceConfig struct {
	Enabled       bool   `yaml:"enabled"`
	TemplatesDir  string `yaml:"templates_dir"`
	CommunitySync bool   `yaml:"community_sync"`
}

// RegistrationConfig holds user registration configuration.
type RegistrationConfig struct {
	Mode string `yaml:"mode"` // open, invite_only, approval, disabled, sso_only
}

// SSOConfig holds SSO provider configuration.
type SSOConfig struct {
	GoogleClientID     string `yaml:"google_client_id"`
	GoogleClientSecret string `yaml:"google_client_secret"`
}

// SecretsConfig holds secret vault configuration.
type SecretsConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
}

// BillingConfig holds billing configuration.
type BillingConfig struct {
	Enabled          bool   `yaml:"enabled"`
	StripeSecretKey  string `yaml:"stripe_secret_key"`
	StripeWebhookKey string `yaml:"stripe_webhook_key"`
}

// LimitsConfig holds default resource limits.
type LimitsConfig struct {
	MaxAppsPerTenant    int `yaml:"max_apps_per_tenant"`
	MaxBuildMinutes     int `yaml:"max_build_minutes"`
	MaxConcurrentBuilds int `yaml:"max_concurrent_builds"`
}

// EnterpriseConfig holds enterprise feature configuration.
type EnterpriseConfig struct {
	Enabled    bool   `yaml:"enabled"`
	LicenseKey string `yaml:"license_key"`
}

// LoadConfig loads configuration from monster.yaml, applies env var overrides,
// and sets defaults. If configPath is non-empty, it is used instead of the
// default search paths. Priority: env vars > yaml > defaults.
func LoadConfig(configPath string) (*Config, error) {
	cfg := &Config{}
	applyDefaults(cfg)

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse %s: %w", configPath, err)
		}
	} else {
		// Try loading monster.yaml from standard locations
		for _, path := range []string{
			"monster.yaml",
			"/etc/deploymonster/monster.yaml",
			"/var/lib/deploymonster/monster.yaml",
		} {
			data, err := os.ReadFile(path)
			if err == nil {
				if err := yaml.Unmarshal(data, cfg); err != nil {
					return nil, fmt.Errorf("parse %s: %w", path, err)
				}
				break
			}
		}
	}

	applyEnvOverrides(cfg)

	// Auto-generate secret key if not set
	if cfg.Server.SecretKey == "" {
		cfg.Server.SecretKey = GenerateSecret(32)
	}

	// Derive CORS origins from server domain if not explicitly set
	if cfg.Server.CORSOrigins == "" && cfg.Server.Domain != "" {
		origin := "https://" + cfg.Server.Domain
		if cfg.Server.Port != 443 && cfg.Server.Port != 80 {
			origin = fmt.Sprintf("https://%s:%d", cfg.Server.Domain, cfg.Server.Port)
		}
		cfg.Server.CORSOrigins = origin
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that the configuration is well-formed and catches common misconfiguration.
func (c *Config) Validate() error {
	// Server
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("config: server.port %d out of range (1-65535)", c.Server.Port)
	}
	if len(c.Server.SecretKey) < 16 {
		return fmt.Errorf("config: server.secret_key must be at least 16 characters")
	}

	// Database
	switch c.Database.Driver {
	case "sqlite":
		if c.Database.Path == "" {
			return fmt.Errorf("config: database.path is required for sqlite driver")
		}
	case "postgres":
		if c.Database.URL == "" {
			return fmt.Errorf("config: database.url is required for postgres driver")
		}
	default:
		return fmt.Errorf("config: unsupported database.driver %q (sqlite, postgres)", c.Database.Driver)
	}

	// Ingress ports
	if c.Ingress.HTTPPort < 1 || c.Ingress.HTTPPort > 65535 {
		return fmt.Errorf("config: ingress.http_port %d out of range (1-65535)", c.Ingress.HTTPPort)
	}
	if c.Ingress.HTTPSPort < 1 || c.Ingress.HTTPSPort > 65535 {
		return fmt.Errorf("config: ingress.https_port %d out of range (1-65535)", c.Ingress.HTTPSPort)
	}

	// Registration mode
	switch c.Registration.Mode {
	case "open", "invite_only", "approval", "disabled", "sso_only":
		// valid
	default:
		return fmt.Errorf("config: registration.mode %q not recognized (open, invite_only, approval, disabled, sso_only)", c.Registration.Mode)
	}

	// Resource limits
	if c.Limits.MaxAppsPerTenant < 0 {
		return fmt.Errorf("config: limits.max_apps_per_tenant must be non-negative")
	}
	if c.Limits.MaxConcurrentBuilds < 1 {
		return fmt.Errorf("config: limits.max_concurrent_builds must be at least 1")
	}

	return nil
}

func applyDefaults(cfg *Config) {
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = 8443
	cfg.Database.Driver = "sqlite"
	cfg.Database.Path = "deploymonster.db"
	cfg.Ingress.HTTPPort = 80
	cfg.Ingress.HTTPSPort = 443
	cfg.Ingress.EnableHTTPS = true
	cfg.ACME.Staging = false
	cfg.ACME.Provider = "http-01"
	cfg.Docker.Host = "unix:///var/run/docker.sock"
	cfg.Backup.RetentionDays = 30
	cfg.Backup.StoragePath = "/var/lib/deploymonster/backups"
	cfg.Backup.Encryption = true
	cfg.Marketplace.Enabled = true
	cfg.Marketplace.TemplatesDir = "marketplace/templates"
	cfg.Registration.Mode = "open"
	cfg.Limits.MaxAppsPerTenant = 100
	cfg.Limits.MaxBuildMinutes = 30
	cfg.Limits.MaxConcurrentBuilds = 5
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("MONSTER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("MONSTER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("MONSTER_DOMAIN"); v != "" {
		cfg.Server.Domain = v
	}
	if v := os.Getenv("MONSTER_SECRET"); v != "" {
		cfg.Server.SecretKey = v
	}
	if v := os.Getenv("MONSTER_PREVIOUS_SECRET_KEYS"); v != "" {
		cfg.Server.PreviousSecretKeys = strings.Split(v, ",")
	}
	if v := os.Getenv("MONSTER_DB_PATH"); v != "" {
		cfg.Database.Path = v
	}
	if v := os.Getenv("MONSTER_DB_URL"); v != "" {
		cfg.Database.URL = v
		cfg.Database.Driver = "postgres"
	}
	if v := os.Getenv("MONSTER_DOCKER_HOST"); v != "" {
		cfg.Docker.Host = v
	}
	if v := os.Getenv("MONSTER_LOG_LEVEL"); v != "" {
		_ = v // handled by logger setup
	}
	if v := os.Getenv("MONSTER_ACME_EMAIL"); v != "" {
		cfg.ACME.Email = v
	}
	if v := os.Getenv("MONSTER_REGISTRATION_MODE"); v != "" {
		cfg.Registration.Mode = v
	}
	if v := os.Getenv("MONSTER_CORS_ORIGINS"); v != "" {
		cfg.Server.CORSOrigins = v
	}
	if os.Getenv("MONSTER_ENABLE_PPROF") == "true" {
		cfg.Server.EnablePprof = true
	}
}
