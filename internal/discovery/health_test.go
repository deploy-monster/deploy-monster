package discovery

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- HealthChecker Registration ---

func TestHealthChecker_Register(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("localhost:8080", "http", "/health")

	status := hc.Status()
	if len(status) != 1 {
		t.Fatalf("expected 1 check, got %d", len(status))
	}

	check, ok := status["localhost:8080"]
	if !ok {
		t.Fatal("expected check for localhost:8080")
	}
	if check.Backend != "localhost:8080" {
		t.Errorf("backend = %q, want localhost:8080", check.Backend)
	}
	if check.Type != "http" {
		t.Errorf("type = %q, want http", check.Type)
	}
	if check.Path != "/health" {
		t.Errorf("path = %q, want /health", check.Path)
	}
	if !check.Healthy {
		t.Error("newly registered check should be healthy")
	}
	if check.Failures != 0 {
		t.Errorf("failures = %d, want 0", check.Failures)
	}
	if check.Threshold != 3 {
		t.Errorf("threshold = %d, want 3", check.Threshold)
	}
}

func TestHealthChecker_RegisterMultiple(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("backend-1:3000", "http", "/")
	hc.Register("backend-2:3001", "tcp", "")
	hc.Register("backend-3:3002", "http", "/healthz")

	status := hc.Status()
	if len(status) != 3 {
		t.Errorf("expected 3 checks, got %d", len(status))
	}
}

func TestHealthChecker_RegisterOverwrite(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("backend:8080", "tcp", "")
	hc.Register("backend:8080", "http", "/health") // Overwrite.

	status := hc.Status()
	if len(status) != 1 {
		t.Fatalf("expected 1 check after overwrite, got %d", len(status))
	}
	if status["backend:8080"].Type != "http" {
		t.Errorf("type = %q, want http (overwritten)", status["backend:8080"].Type)
	}
}

// --- Deregistration ---

func TestHealthChecker_Deregister(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("backend:3000", "http", "/")
	hc.Register("backend:3001", "tcp", "")

	hc.Deregister("backend:3000")

	status := hc.Status()
	if len(status) != 1 {
		t.Fatalf("expected 1 check after deregister, got %d", len(status))
	}
	if _, ok := status["backend:3000"]; ok {
		t.Error("deregistered backend should not be present")
	}
	if _, ok := status["backend:3001"]; !ok {
		t.Error("remaining backend should still be present")
	}
}

func TestHealthChecker_DeregisterNonExistent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	// Should not panic.
	hc.Deregister("nonexistent:9999")

	status := hc.Status()
	if len(status) != 0 {
		t.Errorf("expected 0 checks, got %d", len(status))
	}
}

func TestHealthChecker_DeregisterAll(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("a:1", "tcp", "")
	hc.Register("b:2", "tcp", "")
	hc.Register("c:3", "tcp", "")

	hc.Deregister("a:1")
	hc.Deregister("b:2")
	hc.Deregister("c:3")

	status := hc.Status()
	if len(status) != 0 {
		t.Errorf("expected 0 checks after deregistering all, got %d", len(status))
	}
}

// --- IsHealthy ---

func TestHealthChecker_IsHealthy_Default(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("backend:3000", "http", "/")

	if !hc.IsHealthy("backend:3000") {
		t.Error("newly registered backend should be healthy")
	}
}

func TestHealthChecker_IsHealthy_UnknownBackend(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	// Unknown backends are assumed healthy per the code.
	if !hc.IsHealthy("unknown:9999") {
		t.Error("unknown backends should be assumed healthy")
	}
}

// --- Health Check Execution ---

func TestHealthChecker_CheckAll_HTTPHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	// Extract host:port from the test server URL.
	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/")

	// Run check.
	hc.checkAll()

	status := hc.Status()
	check := status[backend]
	if check == nil {
		t.Fatal("check not found")
	}
	if !check.Healthy {
		t.Error("backend should be healthy after 200 response")
	}
	if check.Failures != 0 {
		t.Errorf("failures = %d, want 0", check.Failures)
	}
	if check.LastChecked.IsZero() {
		t.Error("LastChecked should be set")
	}
}

func TestHealthChecker_CheckAll_HTTPUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/health")

	// Run check multiple times to exceed threshold (default 3).
	hc.checkAll()
	hc.checkAll()

	// After 2 failures, still healthy (threshold = 3).
	if !hc.IsHealthy(backend) {
		t.Error("should still be healthy after 2 failures (threshold=3)")
	}

	hc.checkAll() // 3rd failure.

	if hc.IsHealthy(backend) {
		t.Error("should be unhealthy after 3 failures (threshold reached)")
	}

	status := hc.Status()
	check := status[backend]
	if check.Failures < 3 {
		t.Errorf("failures = %d, want >= 3", check.Failures)
	}
	if check.LastError == "" {
		t.Error("LastError should be set")
	}
}

func TestHealthChecker_CheckAll_Recovery(t *testing.T) {
	healthy := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/")

	// Start healthy.
	hc.checkAll()
	if !hc.IsHealthy(backend) {
		t.Error("should be healthy initially")
	}

	// Make unhealthy.
	healthy = false
	for i := 0; i < 3; i++ {
		hc.checkAll()
	}
	if hc.IsHealthy(backend) {
		t.Error("should be unhealthy after threshold failures")
	}

	// Recover.
	healthy = true
	hc.checkAll()
	if !hc.IsHealthy(backend) {
		t.Error("should recover after successful check")
	}

	status := hc.Status()
	check := status[backend]
	if check.Failures != 0 {
		t.Errorf("failures = %d, want 0 after recovery", check.Failures)
	}
	if check.LastError != "" {
		t.Errorf("LastError = %q, want empty after recovery", check.LastError)
	}
}

func TestHealthChecker_CheckAll_TCPHealthy(t *testing.T) {
	// Use a test HTTP server as a TCP endpoint (it accepts TCP connections).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "tcp", "")

	hc.checkAll()

	if !hc.IsHealthy(backend) {
		t.Error("TCP check to live server should be healthy")
	}
}

func TestHealthChecker_CheckAll_TCPUnhealthy(t *testing.T) {
	// Obtain a reliably-closed port: bind 127.0.0.1:0, read the assigned
	// ephemeral port, close immediately. Using a hardcoded low port like :1
	// is unreliable on Windows, where local services or AV drivers can
	// accept loopback connections on reserved ports.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	closedAddr := l.Addr().String()
	l.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register(closedAddr, "tcp", "")

	for i := 0; i < 3; i++ {
		hc.checkAll()
	}

	if hc.IsHealthy(closedAddr) {
		t.Errorf("TCP check to closed port %s should be unhealthy", closedAddr)
	}
}

func TestHealthChecker_CheckAll_UnknownTypeFallsToTCP(t *testing.T) {
	// Use a test HTTP server as a TCP endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "grpc", "") // Unknown type should fall through to TCP.

	hc.checkAll()

	if !hc.IsHealthy(backend) {
		t.Error("unknown check type should fall back to TCP")
	}
}

// --- Status ---

func TestHealthChecker_Status_ReturnsSnapshot(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("backend:3000", "http", "/")
	hc.Register("backend:3001", "tcp", "")

	status := hc.Status()

	// Modify the returned status — should not affect internal state.
	delete(status, "backend:3000")

	internalStatus := hc.Status()
	if len(internalStatus) != 2 {
		t.Error("modifying returned status should not affect internal state")
	}
}

func TestHealthChecker_Status_Empty(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	status := hc.Status()
	if status == nil {
		t.Error("Status() should return non-nil map even when empty")
	}
	if len(status) != 0 {
		t.Errorf("expected 0 entries, got %d", len(status))
	}
}

// --- Start/Stop ---

func TestHealthChecker_StartStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	// Start should not panic.
	hc.Start()

	// Stop should not panic.
	hc.Stop()
}

// --- NewHealthChecker ---

func TestNewHealthChecker(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	if hc == nil {
		t.Fatal("NewHealthChecker returned nil")
	}
	if hc.checks == nil {
		t.Error("checks map should be initialized")
	}
	if hc.client == nil {
		t.Error("HTTP client should be initialized")
	}
	if hc.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}
