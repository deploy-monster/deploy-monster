package core

import (
	"testing"
)

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
