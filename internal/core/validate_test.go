package core

import "testing"

func TestValidateConfig_Valid(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if err := ValidateConfig(cfg); err != nil {
		t.Errorf("default config should be valid: %v", err)
	}
}

func TestValidateConfig_InvalidPort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.Port = 0

	if err := ValidateConfig(cfg); err == nil {
		t.Error("port 0 should be invalid")
	}
}

func TestValidateConfig_PortConflict(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.Port = 80 // conflicts with HTTP port

	if err := ValidateConfig(cfg); err == nil {
		t.Error("API port conflicting with HTTP port should be invalid")
	}
}

func TestValidateConfig_InvalidDriver(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Database.Driver = "mysql" // not supported as main DB

	if err := ValidateConfig(cfg); err == nil {
		t.Error("mysql driver should be invalid")
	}
}

func TestValidateConfig_PostgresWithoutURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Database.Driver = "postgres"
	cfg.Database.URL = ""

	if err := ValidateConfig(cfg); err == nil {
		t.Error("postgres without URL should be invalid")
	}
}

func TestValidateConfig_InvalidRegMode(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Registration.Mode = "invalid"

	if err := ValidateConfig(cfg); err == nil {
		t.Error("invalid registration mode should fail")
	}
}
