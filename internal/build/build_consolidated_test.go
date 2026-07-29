package build

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// === merged from build_edge_test.go ===

// dockerAuthEnv error paths (ensure unique names by prefixing with "Edge")

func TestEdge_DockerAuthEnv_NoCreds(t *testing.T) {
	env, cleanup, err := dockerAuthEnv("nginx:latest", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup()
	if env != nil {
		t.Errorf("expected nil env, got %v", env)
	}
}

func TestEdge_DockerAuthEnv_PartialCreds(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx:latest", "user", "")
	if err == nil {
		t.Fatal("expected error for partial credentials")
	}
}

func TestEdge_DockerAuthEnv_NoRegistryHost(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx", "user", "pass")
	if err == nil {
		t.Fatal("expected error for no registry host")
	}
}

// resolveDockerfilePath edge cases

func TestEdge_ResolveDockerfilePath_NullByte(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp", "Dockerfile\x00")
	if err == nil {
		t.Fatal("expected error for null byte")
	}
}

func TestEdge_ResolveDockerfilePath_AbsolutePath(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp", "/etc/Dockerfile")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestEdge_ResolveDockerfilePath_EscapesContext(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp/build", "../etc/Dockerfile")
	if err == nil {
		t.Fatal("expected error for path escaping")
	}
}

func TestEdge_ResolveDockerfilePath_NormalizedEscapes(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp/build", "sub/../../../etc/Dockerfile")
	if err == nil {
		t.Fatal("expected error for normalized escape")
	}
}

func TestEdge_ResolveDockerfilePath_ValidRelative(t *testing.T) {
	path, err := resolveDockerfilePath("/tmp/build", "sub/Dockerfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/tmp/build/sub/Dockerfile" {
		t.Errorf("expected /tmp/build/sub/Dockerfile, got %s", path)
	}
}

// validateBuildArg edge cases (unique names)

func TestEdge_ValidateBuildArg_ControlChars(t *testing.T) {
	err := validateBuildArg("MY_ARG", "value\x00null")
	if err == nil {
		t.Fatal("expected error for control chars")
	}
}

func TestEdge_ValidateBuildArg_DashPrefixValue(t *testing.T) {
	err := validateBuildArg("MY_ARG", "-flag")
	if err == nil {
		t.Fatal("expected error for dash-prefixed value")
	}
}

// validateDockerImageTag edge cases

func TestEdge_ValidateDockerImageTag_InvalidChars(t *testing.T) {
	err := validateDockerImageTag("image:tag$pecial")
	if err == nil {
		t.Fatal("expected error for invalid chars")
	}
}

func TestEdge_ValidateDockerImageTag_Valid(t *testing.T) {
	err := validateDockerImageTag("nginx:1.21")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// isPrivateOrBlockedIP edge cases

func TestEdge_IsPrivateOrBlockedIP_NonIP(t *testing.T) {
	if isPrivateOrBlockedIP("hostname") {
		t.Error("expected false for non-IP")
	}
}

func TestEdge_IsPrivateOrBlockedIP_Private10(t *testing.T) {
	if !isPrivateOrBlockedIP("10.0.0.1") {
		t.Error("expected true for 10.x.x.x")
	}
}

func TestEdge_IsPrivateOrBlockedIP_Loopback(t *testing.T) {
	if !isPrivateOrBlockedIP("127.0.0.1") {
		t.Error("expected true for loopback")
	}
}

func TestEdge_IsPrivateOrBlockedIP_Public(t *testing.T) {
	if isPrivateOrBlockedIP("8.8.8.8") {
		t.Error("expected false for public IP")
	}
}

// registryHostFromImage edge cases

func TestEdge_RegistryHostFromImage_NoSlash(t *testing.T) {
	if h := registryHostFromImage("nginx"); h != "" {
		t.Errorf("expected empty, got %s", h)
	}
}

func TestEdge_RegistryHostFromImage_LibraryName(t *testing.T) {
	if h := registryHostFromImage("library/nginx"); h != "" {
		t.Errorf("expected empty, got %s", h)
	}
}

func TestEdge_RegistryHostFromImage_WithRegistry(t *testing.T) {
	if h := registryHostFromImage("reg.example.com/app:v1"); h != "reg.example.com" {
		t.Errorf("expected reg.example.com, got %s", h)
	}
}

func TestEdge_RegistryHostFromImage_Localhost(t *testing.T) {
	if h := registryHostFromImage("localhost:5000/app:v1"); h != "localhost:5000" {
		t.Errorf("expected localhost:5000, got %s", h)
	}
}

// ValidateGitURL additions (unique names)

func TestEdge_ValidateGitURL_DashPrefix(t *testing.T) {
	err := ValidateGitURL("-arg")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEdge_ValidateGitURL_LocalPathDisabled(t *testing.T) {
	t.Setenv("MONSTER_ALLOW_LOCAL_GIT_PATHS", "")
	err := ValidateGitURL("/tmp/repo")
	if err == nil {
		t.Fatal("expected error for local path when disabled")
	}
}

func TestEdge_ValidateGitURL_LocalPathEnabled(t *testing.T) {
	t.Setenv("MONSTER_ALLOW_LOCAL_GIT_PATHS", "true")
	err := ValidateGitURL("/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEdge_ValidateGitURL_HTTPSValid(t *testing.T) {
	err := ValidateGitURL("https://github.com/org/repo.git")
	if err != nil {
		t.Fatalf("expected nil for valid HTTPS, got %v", err)
	}
}

func TestEdge_ValidateGitURL_DockerRef(t *testing.T) {
	err := ValidateGitURL("nginx:latest")
	if err != nil {
		t.Fatalf("expected nil for docker ref, got %v", err)
	}
}

// isAbsPath edge cases

func TestEdge_IsAbsPath_UnixAbsolute(t *testing.T) {
	if !isAbsPath("/tmp/repo") {
		t.Error("expected true for unix absolute path")
	}
}

func TestEdge_IsAbsPath_RelativePath(t *testing.T) {
	if isAbsPath("relative/path") {
		t.Error("expected false for relative path")
	}
}

// redactingWriter edge cases

func TestEdge_RedactingWriter_NoSecrets(t *testing.T) {
	var buf testBufferCapture
	w := redactingWriter{dst: &buf}
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
	if buf.String() != "hello" {
		t.Errorf("expected 'hello', got '%s'", buf.String())
	}
}

func TestEdge_RedactingWriter_WithSecrets(t *testing.T) {
	var buf testBufferCapture
	w := redactingWriter{dst: &buf, secrets: []string{"secret", ""}}
	n, err := w.Write([]byte("my secret is here"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 17 {
		t.Errorf("expected 17, got %d", n)
	}
	if buf.String() != "my [redacted] is here" {
		t.Errorf("expected redacted, got '%s'", buf.String())
	}
}

// validateResolvedHost edge cases

func TestEdge_ValidateResolvedHost_UnknownScheme(t *testing.T) {
	err := validateResolvedHost("unknown://host/path")
	if err != nil {
		t.Fatalf("expected nil for unknown scheme, got %v", err)
	}
}

// testBufferCapture implements io.Writer for testing
type testBufferCapture struct {
	buf []byte
}

func (b *testBufferCapture) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *testBufferCapture) String() string {
	return string(b.buf)
}

// === merged from builder_boost_test.go ===

func TestValidateDockerImageTag(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{"empty", "", true},
		{"simple", "nginx", false},
		{"with tag", "nginx:latest", false},
		{"registry path", "registry.example.com/app:v1", false},
		{"with digest", "nginx@sha256:abc123", false},
		{"complex", "my-registry.io:5000/user/app:v1.0.0-beta", true},
		{"invalid start char", ":latest", true},
		{"invalid slash start", "/nginx", true},
		{"invalid char space", "nginx latest", true},
		{"invalid char", "nginx|latest", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDockerImageTag(tc.tag)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateDockerImageTag(%q) error = %v, wantErr %v", tc.tag, err, tc.wantErr)
			}
		})
	}
}

func TestValidateBuildArg(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{"valid", "KEY", "value", false},
		{"valid underscore", "_KEY", "value", false},
		{"valid mixed", "MY_KEY", "some_value", false},
		{"invalid key number start", "123KEY", "value", true},
		{"invalid key hyphen", "MY-KEY", "value", true},
		{"control char null", "KEY", "val\x00ue", true},
		{"control char newline", "KEY", "val\nue", true},
		{"control char carriage return", "KEY", "val\rue", true},
		{"flag injection", "KEY", "--flag", true},
		{"flag injection single", "KEY", "-f", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBuildArg(tc.key, tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateBuildArg(%q, %q) error = %v, wantErr %v", tc.key, tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestIsPrivateOrBlockedIP(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", false},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := isPrivateOrBlockedIP(tc.host)
			if got != tc.want {
				t.Errorf("isPrivateOrBlockedIP(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestValidateResolvedHost(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errSub  string
	}{
		{"invalid_url", "://not-a-url", true, ""},
		{"file_scheme", "file:///etc/passwd", false, ""},
		{"ftp_scheme", "ftp://example.com/file", false, ""},
		{"ssh_empty_host", "ssh:///path/to/repo", false, ""},
		{"https_empty_host", "https:///no-host", false, ""},
		{"http_empty_host", "http:///no-host", false, ""},
		{"unresolvable_host", "https://this-host-definitely-does-not-exist-xyz.invalid/owner/repo", true, "DNS lookup failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResolvedHost(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// === merged from tier69_hardening_test.go ===

// Tier 69 — build worker pool hardening tests.
//
// These cover the regressions fixed in Tier 69:
//   - Submit+Wait race on wg.Add (Go WaitGroup contract violation)
//   - Submit wedges indefinitely on a full sem after pool shutdown
//   - Module.Stop ignores its ctx parameter
//   - NewWorkerPool(negative) panics at runtime in make(chan)
//   - Submit after Shutdown silently drops (backward compat) /
//     SubmitCtx returns ErrPoolClosed
//   - Shutdown is idempotent and honors ctx deadline
//   - Panic recovery uses the pool's structured logger

// ─── Shutdown idempotency ─────────────────────────────────────────────────

func TestTier69_WorkerPool_Shutdown_Idempotent(t *testing.T) {
	pool := NewWorkerPoolWithLogger(3, tier69Logger())

	// Two Shutdown calls must not panic (no double-close of stopCh).
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if !pool.Closed() {
		t.Error("Closed() should report true after Shutdown")
	}
}

// ─── Submit after Shutdown is a no-op ──────────────────────────────────────

func TestTier69_WorkerPool_SubmitAfterShutdown_Dropped(t *testing.T) {
	pool := NewWorkerPoolWithLogger(3, tier69Logger())
	_ = pool.Shutdown(context.Background())

	// The legacy Submit contract is "no error" — we drop silently.
	var called atomic.Bool
	pool.Submit(func() { called.Store(true) })

	// Give any goroutine a moment to run (there shouldn't be one).
	time.Sleep(20 * time.Millisecond)
	if called.Load() {
		t.Error("Submit after Shutdown executed the job — should have been dropped")
	}

	// SubmitCtx returns ErrPoolClosed so callers can distinguish.
	err := pool.SubmitCtx(context.Background(), func() {})
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("expected ErrPoolClosed, got %v", err)
	}
}

// ─── Shutdown unblocks a pending Submit waiting for a full sem ────────────

// TestTier69_WorkerPool_Shutdown_UnblocksPendingSubmit exercises the
// "Submit wedges when the semaphore is full and nothing else will
// drain it" bug. Before Tier 69 a Submit on a saturated pool had no
// escape hatch; Shutdown could not rescue it.
func TestTier69_WorkerPool_Shutdown_UnblocksPendingSubmit(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())

	// Occupy the single worker slot with a job that blocks until we
	// release it.
	release := make(chan struct{})
	first := make(chan struct{})
	pool.Submit(func() {
		close(first)
		<-release
	})

	<-first // make sure the blocker is holding the slot

	// Now try to submit a second job — it should block on the full
	// semaphore. We wrap it in SubmitCtx so we can observe the
	// rejection.
	submitReturned := make(chan error, 1)
	go func() {
		submitReturned <- pool.SubmitCtx(context.Background(), func() {})
	}()

	// Give the second submit a moment to park on sem.
	time.Sleep(50 * time.Millisecond)

	// Shutdown must rescue the pending submit by signaling stopCh.
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- pool.Shutdown(context.Background()) }()

	// The pending SubmitCtx should return ErrPoolClosed quickly.
	select {
	case err := <-submitReturned:
		if !errors.Is(err, ErrPoolClosed) {
			t.Errorf("pending SubmitCtx should return ErrPoolClosed after Shutdown, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("pending SubmitCtx never returned — Shutdown did not unblock the slot acquire")
	}

	// Release the in-flight job so Shutdown can drain.
	close(release)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Shutdown never returned after in-flight job completed")
	}
}

// ─── Shutdown honors ctx deadline when jobs don't drain ────────────────────

func TestTier69_WorkerPool_Shutdown_HonorsCtxDeadline(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())

	// A job that blocks forever (until the test ends).
	never := make(chan struct{})
	defer close(never)
	pool.Submit(func() { <-never })

	// Give the job a tick to enter fn.
	time.Sleep(20 * time.Millisecond)

	// Shutdown with a tight deadline should return ctx.DeadlineExceeded
	// rather than wait forever.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := pool.Shutdown(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown took %v — did not honor ctx deadline", elapsed)
	}
}

// ─── NewWorkerPool guards against negative max ─────────────────────────────

// TestTier69_NewWorkerPool_NegativeMax_NoPanic catches a real panic:
// make(chan struct{}, -1) panics at runtime with "makechan: size out
// of range". Before Tier 69 a misconfigured Limits.MaxConcurrentBuilds
// would crash the process at Init time.
func TestTier69_NewWorkerPool_NegativeMax_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewWorkerPool(-1) panicked: %v", r)
		}
	}()

	pool := NewWorkerPoolWithLogger(-1, tier69Logger())
	if pool == nil {
		t.Fatal("NewWorkerPool returned nil")
	}
	// The raw input field is preserved for observability.
	if pool.maxWorkers != -1 {
		t.Errorf("expected maxWorkers=-1 (raw), got %d", pool.maxWorkers)
	}
	// Submit is expected to be unusable on a negative-max pool (the
	// sem is unbuffered), but Shutdown must still succeed cleanly.
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on negative-max pool: %v", err)
	}
}

// ─── NewWorkerPoolWithLogger nil-logger guard ──────────────────────────────

func TestTier69_NewWorkerPoolWithLogger_NilLogger(t *testing.T) {
	pool := NewWorkerPoolWithLogger(3, nil)
	if pool == nil {
		t.Fatal("NewWorkerPoolWithLogger returned nil")
	}
	if pool.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
}

// ─── Panic in a worker does not tear the pool down ─────────────────────────

func TestTier69_WorkerPool_PanicRecovery(t *testing.T) {
	pool := NewWorkerPoolWithLogger(2, tier69Logger())

	var okCount atomic.Int32
	var done sync.WaitGroup

	// Panic job.
	done.Add(1)
	pool.Submit(func() {
		defer done.Done()
		panic("kaboom")
	})

	// Normal job submitted after the panic must still run.
	done.Add(1)
	pool.Submit(func() {
		defer done.Done()
		okCount.Add(1)
	})

	done.Wait()

	if okCount.Load() != 1 {
		t.Errorf("normal job should still run after panic, got %d", okCount.Load())
	}

	if err := pool.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown after panic: %v", err)
	}
}

// ─── SubmitCtx cancellation during slot acquire ────────────────────────────

func TestTier69_WorkerPool_SubmitCtx_CancelPendingSlot(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())
	defer func() { _ = pool.Shutdown(context.Background()) }()

	// Occupy the single slot.
	release := make(chan struct{})
	defer close(release)
	pool.Submit(func() { <-release })

	// Give the first job a moment to claim the slot.
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := pool.SubmitCtx(ctx, func() { t.Error("job should not run") })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

// ─── Concurrent Submit + Shutdown does not race wg ─────────────────────────

// TestTier69_WorkerPool_ConcurrentSubmitShutdown stresses the
// mutex-guarded closed-check. Before Tier 69 the race detector
// flagged this under load: Submit's wg.Add(1) could happen after
// Wait() had already observed zero, which is undefined behavior per
// the WaitGroup contract.
func TestTier69_WorkerPool_ConcurrentSubmitShutdown(t *testing.T) {
	pool := NewWorkerPoolWithLogger(10, tier69Logger())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Submit(func() { time.Sleep(1 * time.Millisecond) })
		}()
	}

	// Race Shutdown against the submits.
	shutdownDone := make(chan error, 1)
	go func() {
		time.Sleep(5 * time.Millisecond)
		shutdownDone <- pool.Shutdown(context.Background())
	}()

	wg.Wait()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Errorf("Shutdown returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown deadlocked under concurrent submit")
	}
}

// ─── Module.Stop honors its ctx ────────────────────────────────────────────

// TestTier69_Module_Stop_HonorsCtx proves that Module.Stop now
// propagates its context parameter to pool.Shutdown. Before Tier 69
// Stop was `Stop(_ context.Context)` and called pool.Wait() with no
// deadline, so a stuck build could hold up the entire module graph
// indefinitely.
func TestTier69_Module_Stop_HonorsCtx(t *testing.T) {
	m := &Module{
		logger: tier69Logger(),
		pool:   NewWorkerPoolWithLogger(1, tier69Logger()),
	}

	never := make(chan struct{})
	defer close(never)
	m.pool.Submit(func() { <-never })

	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := m.Stop(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded from Module.Stop, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Module.Stop took %v — ctx deadline not honored", elapsed)
	}
}

// ─── Closed() status tracking ──────────────────────────────────────────────

func TestTier69_WorkerPool_ClosedStatus(t *testing.T) {
	pool := NewWorkerPoolWithLogger(1, tier69Logger())
	if pool.Closed() {
		t.Error("new pool should not be Closed")
	}
	_ = pool.Shutdown(context.Background())
	if !pool.Closed() {
		t.Error("pool should be Closed after Shutdown")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier69Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
