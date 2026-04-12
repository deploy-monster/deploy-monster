package ingress

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewReverseProxy(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()

	rp := NewReverseProxy(rt, logger)
	if rp == nil {
		t.Fatal("expected non-nil ReverseProxy")
	}
	if rp.router != rt {
		t.Error("expected router to be set")
	}
	if rp.logger != logger {
		t.Error("expected logger to be set")
	}
	if rp.metrics == nil {
		t.Error("expected metrics to be initialized")
	}
	if rp.circuit == nil {
		t.Error("expected circuit breaker manager to be initialized")
	}
}

func TestReverseProxy_ServeHTTP_NoRoute(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	req := httptest.NewRequest("GET", "http://unknown.host.com/path", nil)
	req.Host = "unknown.host.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected status 502 for unknown host, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "502") {
		t.Error("expected response body to contain 502")
	}
}

func TestReverseProxy_ServeHTTP_NoBackends(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{}, // no backends
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 for no backends, got %d", rr.Code)
	}
}

func TestReverseProxy_ServeHTTP_WithBackend(t *testing.T) {
	// Create a real test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "test")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	// Extract host:port from backend URL (strip http://)
	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{backendAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	req := httptest.NewRequest("GET", "http://app.example.com/test", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "hello from backend") {
		t.Error("expected response from backend")
	}
}

func TestReverseProxy_CircuitBreaker_FiltersOpenCircuit(t *testing.T) {
	// Create a healthy backend
	healthyBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	}))
	defer healthyBackend.Close()

	healthyAddr := strings.TrimPrefix(healthyBackend.URL, "http://")
	failingAddr := "127.0.0.1:1" // Will fail to connect

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{failingAddr, healthyAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Manually open the circuit for the failing backend
	// Record 5 failures to trigger open state (default threshold)
	for i := 0; i < 5; i++ {
		rp.circuit.RecordFailure(failingAddr)
	}

	// Verify circuit is open
	if !rp.circuit.IsOpen(failingAddr) {
		t.Fatal("circuit should be open for failing backend")
	}

	// Make request - should route to healthy backend only
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	// Should succeed because healthy backend is available
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestReverseProxy_CircuitBreaker_AllBackendsOpen(t *testing.T) {
	failingAddr := "127.0.0.1:1"

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{failingAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Open the circuit
	for i := 0; i < 5; i++ {
		rp.circuit.RecordFailure(failingAddr)
	}

	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	// Should return 503 because all backends have open circuits
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestReverseProxy_CircuitBreaker_RecordsSuccess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{backendAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// First, create a circuit breaker by recording a failure
	rp.circuit.RecordFailure(backendAddr)

	// Now make a successful request - should record success and reset failure count
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Check circuit breaker stats - should have recorded success (failure count reset)
	stats, ok := rp.circuit.Stats(backendAddr)
	if !ok {
		t.Fatal("expected circuit breaker stats for backend")
	}
	if stats.FailureCount != 0 {
		t.Errorf("failure count = %d, want 0 (should be reset after success)", stats.FailureCount)
	}
	if stats.State.String() != "closed" {
		t.Errorf("circuit state = %s, want closed", stats.State)
	}
}

func TestReverseProxy_CircuitBreaker_RecordsFailure(t *testing.T) {
	failingAddr := "127.0.0.1:1" // Will fail to connect

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{failingAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Make a request that will fail
	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	// Should get 502
	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}

	// Check circuit breaker stats - should have recorded failure
	stats, ok := rp.circuit.Stats(failingAddr)
	if !ok {
		t.Fatal("expected circuit breaker stats for backend")
	}
	if stats.FailureCount < 1 {
		t.Errorf("expected at least 1 failure, got %d", stats.FailureCount)
	}
}

func TestReverseProxy_Metrics(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	m := rp.Metrics()
	if m.TotalRequests.Load() != 0 {
		t.Errorf("expected TotalRequests 0, got %d", m.TotalRequests.Load())
	}
	if m.ActiveRequests.Load() != 0 {
		t.Errorf("expected ActiveRequests 0, got %d", m.ActiveRequests.Load())
	}
	if m.ErrorCount.Load() != 0 {
		t.Errorf("expected ErrorCount 0, got %d", m.ErrorCount.Load())
	}
}

func TestReverseProxy_MetricsAfterRequest(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Make a request that will fail (no route match) to increment counters
	req := httptest.NewRequest("GET", "http://noroute.com/", nil)
	req.Host = "noroute.com"
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	snapshot := rp.metrics.Snapshot()
	if snapshot.TotalRequests != 1 {
		t.Errorf("expected TotalRequests 1, got %d", snapshot.TotalRequests)
	}
	// ActiveRequests should be 0 after request completes (deferred DecrementActive)
	if snapshot.ActiveRequests != 0 {
		t.Errorf("expected ActiveRequests 0 after request, got %d", snapshot.ActiveRequests)
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com"},
		{"example.com:443", "example.com"},
		{"example.com:80", "example.com"},
		{"sub.example.com:8080", "sub.example.com"},
		{"localhost", "localhost"},
		{"localhost:3000", "localhost"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"[::1]:8080", "::1"},
	}

	for _, tt := range tests {
		got := extractHost(tt.input)
		if got != tt.want {
			t.Errorf("extractHost(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		xri        string
		remoteAddr string
		want       string
	}{
		{
			name:       "X-Forwarded-For single IP",
			xff:        "203.0.113.50",
			remoteAddr: "10.0.0.1:1234",
			want:       "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For multiple IPs (takes first)",
			xff:        "203.0.113.50, 70.41.3.18, 150.172.238.178",
			remoteAddr: "10.0.0.1:1234",
			want:       "203.0.113.50",
		},
		{
			name:       "X-Real-IP fallback",
			xri:        "198.51.100.10",
			remoteAddr: "10.0.0.1:1234",
			want:       "198.51.100.10",
		},
		{
			name:       "RemoteAddr fallback",
			remoteAddr: "192.168.1.100:54321",
			want:       "192.168.1.100",
		},
		{
			name:       "X-Forwarded-For takes precedence over X-Real-IP",
			xff:        "203.0.113.50",
			xri:        "198.51.100.10",
			remoteAddr: "10.0.0.1:1234",
			want:       "203.0.113.50",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			got := clientIP(req)
			if got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScheme(t *testing.T) {
	tests := []struct {
		name  string
		tls   bool
		proto string
		want  string
	}{
		{
			name: "TLS connection",
			tls:  true,
			want: "https",
		},
		{
			name:  "X-Forwarded-Proto header",
			proto: "https",
			want:  "https",
		},
		{
			name: "plain HTTP",
			want: "http",
		},
		{
			name:  "TLS takes precedence over X-Forwarded-Proto",
			tls:   true,
			proto: "http",
			want:  "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}

			got := scheme(req)
			if got != tt.want {
				t.Errorf("scheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorPage(t *testing.T) {
	tests := []struct {
		status  int
		title   string
		message string
	}{
		{502, "Bad Gateway", "No upstream configured"},
		{503, "Service Unavailable", "No healthy backends"},
		{404, "Not Found", "The requested resource was not found"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			page := ErrorPage(tt.status, tt.title, tt.message)
			if len(page) == 0 {
				t.Fatal("expected non-empty error page")
			}

			body := string(page)
			if !strings.Contains(body, "<!DOCTYPE html>") {
				t.Error("expected HTML doctype")
			}
			if !strings.Contains(body, tt.title) {
				t.Errorf("expected title %q in page", tt.title)
			}
			if !strings.Contains(body, tt.message) {
				t.Errorf("expected message %q in page", tt.message)
			}
			if !strings.Contains(body, "DeployMonster Ingress") {
				t.Error("expected DeployMonster branding in page")
			}
		})
	}
}

func TestReverseProxy_ServeHTTP_ForwardHeaders(t *testing.T) {
	// Create a backend that echoes back the forwarded headers
	var gotHeaders http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "myapp.com",
		PathPrefix: "/",
		Backends:   []string{backendAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	req := httptest.NewRequest("GET", "http://myapp.com/test", nil)
	req.Host = "myapp.com"
	req.RemoteAddr = "192.168.1.50:12345"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Check forwarded headers were set
	if got := gotHeaders.Get("X-Forwarded-Host"); got != "myapp.com" {
		t.Errorf("X-Forwarded-Host = %q, want %q", got, "myapp.com")
	}
	if got := gotHeaders.Get("X-Forwarded-Proto"); got != "http" {
		t.Errorf("X-Forwarded-Proto = %q, want %q", got, "http")
	}
	if got := gotHeaders.Get("X-Real-IP"); got != "192.168.1.50" {
		t.Errorf("X-Real-IP = %q, want %q", got, "192.168.1.50")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Drain Manager Tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestReverseProxy_DrainBackend(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Test draining with no active connections
	err := rp.DrainBackend("127.0.0.1:3000", 100*time.Millisecond)
	if err != nil {
		t.Errorf("DrainBackend with no connections should succeed, got: %v", err)
	}
}

func TestReverseProxy_StartDrain(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// First drain should start
	active, ok := rp.StartDrain("127.0.0.1:3000")
	if !ok {
		t.Error("StartDrain should return ok=true for first drain")
	}
	if active != 0 {
		t.Errorf("active = %d, want 0", active)
	}

	// Second drain should return not ok (already draining)
	active, ok = rp.StartDrain("127.0.0.1:3000")
	if ok {
		t.Error("StartDrain should return ok=false for already draining backend")
	}
	if active != 0 {
		t.Errorf("active = %d, want 0", active)
	}
}

func TestReverseProxy_CompleteDrain(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Start drain
	rp.StartDrain("127.0.0.1:3000")

	// Complete drain
	rp.CompleteDrain("127.0.0.1:3000")

	// Should no longer be draining
	if rp.IsDraining("127.0.0.1:3000") {
		t.Error("backend should not be draining after CompleteDrain")
	}
}

func TestReverseProxy_IsDraining(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Initially not draining
	if rp.IsDraining("127.0.0.1:3000") {
		t.Error("backend should not be draining initially")
	}

	// Start drain
	rp.StartDrain("127.0.0.1:3000")

	// Now draining
	if !rp.IsDraining("127.0.0.1:3000") {
		t.Error("backend should be draining after StartDrain")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Circuit Breaker Tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestReverseProxy_CircuitStats(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// No stats for unknown backend
	_, ok := rp.CircuitStats("127.0.0.1:3000")
	if ok {
		t.Error("CircuitStats should return ok=false for unknown backend")
	}

	// Record a failure to create circuit breaker
	rp.circuit.RecordFailure("127.0.0.1:3000")

	// Now should have stats
	stats, ok := rp.CircuitStats("127.0.0.1:3000")
	if !ok {
		t.Fatal("CircuitStats should return ok=true for known backend")
	}
	if stats.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", stats.FailureCount)
	}
}

func TestReverseProxy_AllCircuitStats(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Empty initially
	allStats := rp.AllCircuitStats()
	if len(allStats) != 0 {
		t.Errorf("AllCircuitStats should be empty initially, got %d entries", len(allStats))
	}

	// Add some backends
	rp.circuit.RecordFailure("127.0.0.1:3000")
	rp.circuit.RecordFailure("127.0.0.1:3001")

	allStats = rp.AllCircuitStats()
	if len(allStats) != 2 {
		t.Errorf("AllCircuitStats should have 2 entries, got %d", len(allStats))
	}
}

func TestReverseProxy_ResetCircuit(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Create circuit and record failures
	rp.circuit.RecordFailure("127.0.0.1:3000")
	rp.circuit.RecordFailure("127.0.0.1:3000")
	rp.circuit.RecordFailure("127.0.0.1:3000")
	rp.circuit.RecordFailure("127.0.0.1:3000")
	rp.circuit.RecordFailure("127.0.0.1:3000") // 5 failures = open

	stats, _ := rp.CircuitStats("127.0.0.1:3000")
	if stats.State.String() != "open" {
		t.Errorf("circuit should be open after 5 failures, got %s", stats.State)
	}

	// Reset
	rp.ResetCircuit("127.0.0.1:3000")

	stats, _ = rp.CircuitStats("127.0.0.1:3000")
	if stats.State.String() != "closed" {
		t.Errorf("circuit should be closed after reset, got %s", stats.State)
	}
}

func TestReverseProxy_filterHealthyBackends(t *testing.T) {
	rt := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	backends := []string{"127.0.0.1:3000", "127.0.0.1:3001", "127.0.0.1:3002"}

	// All healthy
	healthy := rp.filterHealthyBackends(backends)
	if len(healthy) != 3 {
		t.Errorf("expected 3 healthy backends, got %d", len(healthy))
	}

	// Drain one
	rp.StartDrain("127.0.0.1:3000")
	healthy = rp.filterHealthyBackends(backends)
	if len(healthy) != 2 {
		t.Errorf("expected 2 healthy backends after draining one, got %d", len(healthy))
	}

	// Open circuit for another
	for i := 0; i < 5; i++ {
		rp.circuit.RecordFailure("127.0.0.1:3001")
	}
	healthy = rp.filterHealthyBackends(backends)
	if len(healthy) != 1 {
		t.Errorf("expected 1 healthy backend after one drain and one open circuit, got %d", len(healthy))
	}
	if healthy[0] != "127.0.0.1:3002" {
		t.Errorf("expected healthy backend to be 127.0.0.1:3002, got %s", healthy[0])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ResponseTracker Tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestResponseTracker_Write_WithoutWriteHeader(t *testing.T) {
	// Backend that writes body without calling WriteHeader (implicit 200)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't call WriteHeader, just write body directly
		w.Write([]byte("implicit 200"))
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{backendAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Create circuit breaker entry first
	rp.circuit.RecordFailure(backendAddr)

	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	// Should get 200 (implicit)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Check that success was recorded (circuit should be closed)
	stats, ok := rp.CircuitStats(backendAddr)
	if !ok {
		t.Fatal("expected circuit stats")
	}
	// After success, failure count should be 0
	if stats.FailureCount != 0 {
		t.Errorf("failure count = %d, want 0 (success should reset failures)", stats.FailureCount)
	}
}

func TestResponseTracker_WriteHeader_MultipleCalls(t *testing.T) {
	// Backend that calls WriteHeader multiple times
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // First call - counts
		w.WriteHeader(http.StatusOK)                  // Second call - ignored
		w.Write([]byte("error"))
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{backendAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	// Should get 500 (first WriteHeader)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	// Check that failure was recorded (500 = server error)
	stats, ok := rp.CircuitStats(backendAddr)
	if !ok {
		t.Fatal("expected circuit stats")
	}
	if stats.FailureCount != 1 {
		t.Errorf("failure count = %d, want 1 (500 should record failure)", stats.FailureCount)
	}
}

func TestResponseTracker_WriteHeader_4xxNotError(t *testing.T) {
	// Backend returns 404 - client error, not server error
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{backendAddr},
	})

	logger := slog.Default()
	rp := NewReverseProxy(rt, logger)

	// Create circuit breaker entry first (it's created lazily on failure)
	rp.circuit.RecordFailure(backendAddr)
	stats, _ := rp.CircuitStats(backendAddr)
	if stats.FailureCount != 1 {
		t.Fatalf("setup: failure count = %d, want 1", stats.FailureCount)
	}

	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	// Should get 404
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	// Check that success was recorded (4xx is NOT a server error, so success is recorded)
	stats, ok := rp.CircuitStats(backendAddr)
	if !ok {
		t.Fatal("expected circuit stats")
	}
	// Success resets failure count to 0
	if stats.FailureCount != 0 {
		t.Errorf("failure count = %d, want 0 (4xx should record success, resetting failures)", stats.FailureCount)
	}
}
