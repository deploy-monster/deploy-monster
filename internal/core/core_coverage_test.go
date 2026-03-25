package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Config edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestLoadConfig_SecretKeyAutoGeneration(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Server.SecretKey == "" {
		t.Error("secret key should be auto-generated when not set")
	}

	// Load again - should get a different secret
	cfg2, _ := LoadConfig()
	if cfg.Server.SecretKey == cfg2.Server.SecretKey {
		t.Error("two LoadConfig calls should produce different auto-generated secrets")
	}
}

func TestLoadConfig_WithSecretEnv(t *testing.T) {
	t.Setenv("MONSTER_SECRET", "my-fixed-secret-key")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Server.SecretKey != "my-fixed-secret-key" {
		t.Errorf("expected fixed secret from env, got %q", cfg.Server.SecretKey)
	}
}

func TestApplyDefaults_AllFields(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	if cfg.ACME.Provider != "http-01" {
		t.Errorf("ACME.Provider = %q, want http-01", cfg.ACME.Provider)
	}
	if cfg.Docker.Host != "unix:///var/run/docker.sock" {
		t.Errorf("Docker.Host = %q, want unix:///var/run/docker.sock", cfg.Docker.Host)
	}
	if cfg.Marketplace.TemplatesDir != "marketplace/templates" {
		t.Errorf("Marketplace.TemplatesDir = %q", cfg.Marketplace.TemplatesDir)
	}
	if cfg.Limits.MaxAppsPerTenant != 100 {
		t.Errorf("MaxAppsPerTenant = %d, want 100", cfg.Limits.MaxAppsPerTenant)
	}
	if cfg.Limits.MaxBuildMinutes != 30 {
		t.Errorf("MaxBuildMinutes = %d, want 30", cfg.Limits.MaxBuildMinutes)
	}
}

func TestApplyEnvOverrides_LogLevel(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	t.Setenv("MONSTER_LOG_LEVEL", "debug")
	applyEnvOverrides(cfg)

	// MONSTER_LOG_LEVEL is handled by logger setup, not config
	// Just verify no panic
}

// ═══════════════════════════════════════════════════════════════════════════════
// ValidateConfig — more edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestValidateConfig_PortTooHigh(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.Port = 70000

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("port > 65535 should be invalid")
	}
}

func TestValidateConfig_HTTPPortInvalid(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Ingress.HTTPPort = -1

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("negative HTTP port should be invalid")
	}
}

func TestValidateConfig_HTTPSPortInvalid(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Ingress.HTTPSPort = 0

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("zero HTTPS port should be invalid")
	}
}

func TestValidateConfig_APIPortConflictsHTTPS(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Server.Port = 443

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("API port conflicting with HTTPS port should be invalid")
	}
}

func TestValidateConfig_PostgresWithURL(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Database.Driver = "postgres"
	cfg.Database.URL = "postgres://localhost:5432/dm"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("postgres with URL should be valid: %v", err)
	}
}

func TestValidateConfig_NegativeConcurrentBuilds(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Limits.MaxConcurrentBuilds = -1

	err := ValidateConfig(cfg)
	if err == nil {
		t.Error("negative concurrent builds should be invalid")
	}
}

func TestValidateConfig_AllValidRegistrationModes(t *testing.T) {
	modes := []string{"open", "invite_only", "approval", "disabled", "sso_only"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			cfg := &Config{}
			applyDefaults(cfg)
			cfg.Registration.Mode = mode
			err := ValidateConfig(cfg)
			if err != nil {
				t.Errorf("registration mode %q should be valid: %v", mode, err)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EventBus — concurrent operations
// ═══════════════════════════════════════════════════════════════════════════════

func TestEventBus_ConcurrentPublish(t *testing.T) {
	eb := NewEventBus(slog.Default())
	var count int64
	var mu sync.Mutex

	eb.Subscribe("concurrent", func(_ context.Context, _ Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			eb.Publish(context.Background(), Event{Type: "concurrent"})
		}()
	}
	wg.Wait()

	mu.Lock()
	if count != 100 {
		t.Errorf("expected 100 handler calls, got %d", count)
	}
	mu.Unlock()
}

func TestEventBus_ConcurrentSubscribeAndPublish(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var wg sync.WaitGroup

	// Subscribe concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			eb.Subscribe(fmt.Sprintf("test.%d", n), func(_ context.Context, _ Event) error {
				return nil
			})
		}(i)
	}

	// Publish concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			eb.Publish(context.Background(), Event{Type: fmt.Sprintf("test.%d", n)})
		}(i)
	}

	wg.Wait()

	stats := eb.Stats()
	if stats.SubscriptionCount != 10 {
		t.Errorf("expected 10 subscriptions, got %d", stats.SubscriptionCount)
	}
}

func TestEventBus_NilLogger(t *testing.T) {
	eb := NewEventBus(nil)
	if eb == nil {
		t.Fatal("NewEventBus(nil) should not return nil")
	}

	// Should work without panic
	eb.Subscribe("test", func(_ context.Context, _ Event) error { return nil })
	eb.Publish(context.Background(), Event{Type: "test"})
}

func TestEventBus_Publish_SyncHandlerError_StopsChain(t *testing.T) {
	eb := NewEventBus(slog.Default())

	eb.Subscribe("fail", func(_ context.Context, _ Event) error {
		return errors.New("first handler fails")
	})

	secondCalled := false
	eb.Subscribe("fail", func(_ context.Context, _ Event) error {
		secondCalled = true
		return nil
	})

	err := eb.Publish(context.Background(), Event{Type: "fail"})
	if err == nil {
		t.Error("expected error from first sync handler")
	}
	if secondCalled {
		t.Error("second handler should not be called when first fails")
	}
}

func TestEventBus_MatchSubscriptions_PrefixEdgeCases(t *testing.T) {
	eb := NewEventBus(slog.Default())

	var matched []string
	eb.Subscribe("app.*", func(_ context.Context, e Event) error {
		matched = append(matched, e.Type)
		return nil
	})

	// "app" alone should NOT match "app.*"
	eb.Publish(context.Background(), Event{Type: "app"})
	if len(matched) != 0 {
		t.Errorf("'app' should not match 'app.*', got %v", matched)
	}

	// "application.created" should NOT match "app.*"
	eb.Publish(context.Background(), Event{Type: "application.created"})
	if len(matched) != 0 {
		t.Errorf("'application.created' should not match 'app.*', got %v", matched)
	}

	// "app.created" should match
	eb.Publish(context.Background(), Event{Type: "app.created"})
	if len(matched) != 1 || matched[0] != "app.created" {
		t.Errorf("'app.created' should match 'app.*', got %v", matched)
	}
}

func TestCutSuffix(t *testing.T) {
	tests := []struct {
		s, suffix string
		wantPre   string
		wantOK    bool
	}{
		{"app.*", ".*", "app", true},
		{"test", ".*", "test", false},
		{".*", ".*", "", true},
		{"", ".*", "", false},
		{"a.b.c.*", ".*", "a.b.c", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.s, tt.suffix), func(t *testing.T) {
			pre, ok := cutSuffix(tt.s, tt.suffix)
			if pre != tt.wantPre || ok != tt.wantOK {
				t.Errorf("cutSuffix(%q, %q) = (%q, %v), want (%q, %v)",
					tt.s, tt.suffix, pre, ok, tt.wantPre, tt.wantOK)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Registry — dependency resolution edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestRegistry_InitAll_FailingModule(t *testing.T) {
	r := NewRegistry()

	failing := &failingModule{id: "failer", failOn: "init"}
	ok := newStub("ok")

	r.Register(ok)
	r.Register(failing)
	r.Resolve()

	err := r.InitAll(context.Background(), &Core{})
	if err == nil {
		t.Error("expected error from failing init")
	}
	if !strings.Contains(err.Error(), "failer") {
		t.Errorf("error should mention failing module: %v", err)
	}
}

func TestRegistry_StartAll_FailingModule(t *testing.T) {
	r := NewRegistry()

	failing := &failingModule{id: "failer", failOn: "start"}
	ok := newStub("ok")

	r.Register(ok)
	r.Register(failing)
	r.Resolve()

	// Init succeeds
	r.InitAll(context.Background(), &Core{})

	err := r.StartAll(context.Background())
	if err == nil {
		t.Error("expected error from failing start")
	}
	if !strings.Contains(err.Error(), "failer") {
		t.Errorf("error should mention failing module: %v", err)
	}
}

func TestRegistry_StopAll_FailingModule(t *testing.T) {
	r := NewRegistry()

	failing := &failingModule{id: "failer", failOn: "stop"}
	ok := newStub("ok")

	r.Register(ok)
	r.Register(failing)
	r.Resolve()

	r.InitAll(context.Background(), &Core{})
	r.StartAll(context.Background())

	err := r.StopAll(context.Background())
	if err == nil {
		t.Error("expected error from failing stop")
	}
}

func TestRegistry_StopAll_MultipleFailures(t *testing.T) {
	r := NewRegistry()

	f1 := &failingModule{id: "f1", failOn: "stop"}
	f2 := &failingModule{id: "f2", failOn: "stop"}

	r.Register(f1)
	r.Register(f2)
	r.Resolve()

	err := r.StopAll(context.Background())
	if err == nil {
		t.Error("expected error from multiple failing stops")
	}
	// Should return first error
}

func TestRegistry_Resolve_Diamond(t *testing.T) {
	r := NewRegistry()
	// Diamond dependency: d -> b, c; b -> a; c -> a
	r.Register(newStub("a"))
	r.Register(newStub("b", "a"))
	r.Register(newStub("c", "a"))
	r.Register(newStub("d", "b", "c"))

	err := r.Resolve()
	if err != nil {
		t.Fatalf("diamond dependency should resolve: %v", err)
	}

	order := r.All()
	indexOf := func(id string) int {
		for i, v := range order {
			if v == id {
				return i
			}
		}
		return -1
	}

	if indexOf("a") > indexOf("b") || indexOf("a") > indexOf("c") {
		t.Error("a should come before b and c")
	}
	if indexOf("b") > indexOf("d") || indexOf("c") > indexOf("d") {
		t.Error("b and c should come before d")
	}
}

func TestRegistry_Get_NonExistent(t *testing.T) {
	r := NewRegistry()
	if r.Get("nonexistent") != nil {
		t.Error("Get should return nil for non-existent module")
	}
}

func TestRegistry_All_Empty(t *testing.T) {
	r := NewRegistry()
	r.Resolve()
	all := r.All()
	if len(all) != 0 {
		t.Errorf("All should return empty slice for empty registry, got %d", len(all))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Logger
// ═══════════════════════════════════════════════════════════════════════════════

func TestSetupLogger_Levels(t *testing.T) {
	tests := []struct {
		level string
	}{
		{"debug"},
		{"info"},
		{"warn"},
		{"warning"},
		{"error"},
		{"unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			logger := SetupLogger(tt.level, "text")
			if logger == nil {
				t.Error("SetupLogger should not return nil")
			}
		})
	}
}

func TestSetupLogger_JSONFormat(t *testing.T) {
	logger := SetupLogger("info", "json")
	if logger == nil {
		t.Error("SetupLogger should not return nil for JSON format")
	}
}

func TestSetupLogger_TextFormat(t *testing.T) {
	logger := SetupLogger("info", "text")
	if logger == nil {
		t.Error("SetupLogger should not return nil for text format")
	}
}

func TestNewLogWriter(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	w := NewLogWriter(logger, slog.LevelInfo, "[build] ")
	if w == nil {
		t.Fatal("NewLogWriter should not return nil")
	}

	n, err := w.Write([]byte("hello world\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 12 {
		t.Errorf("Write returned %d, want 12", n)
	}
}

func TestLogWriter_EmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	w := NewLogWriter(logger, slog.LevelInfo, "[test] ")

	n, err := w.Write([]byte("   \n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 4 {
		t.Errorf("Write returned %d, want 4", n)
	}

	// Empty message should not produce log output
	// (the string after TrimSpace is empty)
}

func TestLogWriter_TrimsWhitespace(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	w := NewLogWriter(logger, slog.LevelInfo, "")

	w.Write([]byte("  hello  \n"))
	if !strings.Contains(buf.String(), "hello") {
		t.Error("log output should contain trimmed message")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Banner
// ═══════════════════════════════════════════════════════════════════════════════

func TestPrintBanner(t *testing.T) {
	// Capture stdout is complex; just verify no panic
	cfg := &Config{}
	applyDefaults(cfg)

	build := BuildInfo{
		Version: "1.0.0",
		Commit:  "abc123",
		Date:    "2024-01-01",
	}

	// Should not panic
	PrintBanner(build, cfg)
}

func TestPrintBanner_NoHTTPS(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	cfg.Ingress.EnableHTTPS = false

	build := BuildInfo{Version: "1.0.0", Commit: "abc"}
	PrintBanner(build, cfg)
}

// ═══════════════════════════════════════════════════════════════════════════════
// ID generation edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestGenerateSecret_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		s := GenerateSecret(32)
		if seen[s] {
			t.Fatalf("duplicate secret generated: %s", s)
		}
		seen[s] = true
	}
}

func TestGeneratePassword_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		p := GeneratePassword(20)
		if seen[p] {
			t.Fatalf("duplicate password generated: %s", p)
		}
		seen[p] = true
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestScheduler_Tick_DisabledJob(t *testing.T) {
	s := NewScheduler(slog.Default())

	called := false
	s.Add(&CronJob{
		ID:       "disabled-job",
		Name:     "disabled",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			called = true
			return nil
		},
	})

	// Disable the job
	s.mu.Lock()
	s.jobs["disabled-job"].Enabled = false
	s.jobs["disabled-job"].NextRun = time.Now().Add(-time.Second)
	s.mu.Unlock()

	s.tick()
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("disabled job should not be executed")
	}
}

func TestScheduler_Tick_RunningJob(t *testing.T) {
	s := NewScheduler(slog.Default())

	callCount := 0
	s.Add(&CronJob{
		ID:       "running-job",
		Name:     "running",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			callCount++
			return nil
		},
	})

	// Mark as already running
	s.mu.Lock()
	s.jobs["running-job"].Running = true
	s.jobs["running-job"].NextRun = time.Now().Add(-time.Second)
	s.mu.Unlock()

	s.tick()
	time.Sleep(50 * time.Millisecond)

	if callCount > 0 {
		t.Error("running job should not be executed again")
	}
}

func TestScheduler_Tick_FutureJob(t *testing.T) {
	s := NewScheduler(slog.Default())

	called := false
	s.Add(&CronJob{
		ID:       "future-job",
		Name:     "future",
		Schedule: "@every 1h",
		Handler: func(_ context.Context) error {
			called = true
			return nil
		},
	})

	// NextRun is already in the future from Add()
	s.tick()
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Error("future job should not be executed before NextRun")
	}
}

func TestScheduler_Tick_ErrorHandler(t *testing.T) {
	s := NewScheduler(slog.Default())

	s.Add(&CronJob{
		ID:       "error-job",
		Name:     "errorer",
		Schedule: "@every 1s",
		Handler: func(_ context.Context) error {
			return errors.New("job failed")
		},
	})

	s.mu.Lock()
	s.jobs["error-job"].NextRun = time.Now().Add(-time.Second)
	s.mu.Unlock()

	s.tick()
	time.Sleep(100 * time.Millisecond)

	// Should not panic; error is logged
}

func TestScheduler_CalcNextRun_InvalidInterval(t *testing.T) {
	s := NewScheduler(slog.Default())

	next := s.calcNextRun("@every invalid")

	// Should fall through to HH:MM parsing or default
	now := time.Now()
	if next.Before(now.Add(-time.Second)) {
		t.Error("invalid schedule should still return future time")
	}
}

func TestScheduler_CalcNextRun_ShortSchedule(t *testing.T) {
	s := NewScheduler(slog.Default())

	next := s.calcNextRun("ab")

	// Short string falls through to default (1 hour from now)
	now := time.Now()
	diff := next.Sub(now)
	if diff < 59*time.Minute || diff > 61*time.Minute {
		t.Errorf("short schedule should default to ~1 hour, got %v", diff)
	}
}

func TestAtoi(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"0", 0},
		{"123", 123},
		{"02", 2},
		{"59", 59},
		{"", 0},
		{"abc", 0},
		{"1a2", 12},
	}

	for _, tt := range tests {
		got := atoi(tt.input)
		if got != tt.want {
			t.Errorf("atoi(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// HealthStatus
// ═══════════════════════════════════════════════════════════════════════════════

func TestHealthStatus_AllValues(t *testing.T) {
	if HealthOK.String() != "ok" {
		t.Errorf("HealthOK.String() = %q", HealthOK.String())
	}
	if HealthDegraded.String() != "degraded" {
		t.Errorf("HealthDegraded.String() = %q", HealthDegraded.String())
	}
	if HealthDown.String() != "down" {
		t.Errorf("HealthDown.String() = %q", HealthDown.String())
	}
	if HealthStatus(99).String() != "unknown" {
		t.Errorf("HealthStatus(99).String() = %q", HealthStatus(99).String())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// App — RegisterModule
// ═══════════════════════════════════════════════════════════════════════════════

func TestRegisterModule(t *testing.T) {
	// Save and restore the global factories
	original := moduleFactories
	defer func() { moduleFactories = original }()

	moduleFactories = nil

	RegisterModule(func() Module { return newStub("test-factory") })

	if len(moduleFactories) != 1 {
		t.Errorf("expected 1 factory, got %d", len(moduleFactories))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// AppError
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewAppError(t *testing.T) {
	err := NewAppError(404, "not found", nil)

	if err.Code != 404 {
		t.Errorf("Code = %d, want 404", err.Code)
	}
	if err.Message != "not found" {
		t.Errorf("Message = %q, want 'not found'", err.Message)
	}
	if err.Error() != "not found" {
		t.Errorf("Error() = %q, want 'not found'", err.Error())
	}
}

func TestNewAppError_WithCause(t *testing.T) {
	cause := errors.New("database connection failed")
	err := NewAppError(500, "internal error", cause)

	if err.Err != cause {
		t.Error("Err should be set")
	}

	// Error() should include both message and cause
	if !strings.Contains(err.Error(), "internal error") {
		t.Error("Error() should contain the message")
	}
	if !strings.Contains(err.Error(), "database connection failed") {
		t.Error("Error() should contain the cause")
	}

	// Unwrap should return the cause
	if errors.Unwrap(err) != cause {
		t.Error("Unwrap should return the cause")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Helpers
// ═══════════════════════════════════════════════════════════════════════════════

type failingModule struct {
	id     string
	failOn string // "init", "start", or "stop"
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
		return errors.New("init failed")
	}
	return nil
}

func (f *failingModule) Start(_ context.Context) error {
	if f.failOn == "start" {
		return errors.New("start failed")
	}
	return nil
}

func (f *failingModule) Stop(_ context.Context) error {
	if f.failOn == "stop" {
		return errors.New("stop failed")
	}
	return nil
}

// Verify the io.Writer interface is satisfied by LogWriter
var _ io.Writer = (*LogWriter)(nil)
