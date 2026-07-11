package core

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

// =============================================================================
// Registry.StopAll — error from module.Stop
// =============================================================================

type covStopErrorMod struct {
	IDValue    string
	NameValue  string
	VersionVal string
	StopError  error
}

func (m *covStopErrorMod) Init(_ context.Context, _ *Core) error  { return nil }
func (m *covStopErrorMod) Start(_ context.Context) error           { return nil }
func (m *covStopErrorMod) Stop(_ context.Context) error            { return m.StopError }
func (m *covStopErrorMod) Health() HealthStatus                    { return HealthOK }
func (m *covStopErrorMod) Routes() []Route                         { return nil }
func (m *covStopErrorMod) Events() []EventHandler                  { return nil }
func (m *covStopErrorMod) ID() string                              { return m.IDValue }
func (m *covStopErrorMod) Name() string                            { return m.NameValue }
func (m *covStopErrorMod) Version() string                         { return m.VersionVal }
func (m *covStopErrorMod) Dependencies() []string                  { return nil }

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
