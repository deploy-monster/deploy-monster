package core

import "fmt"

// ValidateConfig checks configuration for common errors before startup.
func ValidateConfig(cfg *Config) error {
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", cfg.Server.Port)
	}

	if cfg.Ingress.HTTPPort <= 0 || cfg.Ingress.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", cfg.Ingress.HTTPPort)
	}

	if cfg.Ingress.HTTPSPort <= 0 || cfg.Ingress.HTTPSPort > 65535 {
		return fmt.Errorf("invalid HTTPS port: %d", cfg.Ingress.HTTPSPort)
	}

	if cfg.Server.Port == cfg.Ingress.HTTPPort {
		return fmt.Errorf("API port (%d) conflicts with HTTP port", cfg.Server.Port)
	}

	if cfg.Server.Port == cfg.Ingress.HTTPSPort {
		return fmt.Errorf("API port (%d) conflicts with HTTPS port", cfg.Server.Port)
	}

	if cfg.Database.Driver != "sqlite" && cfg.Database.Driver != "postgres" {
		return fmt.Errorf("unsupported database driver: %s (use sqlite or postgres)", cfg.Database.Driver)
	}

	if cfg.Database.Driver == "postgres" && cfg.Database.URL == "" {
		return fmt.Errorf("postgres driver requires database.url")
	}

	validModes := map[string]bool{
		"open": true, "invite_only": true, "approval": true, "disabled": true, "sso_only": true,
	}
	if !validModes[cfg.Registration.Mode] {
		return fmt.Errorf("invalid registration mode: %s", cfg.Registration.Mode)
	}

	if cfg.Limits.MaxConcurrentBuilds <= 0 {
		return fmt.Errorf("max_concurrent_builds must be positive")
	}

	return nil
}
