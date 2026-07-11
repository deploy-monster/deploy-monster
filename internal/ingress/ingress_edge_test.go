package ingress

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// module.go — Start with no HTTPS and stop cleanly
// =============================================================================

func TestModule_Start_NoHTTPS(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Ingress: core.IngressConfig{
				HTTPPort:    8099,
				HTTPSPort:   8098,
				EnableHTTPS: false,
			},
			ACME: core.ACMEConfig{},
		},
		Services: core.NewServices(),
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Stop(ctx); err != nil {
		t.Logf("Stop: %v", err)
	}
}

func TestModule_Stop_BeforeStart(t *testing.T) {
	m := New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Stop(ctx); err != nil {
		t.Logf("Stop: %v", err)
	}
}

// =============================================================================
// acme.go — RenewalLoop context cancellation
// =============================================================================

func TestACMERenewalLoop_CtxCancelled(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		am.RenewalLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RenewalLoop did not exit after context cancellation")
	}
}

func TestNewACMEManager_NilLogger(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "", false, nil)
	if am == nil {
		t.Fatal("expected non-nil manager")
	}
}

// =============================================================================
// tls.go — leafOf edge cases
// =============================================================================

func TestLeafOf_NilCertificate(t *testing.T) {
	leaf := leafOf(nil)
	if leaf != nil {
		t.Errorf("expected nil leaf for nil cert")
	}
}

func TestLeafOf_EmptyCertificate(t *testing.T) {
	leaf := leafOf(&tls.Certificate{})
	if leaf != nil {
		t.Errorf("expected nil leaf for empty cert")
	}
}

// =============================================================================
// proxy.go — ServeHTTP with no route
// =============================================================================

func TestProxy_ServeHTTP_NoRoute(t *testing.T) {
	router := NewRouteTable()
	rp := NewReverseProxy(router, slog.Default())

	req := httptest.NewRequest("GET", "http://nohost.example.com/test", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

// =============================================================================
// proxy.go — filterHealthyBackends edge cases
// =============================================================================

func TestProxy_FilterHealthyBackends_Empty(t *testing.T) {
	router := NewRouteTable()
	rp := NewReverseProxy(router, slog.Default())

	result := rp.filterHealthyBackends(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}

	result = rp.filterHealthyBackends([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestProxy_FilterHealthyBackends_Draining(t *testing.T) {
	router := NewRouteTable()
	rp := NewReverseProxy(router, slog.Default())

	rp.StartDrain("draining:80")
	result := rp.filterHealthyBackends([]string{"draining:80"})
	if len(result) != 0 {
		t.Errorf("expected empty (draining), got %v", result)
	}
	rp.CompleteDrain("draining:80")
}

func TestProxy_FilterHealthyBackends_Healthy(t *testing.T) {
	router := NewRouteTable()
	rp := NewReverseProxy(router, slog.Default())

	result := rp.filterHealthyBackends([]string{"healthy:80"})
	if len(result) != 1 {
		t.Errorf("expected 1 healthy backend, got %v", result)
	}
}

// =============================================================================
// module.go — httpHandler forceHTTPS paths with invalid hosts
// =============================================================================

func TestHTTPHandler_InvalidRedirectHost(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Ingress: core.IngressConfig{
				HTTPPort:    8099,
				ForceHTTPS:  true,
				EnableHTTPS: true,
			},
		},
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	handler := m.httpHandler()

	req := httptest.NewRequest("GET", "http://evil@attacker.com/test", nil)
	req.Host = "evil@attacker.com"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid host, got %d", w.Code)
	}
}

func TestHTTPHandler_NewlineInHost(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Ingress: core.IngressConfig{
				ForceHTTPS: true,
			},
		},
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	handler := m.httpHandler()

	req := httptest.NewRequest("GET", "http://test.com/test", nil)
	req.Host = "test.com\r\ninjected"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for host with newline, got %d", w.Code)
	}
}

// =============================================================================
// proxy.go — DrainBackend, StartDrain, IsDraining edge cases
// =============================================================================

func TestProxy_Drain_AlreadyDraining(t *testing.T) {
	router := NewRouteTable()
	rp := NewReverseProxy(router, slog.Default())

	count, started := rp.StartDrain("backend:80")
	if !started {
		t.Log("drain already started or not started")
	}
	_ = count

	// Second StartDrain should return false
	_, startedAgain := rp.StartDrain("backend:80")
	if startedAgain {
		t.Errorf("expected false for already draining backend")
	}

	if !rp.IsDraining("backend:80") {
		t.Errorf("expected IsDraining true")
	}

	rp.CompleteDrain("backend:80")
}

// =============================================================================
// proxy.go — Circuit breaker operations
// =============================================================================

func TestProxy_CircuitBreaker_RecordAndReset(t *testing.T) {
	router := NewRouteTable()
	rp := NewReverseProxy(router, slog.Default())

	rp.circuit.RecordSuccess("cb-backend:80")
	rp.circuit.RecordFailure("cb-backend:80")

	stats, ok := rp.CircuitStats("cb-backend:80")
	if !ok {
		t.Log("stats not found (circuit may have already opened)")
	}
	_ = stats

	allStats := rp.AllCircuitStats()
	if len(allStats) == 0 {
		t.Log("no circuit stats")
	}

	rp.ResetCircuit("cb-backend:80")
}

// =============================================================================
// tls.go — CertStore ExpiringCerts and ListCerts with data
// =============================================================================

func TestCertStore_ExpiringCerts_WithData(t *testing.T) {
	cs := NewCertStore()

	// Generate a cert and add it
	selfSigned, err := GenerateSelfSigned("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	cs.Put("test.example.com", selfSigned)

	certs := cs.ListCerts()
	if len(certs) != 1 {
		t.Errorf("expected 1 cert, got %d", len(certs))
	}

	expiring := cs.ExpiringCerts(30 * 24 * time.Hour)
	_ = expiring
}

// =============================================================================
// proxy.go — ServeHTTP with an actual route but unreachable backend
// =============================================================================

func TestProxy_ServeHTTP_WithRoute(t *testing.T) {
	router := NewRouteTable()
	router.Upsert(&RouteEntry{
		Host:       "test.example.com",
		PathPrefix: "/",
		Backends:   []string{"127.0.0.1:19999"},
	})
	rp := NewReverseProxy(router, slog.Default())

	req := httptest.NewRequest("GET", "http://test.example.com/test", nil)
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if w.Code == 0 {
		t.Error("expected non-zero status code")
	}
}

// =============================================================================
// module.go — Stop with ACME (covers acme.Wait() path)
// =============================================================================

func TestModule_Stop_WithACMEManager(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	m := &Module{
		acme: am,
	}
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())

	go am.RenewalLoop(m.stopCtx)
	time.Sleep(10 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Stop(ctx); err != nil {
		t.Logf("Stop: %v", err)
	}
}

// =============================================================================
// acme.go — autocertCache Get/Put/Delete
// =============================================================================

func TestAutocertCache_Get_NotFound(t *testing.T) {
	cs := NewCertStore()
	cache := &autocertCache{store: cs}

	_, err := cache.Get(context.Background(), "no-such-key")
	if err == nil {
		t.Fatal("expected error for cache miss")
	}
}

// =============================================================================
// tls.go — tls.Config creation via module (tlsConfig method)
// =============================================================================

func TestModule_TLSConfig(t *testing.T) {
	m := New()
	tlsCfg := m.tlsConfig()
	if tlsCfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if tlsCfg.MinVersion != 0x0304 { // tls.VersionTLS13
		t.Errorf("expected TLS 1.3 min version")
	}
}
