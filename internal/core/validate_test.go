package core

import (
	"runtime"
	"testing"
)

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

func TestValidateVolumePaths(t *testing.T) {
	abs := "/data/myapp"
	if runtime.GOOS == "windows" {
		abs = "C:\\data\\myapp"
	}

	tests := []struct {
		name    string
		volumes map[string]string
		wantErr bool
	}{
		{"nil volumes", nil, false},
		{"empty volumes", map[string]string{}, false},
		{"valid absolute path", map[string]string{abs: "/app/data"}, false},
		{"relative path traversal", map[string]string{"data/../../../etc/shadow": "/app/data"}, true},
		{"relative path", map[string]string{"data/myapp": "/app/data"}, true},
		{"null byte", map[string]string{abs + "/\x00evil": "/app"}, true},
		{"dot-dot only", map[string]string{"..": "/app"}, true},
	}

	// Docker socket tests use Unix paths — only run on Linux where containers execute
	if runtime.GOOS != "windows" {
		tests = append(tests,
			struct {
				name    string
				volumes map[string]string
				wantErr bool
			}{"docker socket blocked", map[string]string{"/var/run/docker.sock": "/var/run/docker.sock"}, true},
			struct {
				name    string
				volumes map[string]string
				wantErr bool
			}{"docker socket alt path blocked", map[string]string{"/run/docker.sock": "/var/run/docker.sock"}, true},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ContainerOpts{Volumes: tt.volumes}
			err := opts.ValidateVolumePaths()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumePaths() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	// Docker socket allowed when AllowDockerSocket is set (Linux only)
	if runtime.GOOS != "windows" {
		t.Run("docker socket allowed with flag", func(t *testing.T) {
			opts := &ContainerOpts{
				Volumes:           map[string]string{"/var/run/docker.sock": "/var/run/docker.sock"},
				AllowDockerSocket: true,
			}
			if err := opts.ValidateVolumePaths(); err != nil {
				t.Errorf("ValidateVolumePaths() with AllowDockerSocket should pass, got: %v", err)
			}
		})
	}
}
