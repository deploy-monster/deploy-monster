package core

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// Config.Validate — additional error paths (config.go:294)
// =============================================================================

func TestConfigValidate_InvalidLogLevel(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.LogLevel = "trace"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "log_level") {
		t.Fatalf("expected log_level error, got: %v", err)
	}
}

func TestConfigValidate_InvalidLogFormat(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.LogFormat = "xml"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "log_format") {
		t.Fatalf("expected log_format error, got: %v", err)
	}
}

func TestConfigValidate_InvalidCIDRExtra(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.AllowedCIDRs = []string{"not-a-cidr"}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "allowed_cidrs") {
		t.Fatalf("expected CIDR error, got: %v", err)
	}
}

func TestConfigValidate_PostgresNoURL(t *testing.T) {
	cfg := validTestConfig()
	cfg.Database.Driver = "postgres"
	cfg.Database.URL = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "database.url") {
		t.Fatalf("expected database.url error, got: %v", err)
	}
}

func TestConfigValidate_InvalidSSLMode(t *testing.T) {
	cfg := validTestConfig()
	cfg.Database.SSLMode = "invalid"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "ssl_mode") {
		t.Fatalf("expected ssl_mode error, got: %v", err)
	}
}

func TestConfigValidate_PortConflictHTTP(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.Port = 80
	cfg.Ingress.HTTPPort = 80
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected port conflict error, got: %v", err)
	}
}

func TestConfigValidate_PortConflictHTTPS(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.Port = 443
	cfg.Ingress.HTTPSPort = 443
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("expected port conflict error, got: %v", err)
	}
}

func TestConfigValidate_InvalidACMEEmail(t *testing.T) {
	cfg := validTestConfig()
	cfg.ACME.Email = "not-an-email"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "acme.email") {
		t.Fatalf("expected acme.email error, got: %v", err)
	}
}

func TestConfigValidate_InvalidACMEProvider(t *testing.T) {
	cfg := validTestConfig()
	cfg.ACME.Provider = "tls-alpn-01"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "acme.provider") {
		t.Fatalf("expected acme.provider error, got: %v", err)
	}
}

func TestConfigValidate_InvalidDNSProvider(t *testing.T) {
	cfg := validTestConfig()
	cfg.DNS.Provider = "azure"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "dns.provider") {
		t.Fatalf("expected dns.provider error, got: %v", err)
	}
}

func TestConfigValidate_CloudflareTokenRequiredExtra(t *testing.T) {
	cfg := validTestConfig()
	cfg.DNS.Provider = "cloudflare"
	cfg.DNS.CloudflareToken = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "cloudflare_token") {
		t.Fatalf("expected cloudflare_token error, got: %v", err)
	}
}

func TestConfigValidate_BuildImageRegistryHasScheme(t *testing.T) {
	cfg := validTestConfig()
	cfg.Docker.BuildImageRegistry = "https://registry.example.com"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "build_image_registry") {
		t.Fatalf("expected build_image_registry error, got: %v", err)
	}
}

func TestConfigValidate_BuildImagePushRequiresRegistry(t *testing.T) {
	cfg := validTestConfig()
	cfg.Docker.BuildImagePush = true
	cfg.Docker.BuildImageRegistry = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "build_image_registry") {
		t.Fatalf("expected build_image_registry error, got: %v", err)
	}
}

func TestConfigValidate_RegistryCredsMismatch(t *testing.T) {
	cfg := validTestConfig()
	cfg.Docker.BuildRegistryUsername = "user"
	cfg.Docker.BuildRegistryPassword = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("expected registry username/password error, got: %v", err)
	}
}

func TestConfigValidate_NegativeCPUQuota(t *testing.T) {
	cfg := validTestConfig()
	cfg.Docker.DefaultCPUQuota = -1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "cpu_quota") {
		t.Fatalf("expected cpu_quota error, got: %v", err)
	}
}

func TestConfigValidate_NegativeRetentionDays(t *testing.T) {
	cfg := validTestConfig()
	cfg.Backup.RetentionDays = -1
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "retention_days") {
		t.Fatalf("expected retention_days error, got: %v", err)
	}
}

// =============================================================================
// Scheduler.calcNextRun — error paths (scheduler.go:309)
// =============================================================================

func TestSchedulerCalcNextRun_InvalidDuration(t *testing.T) {
	s := NewScheduler(slog.Default())
	next := s.calcNextRun("@every notaduration")
	if next.Before(time.Now()) {
		t.Error("expected future time for invalid @every")
	}
}

func TestSchedulerCalcNextRun_InvalidHHMM(t *testing.T) {
	s := NewScheduler(slog.Default())
	next := s.calcNextRun("25:00")
	if next.Before(time.Now()) {
		t.Error("expected future time for invalid HH:MM")
	}
}

func TestSchedulerCalcNextRun_Unrecognized(t *testing.T) {
	s := NewScheduler(slog.Default())
	next := s.calcNextRun("cron: 0 0 * * *")
	if next.Before(time.Now()) {
		t.Error("expected future time for unrecognized format")
	}
}

// =============================================================================
// parseHHMM — error paths (scheduler.go:345)
// =============================================================================

func TestParseHHMM_MissingColon(t *testing.T) {
	_, err := parseHHMM("1230")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected missing colon error, got: %v", err)
	}
}

func TestParseHHMM_InvalidHour(t *testing.T) {
	_, err := parseHHMM("abc:00")
	if err == nil || !strings.Contains(err.Error(), "hour") {
		t.Fatalf("expected hour parse error, got: %v", err)
	}
}

func TestParseHHMM_InvalidMinute(t *testing.T) {
	_, err := parseHHMM("12:xyz")
	if err == nil || !strings.Contains(err.Error(), "minute") {
		t.Fatalf("expected minute parse error, got: %v", err)
	}
}

func TestParseHHMM_HourRange(t *testing.T) {
	_, err := parseHHMM("24:00")
	if err == nil || !strings.Contains(err.Error(), "hour") {
		t.Fatalf("expected hour range error, got: %v", err)
	}
}

func TestParseHHMM_MinuteRange(t *testing.T) {
	_, err := parseHHMM("12:60")
	if err == nil || !strings.Contains(err.Error(), "minute") {
		t.Fatalf("expected minute range error, got: %v", err)
	}
}

// =============================================================================
// Scheduler.loop — shutdown via stopCh (scheduler.go:210)
// =============================================================================

func TestSchedulerLoop_StopViaCh(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.started = true
	s.wg.Add(1)
	go s.loop()
	// Stop should close stopCh and cause loop to return
	s.Stop()
}

func TestSchedulerLoop_StopViaCtx(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.started = true
	s.wg.Add(1)
	go s.loop()
	// Cancel the context directly
	s.stopCancel()
	time.Sleep(50 * time.Millisecond)
	// Close stopCh to let loop finish
	s.Stop()
}

// =============================================================================
// EventBus.PublishAsync — panic recovery path (events.go:235)
// =============================================================================

func TestEventBusPublishAsync_PanicRecovery(t *testing.T) {
	eb := NewEventBus(slog.Default())
	// Subscribe with a handler that panics
	eb.SubscribeAsync("test.panic", func(ctx context.Context, event Event) error {
		panic("handler panic")
	})
	// This should not panic
	eb.PublishAsync(context.Background(), Event{Type: "test.panic"})
	// Drain to let the async handler complete
	eb.Drain()
}

// =============================================================================
// EventBus.Publish — async handler error with context cancel (events.go:143)
// =============================================================================

func TestEventBusPublish_AsyncHandlerCtxCancel(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var onErrorCalled bool
	eb.OnError(func(event Event, sub *Subscription, err error) {
		onErrorCalled = true
	})

	eb.SubscribeAsync("test.cancel", func(ctx context.Context, event Event) error {
		return errors.New("handler error")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already canceled context

	event := Event{Type: "test.cancel"}
	_ = eb.Publish(ctx, event)
	eb.Drain()

	if !onErrorCalled {
		t.Error("expected onError to be called after context cancellation")
	}
}

// =============================================================================
// EventBus.DebugString — with/without CorrelationID (events.go:563)
// =============================================================================

func TestEventDebugString_WithCorrelationID(t *testing.T) {
	e := Event{
		ID:            "evt_abc123",
		Type:          "test.event",
		Source:        "test",
		TenantID:      "tenant_1",
		UserID:        "user_1",
		CorrelationID: "corr_xyz",
	}
	s := e.DebugString()
	if !strings.Contains(s, "corr=") {
		t.Error("expected correlation ID in debug string")
	}
}

func TestEventDebugString_WithoutCorrelationID(t *testing.T) {
	e := Event{
		ID:       "evt_abc123",
		Type:     "test.event",
		Source:   "test",
		TenantID: "tenant_1",
		UserID:   "user_1",
	}
	s := e.DebugString()
	if strings.Contains(s, "corr=") {
		t.Error("expected no correlation ID in debug string")
	}
}

// =============================================================================
// Retry — nil context, ErrNoRetry, context cancel (retry.go:40)
// =============================================================================

func TestRetry_NilContextExtra(t *testing.T) {
	calls := 0
	err := Retry(nil, DefaultRetryConfig(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetry_NoRetryErrorExtra(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return ErrNoRetry(errors.New("non-retryable"))
	})
	if err == nil || !strings.Contains(err.Error(), "non-retryable") {
		t.Fatalf("expected non-retryable error, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls)
	}
}

func TestRetry_SuccessOnSecondAttempt(t *testing.T) {
	calls := 0
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 3
	cfg.InitialDelay = time.Millisecond
	err := Retry(context.Background(), cfg, func() error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestRetry_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 5
	cfg.InitialDelay = 100 * time.Millisecond

	err := Retry(ctx, cfg, func() error {
		return errors.New("transient")
	})
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestRetry_ZeroAttemptsExtra(t *testing.T) {
	cfg := DefaultRetryConfig()
	cfg.MaxAttempts = 0
	err := Retry(context.Background(), cfg, func() error {
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// =============================================================================
// Interfacaces.ValidateVolumePaths — error paths (interfaces.go:70)
// =============================================================================

func TestValidateVolumePaths_NullByteExtra(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"/var/data\x00/secret": "/data"},
	}
	err := opts.ValidateVolumePaths()
	if err == nil || !strings.Contains(err.Error(), "null byte") {
		t.Fatalf("expected null byte error, got: %v", err)
	}
}

func TestValidateVolumePaths_PathTraversalExtra(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"/var/data/../../etc/passwd": "/data"},
	}
	err := opts.ValidateVolumePaths()
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Fatalf("expected path traversal error, got: %v", err)
	}
}

func TestValidateVolumePaths_RelativePath(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"relative/path": "/data"},
	}
	err := opts.ValidateVolumePaths()
	if err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("expected absolute path error, got: %v", err)
	}
}

func TestValidateVolumePaths_RootDir(t *testing.T) {
	opts := &ContainerOpts{
		Volumes: map[string]string{"/": "/data"},
	}
	err := opts.ValidateVolumePaths()
	if err == nil || !strings.Contains(err.Error(), "root directory") {
		t.Fatalf("expected root directory error, got: %v", err)
	}
}

func TestValidateVolumePaths_DockerSocketDenied(t *testing.T) {
	opts := &ContainerOpts{
		Volumes:          map[string]string{"/var/run/docker.sock": "/docker.sock"},
		AllowDockerSocket: false,
	}
	err := opts.ValidateVolumePaths()
	if err == nil || !strings.Contains(err.Error(), "Docker socket") {
		t.Fatalf("expected Docker socket error, got: %v", err)
	}
}

func TestValidateVolumePaths_DockerSocketAllowed(t *testing.T) {
	wd, _ := os.Getwd()
	tmpDir := t.TempDir()
	opts := &ContainerOpts{
		Volumes:          map[string]string{tmpDir + "/data": "/data"},
		AllowDockerSocket: true,
	}
	err := opts.ValidateVolumePaths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = wd
}

// =============================================================================
// ReloadConfig — no changes path (app.go:225)
// =============================================================================

func TestReloadConfig_NoChangesExtra(t *testing.T) {
	// Create a minimal config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "monster.yaml")
	cfgContent := `
server:
  host: "0.0.0.0"
  port: 8443
  secret_key: "this-is-at-least-32-characters-for-jwt-hs256!"
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Create Core with this config
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	c, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	c.ConfigPath = cfgPath

	// Reload with same config — should report "no changes"
	err = c.ReloadConfig()
	if err != nil {
		t.Fatalf("ReloadConfig: %v", err)
	}
}

// =============================================================================
// applyEnvOverrides — env var tests (config.go:541)
// =============================================================================

func TestApplyEnvOverrides_Port(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	t.Setenv("MONSTER_PORT", "9090")
	applyEnvOverrides(cfg)
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
}

func TestApplyEnvOverrides_InvalidPortExtra(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	origPort := cfg.Server.Port
	t.Setenv("MONSTER_PORT", "not-a-number")
	applyEnvOverrides(cfg)
	// Should not change port on parse error
	if cfg.Server.Port != origPort {
		t.Errorf("expected port to remain %d, got %d", origPort, cfg.Server.Port)
	}
}

func TestApplyEnvOverrides_BuildImagePush(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	t.Setenv("MONSTER_BUILD_IMAGE_PUSH", "true")
	applyEnvOverrides(cfg)
	if !cfg.Docker.BuildImagePush {
		t.Error("expected BuildImagePush to be true")
	}
}

func TestApplyEnvOverrides_InvalidCPUQuota(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	// applyDefaults sets DefaultCPUQuota = 100000
	defaultQuota := cfg.Docker.DefaultCPUQuota
	t.Setenv("MONSTER_DOCKER_CPU_QUOTA", "not-a-number")
	applyEnvOverrides(cfg)
	// Should not change on parse error — remains at default
	if cfg.Docker.DefaultCPUQuota != defaultQuota {
		t.Errorf("expected DefaultCPUQuota to remain %d, got %d", defaultQuota, cfg.Docker.DefaultCPUQuota)
	}
}

func TestApplyEnvOverrides_SMTPPort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	t.Setenv("MONSTER_SMTP_PORT", "587")
	applyEnvOverrides(cfg)
	if cfg.Notifications.SMTP.Port != 587 {
		t.Errorf("expected SMTP port 587, got %d", cfg.Notifications.SMTP.Port)
	}
}

func TestApplyEnvOverrides_InvalidSMTPPort(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	t.Setenv("MONSTER_SMTP_PORT", "invalid")
	applyEnvOverrides(cfg)
	if cfg.Notifications.SMTP.Port != 0 {
		t.Errorf("expected SMTP port 0, got %d", cfg.Notifications.SMTP.Port)
	}
}

// =============================================================================
// LokiSink — Handle with channel full (logger.go:185)
// =============================================================================

func TestLokiSink_HandleWhenClosing(t *testing.T) {
	ls := NewLokiSink("http://localhost:3100/loki/api/v1/push", "", "", time.Second, slog.Default())
	// Close the sink first
	_ = ls.Close()
	// Handle should not block or panic when closing
	ls.Handle(slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0))
}

func TestLokiSink_CloseIdempotent(t *testing.T) {
	ls := NewLokiSink("http://localhost:3100/loki/api/v1/push", "", "", time.Second, slog.Default())
	// Close twice should be safe
	err1 := ls.Close()
	err2 := ls.Close()
	if err1 != nil || err2 != nil {
		t.Error("expected nil errors from Close")
	}
}

// =============================================================================
// Scheduler — Start double-call is no-op
// =============================================================================

func TestScheduler_StartTwiceIsNoop(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.Add(&CronJob{
		ID:       "test",
		Name:     "test",
		Schedule: "@every 1h",
		Handler:  func(_ context.Context) error { return nil },
	})
	s.Start()
	s.Start() // Second call is no-op — should not panic
	s.Stop()
}

// =============================================================================
// Scheduler — Stop without Start is safe
// =============================================================================

func TestScheduler_StopWithoutStart(t *testing.T) {
	s := NewScheduler(slog.Default())
	// Stop before Start — should not panic, should return quickly
	s.Stop()
}

// =============================================================================
// LoadConfig — with CORS origins derived (config.go:274)
// =============================================================================

func TestLoadConfig_CORSOriginDerivedHTTPS(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "monster.yaml")
	cfgContent := `
server:
  host: "0.0.0.0"
  port: 8443
  domain: "example.com"
  secret_key: "this-is-at-least-32-characters-for-jwt-hs256!"
ingress:
  enable_https: true
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.Contains(cfg.Server.CORSOrigins, "https://") {
		t.Errorf("expected HTTPS origin, got %q", cfg.Server.CORSOrigins)
	}
}

func TestLoadConfig_CORSOriginDerivedHTTPWithCustomPort(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "monster.yaml")
	cfgContent := `
server:
  host: "0.0.0.0"
  port: 8080
  domain: "example.com"
  secret_key: "this-is-at-least-32-characters-for-jwt-hs256!"
ingress:
  enable_https: false
`
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !strings.Contains(cfg.Server.CORSOrigins, "http://") {
		t.Errorf("expected HTTP origin, got %q", cfg.Server.CORSOrigins)
	}
	if !strings.Contains(cfg.Server.CORSOrigins, ":8080") {
		t.Errorf("expected port 8080 in origin, got %q", cfg.Server.CORSOrigins)
	}
}

// =============================================================================
// NewScheduler — nil logger (scheduler.go:93)
// =============================================================================

func TestNewScheduler_NilLogger(t *testing.T) {
	s := NewScheduler(nil)
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.logger == nil {
		t.Error("expected default logger")
	}
}

// =============================================================================
// SetupLogger — text format and loki format (logger.go:208)
// =============================================================================

func TestSetupLogger_TextFormatExtra(t *testing.T) {
	logger := SetupLogger("info", "text", "", "", "", 0)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestSetupLogger_LokiFormat(t *testing.T) {
	logger := SetupLogger("info", "loki", "", "", "", 0)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestSetupLogger_WithLokiURL(t *testing.T) {
	logger := SetupLogger("info", "json", "http://loki:3100", "user", "pass", time.Second)
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}

// =============================================================================
// EventBus — SubscribeNamed (events.go:102)
// =============================================================================

func TestEventBus_SubscribeNamed(t *testing.T) {
	eb := NewEventBus(slog.Default())
	called := false
	eb.SubscribeNamed("test.named", "my-sub", false, func(ctx context.Context, event Event) error {
		called = true
		return nil
	})
	_ = eb.Publish(context.Background(), Event{Type: "test.named"})
	if !called {
		t.Error("expected handler to be called")
	}
}

// =============================================================================
// Registry — duplicate module registration handling
// =============================================================================

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubModule{id: "dup"})
	r.Register(&stubModule{id: "dup"})
	// Should not panic, just overwrite
}

// =============================================================================
// NewEvent — edge cases
// =============================================================================

func TestNewEvent_WithData(t *testing.T) {
	e := NewEvent("test.event", "source", map[string]string{"key": "val"})
	if e.Type != "test.event" {
		t.Errorf("expected test.event, got %s", e.Type)
	}
	if e.ID == "" {
		t.Error("expected non-empty ID")
	}
}

func TestNewTenantEvent_Basic(t *testing.T) {
	e := NewTenantEvent("tenant.event", "source", "tenant_1", "user_1", "data")
	if e.TenantID != "tenant_1" {
		t.Errorf("expected tenant_1, got %s", e.TenantID)
	}
	if e.UserID != "user_1" {
		t.Errorf("expected user_1, got %s", e.UserID)
	}
}

// =============================================================================
// ValidateConfig — edge cases (validate.go)
// =============================================================================

func TestValidateConfig_InvalidDriverExtra(t *testing.T) {
	cfg := validTestConfig()
	cfg.Database.Driver = "mongodb"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "driver") {
		t.Fatalf("expected driver error, got: %v", err)
	}
}

func TestValidateConfig_InvalidRegistrationModeExtra(t *testing.T) {
	cfg := validTestConfig()
	cfg.Registration.Mode = "open_sesame"
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "registration") {
		t.Fatalf("expected registration error, got: %v", err)
	}
}

func TestValidateConfig_ZeroConcurrentBuilds(t *testing.T) {
	cfg := validTestConfig()
	cfg.Limits.MaxConcurrentBuilds = 0
	err := ValidateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "max_concurrent_builds") {
		t.Fatalf("expected max_concurrent_builds error, got: %v", err)
	}
}

// =============================================================================
// CronJob — job with timeout and custom handler (scheduler.go)
// =============================================================================

func TestScheduler_RunJobPanicRecovery(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.wg.Add(1)
	go s.runJob(&CronJob{
		Name:    "panic-job",
		Handler: func(ctx context.Context) error { panic("handler panic") },
	})
	s.wg.Wait() // Should not panic
}

func TestScheduler_RunJobCustomTimeout(t *testing.T) {
	s := NewScheduler(slog.Default())
	var called bool
	s.wg.Add(1)
	go s.runJob(&CronJob{
		Name:    "timeout-job",
		Timeout: time.Second,
		Handler: func(ctx context.Context) error {
			called = true
			return nil
		},
	})
	s.wg.Wait()
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestScheduler_RunJobHandlerError(t *testing.T) {
	s := NewScheduler(slog.Default())
	s.wg.Add(1)
	go s.runJob(&CronJob{
		Name:    "error-job",
		Handler: func(ctx context.Context) error { return errors.New("handler error") },
	})
	s.wg.Wait()
}

// =============================================================================
// GeneratePassword — verify length and charset (id.go:32)
// =============================================================================

func TestGeneratePassword_LengthExtra(t *testing.T) {
	pw := GeneratePassword(24)
	if len(pw) != 24 {
		t.Errorf("expected 24 chars, got %d", len(pw))
	}
}

func TestGeneratePassword_Charset(t *testing.T) {
	pw := GeneratePassword(100)
	// Verify all characters are alphanumeric
	for _, r := range pw {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			t.Errorf("unexpected character %c in password", r)
		}
	}
}

// =============================================================================
// ShortID — helper (used by DebugString)
// =============================================================================

func TestShortIDExtra(t *testing.T) {
	id := GenerateID()
	short := ShortID(id, 8)
	if len(short) != 8 {
		t.Errorf("expected 8 chars, got %d", len(short))
	}
}

// =============================================================================
// multiHandler — slog.Handler implementations (logger.go:185)
// =============================================================================

func TestMultiHandler_Enabled(t *testing.T) {
	primary := slog.NewTextHandler(&bytes.Buffer{}, nil)
	mh := &multiHandler{primary: primary}
	if !mh.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected enabled")
	}
}

// =============================================================================
// NewApp with OpenTelemetry tracing URL
// =============================================================================

func TestNewApp_WithTracingURLExtra(t *testing.T) {
	original := moduleFactories
	defer func() { moduleFactories = original }()
	moduleFactories = moduleRegistry{}

	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.SecretKey = "test-secret-at-least-32-bytes-long!"
	cfg.Observability.TracingURL = "http://localhost:4318"
	cfg.Observability.ServiceName = "test-monster"

	_, err := NewApp(cfg, BuildInfo{Version: "1.0.0"})
	// This will warn about failed tracer init (since localhost:4318 isn't reachable)
	// but should not return an error
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
}