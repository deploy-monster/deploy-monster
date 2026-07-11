package core

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// NewApp — covers config audit warnings path (app.go:97)
// =============================================================================

func TestNewApp_ConfigAuditSecretsWarning(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret-at-least-32-bytes-long!"
	// Set a plaintext secret that will trigger AuditSecrets warning
	cfg.DNS.CloudflareToken = "plaintext-token"
	// Ensure the env var is not set so the warning fires
	t.Setenv("MONSTER_CLOUDFLARE_TOKEN", "")

	build := BuildInfo{Version: "1.0.0", Commit: "abc", Date: "2024-01-01"}

	// Should not error despite the audit warning
	c, err := NewApp(cfg, build)
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	if c == nil {
		t.Fatal("NewApp returned nil")
	}
}

func TestNewApp_RegisterModulesError(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	// Register a module that will produce a duplicate to force a panic
	RegisterModule(func() Module { return &stubModule{id: "dup"} })
	RegisterModule(func() Module { return &stubModule{id: "dup"} })

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret-at-least-32-bytes-long!"

	_, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("NewApp with duplicate modules should not error: %v", err)
	}
}

// =============================================================================
// ValidateConfig — covers additional error paths (validate.go:6)
// =============================================================================

func TestValidateConfig_InvalidHTTPPort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Ingress.HTTPPort = 0

	if err := ValidateConfig(cfg); err == nil {
		t.Error("expected error for invalid HTTP port")
	}
}

func TestValidateConfig_InvalidHTTPSPort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Ingress.HTTPSPort = 0

	if err := ValidateConfig(cfg); err == nil {
		t.Error("expected error for invalid HTTPS port")
	}
}

func TestValidateConfig_SecretKeyTooShort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "short"

	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "secret_key") {
		t.Errorf("expected error about secret_key, got: %v", err)
	}
}

func TestValidateConfig_PostgresNoURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Database.Driver = "postgres"
	cfg.Database.URL = ""

	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "database.url") {
		t.Errorf("expected error about database.url, got: %v", err)
	}
}

func TestValidateConfig_InvalidRegistrationMode(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Registration.Mode = "invalid_mode"

	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "registration mode") {
		t.Errorf("expected error about registration mode, got: %v", err)
	}
}

func TestValidateConfig_MaxConcurrentBuildsZero(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Limits.MaxConcurrentBuilds = 0

	if err := ValidateConfig(cfg); err == nil || !strings.Contains(err.Error(), "max_concurrent_builds") {
		t.Errorf("expected error about max_concurrent_builds, got: %v", err)
	}
}

// =============================================================================
// Unsubscribe — covers the "not found" path (events.go:126)
// =============================================================================

func TestEventBus_Unsubscribe_NotFound(t *testing.T) {
	eb := NewEventBus(discardLogger())
	// Unsubscribe a non-existent subscription ID
	if eb.Unsubscribe("nonexistent") {
		t.Error("Unsubscribe should return false for non-existent subscription")
	}
}

// =============================================================================
// Publish — covers synchronous handler error path (events.go:143)
// =============================================================================

func TestEventBus_Publish_SyncHandlerError(t *testing.T) {
	eb := NewEventBus(discardLogger())

	// Subscribe a sync handler that returns an error
	sub := eb.Subscribe("test.event", func(_ context.Context, _ Event) error {
		return io.ErrUnexpectedEOF
	})
	if sub == "" {
		t.Fatal("Subscribe returned empty ID")
	}

	err := eb.Publish(context.Background(), NewEvent("test.event", "test", nil))
	if err == nil {
		t.Fatal("expected error from sync handler")
	}
}

// =============================================================================
// PublishAsync — covers the publish wrapper goroutine (events.go:235)
// =============================================================================

func TestEventBus_PublishAsync_ErrorHandled(t *testing.T) {
	eb := NewEventBus(discardLogger())

	handlerErr := make(chan error, 1)
	eb.OnError(func(event Event, sub *Subscription, err error) {
		handlerErr <- err
	})

	sub := eb.SubscribeAsync("test.async", func(_ context.Context, _ Event) error {
		return io.ErrClosedPipe
	})
	if sub == "" {
		t.Fatal("SubscribeAsync returned empty ID")
	}

	eb.PublishAsync(context.Background(), NewEvent("test.async", "test", nil))
	eb.Drain()

	select {
	case err := <-handlerErr:
		if err == nil {
			t.Error("expected non-nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async handler error")
	}
}

func TestEventBus_PublishAsync_PanicRecovery(t *testing.T) {
	eb := NewEventBus(discardLogger())

	sub := eb.SubscribeAsync("test.panic", func(_ context.Context, _ Event) error {
		panic("handler panic")
	})
	if sub == "" {
		t.Fatal("SubscribeAsync returned empty ID")
	}

	// Should not panic
	eb.PublishAsync(context.Background(), NewEvent("test.panic", "panic", nil))
	eb.Drain()
}

// =============================================================================
// DebugString — covers different event types (events.go:563)
// =============================================================================

func TestEvent_DebugString_WithData(t *testing.T) {
	e := NewEvent("test.event", "tester", map[string]string{"key": "val"})
	s := e.DebugString()
	if !strings.Contains(s, "test.event") {
		t.Errorf("DebugString should contain event type: %s", s)
	}
	if !strings.Contains(s, "tester") {
		t.Errorf("DebugString should contain source: %s", s)
	}
}

func TestEvent_DebugString_NilData(t *testing.T) {
	e := Event{
		Type:      "nil.event",
		Source:    "test",
		Data:      nil,
		Timestamp: time.Now(),
		ID:        "test-id",
	}
	s := e.DebugString()
	if !strings.Contains(s, "nil.event") {
		t.Errorf("DebugString should contain event type: %s", s)
	}
}

// =============================================================================
// StopAll — covers error path (registry.go:113)
// =============================================================================

type errorStopModule struct {
	stubModule
	stopErr error
}

func (m *errorStopModule) Stop(_ context.Context) error {
	return m.stopErr
}

func TestRegistry_StopAll_PropagatesError(t *testing.T) {
	r := NewRegistry()
	m1 := &stubModule{id: "m1"}
	m2 := &errorStopModule{stubModule: stubModule{id: "m2"}, stopErr: io.ErrClosedPipe}
	m3 := &stubModule{id: "m3"}

	if err := r.Register(m1); err != nil {
		t.Fatalf("Register m1: %v", err)
	}
	if err := r.Register(m2); err != nil {
		t.Fatalf("Register m2: %v", err)
	}
	if err := r.Register(m3); err != nil {
		t.Fatalf("Register m3: %v", err)
	}

	// Verify resolve works
	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	err := r.StopAll(context.Background())
	if err == nil {
		t.Fatal("StopAll should return error when a module fails to stop")
	}
	if !strings.Contains(err.Error(), "stop") {
		t.Errorf("error = %v, want stop-related error", err)
	}
}

// =============================================================================
// LokiSink — nil logger, flush behavior (logger.go:45, 159)
// =============================================================================

func TestNewLokiSink_NilLogger(t *testing.T) {
	ls := NewLokiSink("http://localhost:3100", "", "", time.Second, nil)
	if ls == nil {
		t.Fatal("NewLokiSink returned nil")
	}
	if ls.logger == nil {
		t.Error("logger should not be nil after construction with nil arg")
	}
	ls.Close()
}

func TestLokiSink_Handle_ClosesDoesNotPanic(t *testing.T) {
	ls := NewLokiSink("http://localhost:3100", "", "", time.Second, nil)
	ls.Close() // Close before Handle to trigger closing path

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	ls.Handle(r) // Should not panic
}

// =============================================================================
// SetupLogger — covers various format paths (logger.go:208)
// =============================================================================

func TestSetupLogger_JSONFormat(t *testing.T) {
	logger := SetupLogger("info", "json", "", "", "", 0)
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_TextFormat(t *testing.T) {
	logger := SetupLogger("info", "text", "", "", "", 0)
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_DebugLevelWithSource(t *testing.T) {
	logger := SetupLogger("debug", "json", "", "", "", 0)
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_WarnLevel(t *testing.T) {
	logger := SetupLogger("warn", "text", "", "", "", 0)
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_ErrorLevel(t *testing.T) {
	logger := SetupLogger("error", "text", "", "", "", 0)
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_WithLokiSink(t *testing.T) {
	logger := SetupLogger("info", "json", "http://loki:3100", "", "", time.Second)
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

// =============================================================================
// Scheduler — tick stopped path, calcNextRun invalid schedule (scheduler.go)
// =============================================================================

func TestScheduler_CalcNextRun_InvalidDuration(t *testing.T) {
	s := NewScheduler(discardLogger())
	result := s.calcNextRun("@every not-a-duration")
	if result.IsZero() {
		t.Fatal("calcNextRun should return a non-zero time even for invalid duration")
	}
}

func TestScheduler_CalcNextRun_InvalidHHMM(t *testing.T) {
	s := NewScheduler(discardLogger())
	result := s.calcNextRun("25:61") // invalid time
	if result.IsZero() {
		t.Fatal("calcNextRun should return a non-zero time even for invalid HH:MM")
	}
}

func TestScheduler_StoppedFlag(t *testing.T) {
	s := NewScheduler(discardLogger())
	s.stopped = true
	// When stopped, tick should return immediately without spawning work
	s.tick()
}

// =============================================================================
// ValidateVolumePaths — covers security checks (interfaces.go:70)
// =============================================================================

func TestValidateVolumePaths_NullByte(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"/var/run/\x00docker.sock": "/var/run/docker.sock"},
	}
	if err := opts.ValidateVolumePaths(); err == nil || !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected null byte error, got: %v", err)
	}
}

func TestValidateVolumePaths_PathTraversal(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"/var/../../etc": "/etc"},
	}
	if err := opts.ValidateVolumePaths(); err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
}

func TestValidateVolumePaths_NotAbsolute(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"relative/path": "/data"},
	}
	if err := opts.ValidateVolumePaths(); err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("expected absolute path error, got: %v", err)
	}
}

func TestValidateVolumePaths_Valid(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"/data/app": "/data"},
	}
	if err := opts.ValidateVolumePaths(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// =============================================================================
// Retry — context cancellation path (retry.go:40)
// =============================================================================

func TestRetry_NoRetryError(t *testing.T) {
	opCount := 0
	err := Retry(context.Background(), DefaultRetryConfig(), func() error {
		opCount++
		return ErrNoRetry(io.ErrNoProgress)
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if opCount != 1 {
		t.Errorf("op called %d times, want 1 (no-retry)", opCount)
	}
}

func TestRetry_LoggerWarning(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	cfg := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
		Logger:       logger,
	}

	opCount := 0
	err := Retry(context.Background(), cfg, func() error {
		opCount++
		if opCount >= 2 {
			return nil
		}
		return io.ErrUnexpectedEOF
	})
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("retrying")) {
		t.Error("expected logger warning about retry")
	}
}

// =============================================================================
// beforeCall — circuit breaker half-open limit path (circuitbreaker.go:139)
// =============================================================================

func TestCircuitBreaker_BeforeCall_HalfOpenLimit(t *testing.T) {
	cb := &CircuitBreaker{
		name:         "test",
		state:        CircuitHalfOpen,
		halfOpenCalls: 1,
		halfOpenMax:  1,
		resetTimeout: time.Hour,
		now:          time.Now,
	}

	err := cb.beforeCall()
	if err == nil {
		t.Fatal("expected error when halfOpenCalls >= halfOpenMax")
	}
}

// =============================================================================
// LoadConfig — config file read error (config.go:241)
// =============================================================================

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/monster.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent config file")
	}
}

func TestLoadConfig_InvalidYAMLPath(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "invalid.yaml")
	os.WriteFile(yamlPath, []byte("server:\n  port: not_a_number\n"), 0644)

	_, err := LoadConfig(yamlPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML values")
	}
}

func TestLoadConfig_CORSOriginFromDomain(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	os.WriteFile(yamlPath, []byte("server:\n  domain: example.com\n  port: 8443\n  secret_key: test-secret-key-at-least-32-bytes-long!\n"), 0644)

	cfg, err := LoadConfig(yamlPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Server.CORSOrigins == "" {
		t.Error("CORSOrigins should be derived from domain")
	}
	if !strings.Contains(cfg.Server.CORSOrigins, "example.com") {
		t.Errorf("CORSOrigins = %q, should contain example.com", cfg.Server.CORSOrigins)
	}
}

func TestLoadConfig_ConfigPathDefaultSearch(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "monster.yaml")
	os.WriteFile(yamlPath, []byte("server:\n  host: 10.0.0.1\n"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("host = %q, want 10.0.0.1", cfg.Server.Host)
	}
}

// =============================================================================
// applyEnvOverrides — covers MONSTER_PORT parse error (config.go:541)
// =============================================================================

func TestApplyEnvOverrides_InvalidPortLogsWarning(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_PORT", "not-a-number")

	applyEnvOverrides(cfg)
	// Port should remain at default since the env var couldn't be parsed
	if cfg.Server.Port != 8443 {
		t.Errorf("port = %d, want 8443 (default unchanged)", cfg.Server.Port)
	}
}

func TestApplyEnvOverrides_WithPostgresURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_DB_URL", "postgres://user:pass@localhost/db")

	applyEnvOverrides(cfg)
	if cfg.Database.Driver != "postgres" {
		t.Errorf("driver = %q, want postgres", cfg.Database.Driver)
	}
	if cfg.Database.URL != "postgres://user:pass@localhost/db" {
		t.Errorf("url = %q", cfg.Database.URL)
	}
}

func TestApplyEnvOverrides_RegistryAuth(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_BUILD_REGISTRY_USERNAME", "user")
	t.Setenv("MONSTER_BUILD_REGISTRY_PASSWORD", "pass")

	applyEnvOverrides(cfg)
	if cfg.Docker.BuildRegistryUsername != "user" {
		t.Errorf("username = %q", cfg.Docker.BuildRegistryUsername)
	}
	if cfg.Docker.BuildRegistryPassword != "pass" {
		t.Errorf("password = %q", cfg.Docker.BuildRegistryPassword)
	}
}

// =============================================================================
// Validate — additional config validation paths (config.go:294)
// =============================================================================

func TestConfigValidate_InvalidCIDR(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.AllowedCIDRs = []string{"not-a-cidr"}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "CIDR") {
		t.Errorf("expected CIDR error, got: %v", err)
	}
}

func TestConfigValidate_InvalidSSlMode(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Database.SSLMode = "invalid"

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ssl_mode") {
		t.Errorf("expected SSL mode error, got: %v", err)
	}
}

func TestConfigValidate_PortConflictWithHTTP(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.Port = cfg.Ingress.HTTPPort // Set API port to same as HTTP port

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("expected port conflict error, got: %v", err)
	}
}

func TestConfigValidate_ACMEEmailInvalid(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.ACME.Email = "not-an-email"

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "email") {
		t.Errorf("expected email error, got: %v", err)
	}
}

func TestConfigValidate_DockerRegistryBothOrNeither(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Docker.BuildRegistryPassword = "pass-only"

	// One set without the other should error
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Errorf("expected username/password error, got: %v", err)
	}
}

func TestConfigValidate_BackupEncryptionKeyInvalid(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Backup.EncryptionKey = "not-base64!!"

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "base64") {
		t.Errorf("expected base64 error, got: %v", err)
	}
}

func TestConfigValidate_LokiURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Observability.LokiURL = "localhost:3100" // Missing http:// prefix

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "loki_url") {
		t.Errorf("expected loki URL error, got: %v", err)
	}
}

func TestConfigValidate_TracingURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Observability.TracingURL = "localhost:4318" // Missing prefix

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "tracing_url") {
		t.Errorf("expected tracing URL error, got: %v", err)
	}
}

func TestConfigValidate_DockerBuildImageRegistryWithScheme(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Docker.BuildImageRegistry = "https://registry.example.com"

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Errorf("expected scheme error, got: %v", err)
	}
}

func TestConfigValidate_ObservabilityServiceNameInvalid(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Observability.ServiceName = strings.Repeat("x", 257) // > 256

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "service_name") {
		t.Errorf("expected service_name error, got: %v", err)
	}
}

// =============================================================================
// HasPermission — empty PermissionsJSON (store.go:342)
// =============================================================================

func TestRole_HasPermission_EmptyJSON(t *testing.T) {
	r := &Role{PermissionsJSON: ""}
	if r.HasPermission("anything") {
		t.Error("expected false for empty PermissionsJSON")
	}
}
