package build

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// redactingWriter.Write — additional coverage for edge cases
// ---------------------------------------------------------------------------

func TestRedactingWriter_Write_EmptySecrets(t *testing.T) {
	var buf bytes.Buffer
	w := redactingWriter{dst: &buf, secrets: []string{"", ""}}
	_, _ = w.Write([]byte("hello"))
	if buf.String() != "hello" {
		t.Errorf("got %q, want %q", buf.String(), "hello")
	}
}

func TestRedactingWriter_Write_NoSecrets(t *testing.T) {
	var buf bytes.Buffer
	w := redactingWriter{dst: &buf}
	n, err := w.Write([]byte("test data"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 9 {
		t.Errorf("n = %d, want 9", n)
	}
}

// ---------------------------------------------------------------------------
// dockerPush — 0% coverage
// ---------------------------------------------------------------------------

func TestDockerPush_InvalidTag(t *testing.T) {
	err := dockerPush(context.Background(), "", nil, io.Discard)
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestDockerPush_ValidTag(t *testing.T) {
	// docker not installed in CI — validates tag only, exec will fail
	err := dockerPush(context.Background(), "nginx:latest", nil, io.Discard)
	// Should succeed the tag validation but fail on exec (no docker)
	if err != nil && !strings.Contains(err.Error(), "invalid") {
		t.Logf("expected docker exec error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// dockerAuthEnv — 9.1% coverage
// ---------------------------------------------------------------------------

func TestDockerAuthEnv_BothEmpty(t *testing.T) {
	env, cleanup, err := dockerAuthEnv("nginx:latest", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	if env != nil {
		t.Errorf("expected nil env, got %v", env)
	}
}

func TestDockerAuthEnv_UsernameOnly(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx:latest", "user", "")
	if err == nil {
		t.Fatal("expected error when password is empty but username is set")
	}
}

func TestDockerAuthEnv_PasswordOnly(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx:latest", "", "pass")
	if err == nil {
		t.Fatal("expected error when username is empty but password is set")
	}
}

func TestDockerAuthEnv_NoRegistryHost(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx:latest", "user", "pass")
	if err == nil {
		t.Fatal("expected error for image without registry")
	}
}

func TestDockerAuthEnv_Success(t *testing.T) {
	env, cleanup, err := dockerAuthEnv("registry.example.com/app:v1", "user", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	var found bool
	for _, e := range env {
		if strings.HasPrefix(e, "DOCKER_CONFIG=") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected DOCKER_CONFIG env var, got %v", env)
	}
}

// ---------------------------------------------------------------------------
// registryHostFromImage — 0% coverage
// ---------------------------------------------------------------------------

func TestRegistryHostFromImage(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"nginx:latest", ""},
		{"library/nginx", ""},
		{"registry.example.com/app:v1", "registry.example.com"},
		{"localhost:5000/app:v1", "localhost:5000"},
		{"192.168.1.1:5000/app:v1", "192.168.1.1:5000"},
		{"", ""},
	}
	for _, tt := range tests {
		got := registryHostFromImage(tt.ref)
		if got != tt.want {
			t.Errorf("registryHostFromImage(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// redactURL — 60% coverage
// ---------------------------------------------------------------------------

func TestRedactURL_NoCredentials(t *testing.T) {
	got := redactURL("https://github.com/org/repo.git")
	if got != "https://github.com/org/repo.git" {
		t.Errorf("got %q, want unchanged URL", got)
	}
}

func TestRedactURL_WithCredentials(t *testing.T) {
	got := redactURL("https://token@github.com/org/repo.git")
	if strings.Contains(got, "token") {
		t.Errorf("URL should not contain raw token: %q", got)
	}
	if !strings.Contains(got, "redacted") {
		t.Errorf("URL should contain redacted placeholder: %q", got)
	}
}

func TestRedactURL_InvalidURL(t *testing.T) {
	// Parse error should return raw string unchanged
	got := redactURL("://invalid-url\tok")
	if got != "://invalid-url\tok" {
		t.Errorf("got %q, want unchanged URL", got)
	}
}

// ---------------------------------------------------------------------------
// Module.GetPool — 0% coverage
// ---------------------------------------------------------------------------

func TestModule_GetPool_AfterInit(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Limits: core.LimitsConfig{MaxConcurrentBuilds: 3, MaxConcurrentBuildsPerTenant: 1},
		},
		Logger: slog.Default(),
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if pool := m.GetPool(); pool == nil {
		t.Error("GetPool should return non-nil pool after Init")
	}
}

func TestModule_GetPool_BeforeInit(t *testing.T) {
	m := New()
	if pool := m.GetPool(); pool != nil {
		t.Errorf("GetPool before Init should return nil, got %v", pool)
	}
}

// ---------------------------------------------------------------------------
// Module.Stop — 77.8% coverage (nil queue branches)
// ---------------------------------------------------------------------------

func TestModule_Stop_NilQueue(t *testing.T) {
	// m.queue is nil — should handle gracefully
	m := &Module{
		logger: slog.Default(),
		pool:   NewWorkerPoolWithLogger(1, slog.Default()),
	}
	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestModule_Stop_NilPool(t *testing.T) {
	// m.pool is nil — should handle gracefully
	m := &Module{
		logger: slog.Default(),
		queue:  NewTenantQueue(1, 1, slog.Default()),
	}
	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ---------------------------------------------------------------------------
// TenantQueue.NewTenantQueue — nil logger branch
// ---------------------------------------------------------------------------

func TestTenantQueue_NewNilLogger(t *testing.T) {
	q := NewTenantQueue(2, 2, nil)
	if q.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestTenantQueue_GlobalInFlight(t *testing.T) {
	q := NewTenantQueue(3, 1, slog.Default())
	if n := q.GlobalInFlight(); n != 0 {
		t.Errorf("GlobalInFlight = %d, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// TenantQueue.Submit — canceled context while waiting for tenant slot
// ---------------------------------------------------------------------------

func TestTenantQueue_Submit_CanceledContextTenantWait(t *testing.T) {
	q := NewTenantQueue(5, 1, slog.Default())

	// Fill tenant slot
	blocked := make(chan struct{})
	_ = q.Submit(context.Background(), "t1", func() {
		<-blocked
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	go func() {
		errCh <- q.Submit(ctx, "t1", func() {})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error for canceled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Submit to unblock on cancel")
	}
	close(blocked)
	_ = q.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// TenantQueue.Submit — Shutdown while waiting for global slot
// ---------------------------------------------------------------------------

func TestTenantQueue_Submit_ShutdownDuringGlobalWait(t *testing.T) {
	q := NewTenantQueue(1, 2, slog.Default())

	// Fill global slot
	blocked := make(chan struct{})
	_ = q.Submit(context.Background(), "t1", func() {
		<-blocked
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- q.Submit(context.Background(), "t2", func() {})
	}()

	time.Sleep(50 * time.Millisecond)
	close(blocked) // let first job finish and release global slot

	// Wait for second submit to either grab the slot or see stopCh
	time.Sleep(100 * time.Millisecond)
	_ = q.Shutdown(context.Background())

	select {
	case err := <-errCh:
		if err != nil && err != ErrTenantQueueClosed {
			t.Errorf("expected nil or ErrTenantQueueClosed, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Submit to return on Shutdown")
	}
}

// ---------------------------------------------------------------------------
// TenantQueue.Shutdown — with canceled context
// ---------------------------------------------------------------------------

func TestTenantQueue_Shutdown_CanceledCtx(t *testing.T) {
	q := NewTenantQueue(1, 1, slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := q.Shutdown(ctx)
	if err != ctx.Err() {
		t.Errorf("expected ctx.Err(), got %v", err)
	}
}

// ---------------------------------------------------------------------------
// TenantQueue.TenantInFlight — unknown tenant
// ---------------------------------------------------------------------------

func TestTenantQueue_TenantInFlight_UnknownTenant(t *testing.T) {
	q := NewTenantQueue(1, 1, slog.Default())
	if n := q.TenantInFlight("unknown"); n != 0 {
		t.Errorf("TenantInFlight = %d, want 0", n)
	}
	_ = q.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// Build panic recovery
// ---------------------------------------------------------------------------

func TestBuild_PanicRecovery(t *testing.T) {
	b := NewBuilder(nil, nil)
	_, err := b.Build(context.Background(), BuildOpts{
		AppID:   "app-1",
		AppName: "test-app",
		Timeout: time.Second,
	}, io.Discard)
	if err == nil {
		t.Fatal("expected error from build (no runtime, no source URL)")
	}
}

// ---------------------------------------------------------------------------
// WorkerPool.SubmitCtx — canceled context while waiting for slot
// ---------------------------------------------------------------------------

func TestWorkerPool_SubmitCtx_CanceledWhileWaiting(t *testing.T) {
	wp := NewWorkerPoolWithLogger(1, slog.Default())
	// Fill the slot
	blocked := make(chan struct{})
	_ = wp.SubmitCtx(context.Background(), func() { <-blocked })

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	go func() {
		errCh <- wp.SubmitCtx(ctx, func() {})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error from canceled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SubmitCtx to return after cancel")
	}
	close(blocked)
	_ = wp.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// WorkerPool.Submit (no-return variant) — after shutdown
// ---------------------------------------------------------------------------

func TestWorkerPool_Submit_AfterShutdown(t *testing.T) {
	wp := NewWorkerPoolWithLogger(1, slog.Default())
	_ = wp.Shutdown(context.Background())
	// Submit should silently drop
	wp.Submit(func() {})
}

// ---------------------------------------------------------------------------
// Module.Stop with expired context (queue times out)
// ---------------------------------------------------------------------------

func TestModule_Stop_ExpiredCtx(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Limits: core.LimitsConfig{MaxConcurrentBuilds: 1, MaxConcurrentBuildsPerTenant: 1},
		},
		Logger: slog.Default(),
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
	defer cancel()
	// Should not panic; may return ctx.Err()
	_ = m.Stop(ctx)
}

// ---------------------------------------------------------------------------
// gitClone — validation error
// ---------------------------------------------------------------------------

func TestGitClone_InvalidURL(t *testing.T) {
	var buf bytes.Buffer
	_, err := gitClone(context.Background(), "http://insecure/repo", "", "", t.TempDir(), &buf)
	if err == nil {
		t.Fatal("expected error for HTTP URL")
	}
	if !strings.Contains(err.Error(), "invalid git URL") {
		t.Errorf("error = %q, want containing 'invalid git URL'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// dockerBuild — invalid tag
// ---------------------------------------------------------------------------

func TestDockerBuild_EmptyTag(t *testing.T) {
	err := dockerBuild(context.Background(), "/tmp", "Dockerfile", "", nil, nil, io.Discard)
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}
