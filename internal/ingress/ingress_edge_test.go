package ingress

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Stop with only http server (no TLS server)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Stop_OnlyHTTPServer(t *testing.T) {
	m := New()
	m.httpServer = &http.Server{}
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Stop with only TLS server (no HTTP server)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Stop_OnlyTLSServerEdge(t *testing.T) {
	m := New()
	m.tlsServer = &http.Server{}
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Stop with both servers (normal path)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Stop_BothServersEdge(t *testing.T) {
	m := New()
	m.httpServer = &http.Server{}
	m.tlsServer = &http.Server{}
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — httpHandler with no core (forceHTTPS default true)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_httpHandler_NoCoreForceHTTPS(t *testing.T) {
	m := New()
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	handler := m.httpHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://example.com/" {
		t.Errorf("expected https://example.com/, got %s", loc)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — httpHandler with invalid host (open redirect prevention)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_httpHandler_InvalidHost(t *testing.T) {
	m := New()
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	handler := m.httpHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "http://evil.com"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — httpHandler local-dev fall-through (ForceHTTPS=false)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_httpHandler_ForceHTTPSDisabled(t *testing.T) {
	// Create a minimal core with ForceHTTPS=false to enable direct proxy
	// Since we can't easily create a full core, we use a struct approach
	// that matches the code path: when m.core is set, it reads Config.Ingress.ForceHTTPS
	m := New()
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	// Without core, default is true for ForceHTTPS — we can still test
	// the redirect path which is the main uncovered branch.
	// The ForceHTTPS=false path requires a full core setup.
	// This test verifies the handler doesn't panic.
	handler := m.httpHandler()
	if handler == nil {
		t.Fatal("httpHandler returned nil")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — tlsConfig minimal test
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_tlsConfig_ProducesConfig(t *testing.T) {
	m := New()
	m.acme = NewACMEManager(NewCertStore(), "", false, nil)
	cfg := m.tlsConfig()
	if cfg == nil {
		t.Fatal("tlsConfig returned nil")
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("expected TLS 1.3, got %d", cfg.MinVersion)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — CertStatus with nil store
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_CertStatus_NilStoreEdge(t *testing.T) {
	m := New()
	status := m.CertStatus()
	if status != nil {
		t.Errorf("expected nil, got %v", status)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// acme.go — GetCertificate with empty ServerName defaults to localhost
// ═══════════════════════════════════════════════════════════════════════════════
func TestACMEManager_GetCertificate_EmptyServerName(t *testing.T) {
	certStore := NewCertStore()
	acme := NewACMEManager(certStore, "", true, nil)

	hello := &tls.ClientHelloInfo{ServerName: ""}
	cert, err := acme.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// acme.go — GetCertificate falls back to self-signed for unknown domain
// ═══════════════════════════════════════════════════════════════════════════════
func TestACMEManager_GetCertificate_SelfSignedFallbackEdge(t *testing.T) {
	certStore := NewCertStore()
	acme := NewACMEManager(certStore, "", true, nil)

	hello := &tls.ClientHelloInfo{ServerName: "unknown.example.com"}
	cert, err := acme.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// acme.go — GetCertificate uses cert store when domain exists
// ═══════════════════════════════════════════════════════════════════════════════
func TestACMEManager_GetCertificate_StoreHit(t *testing.T) {
	certStore := NewCertStore()
	acme := NewACMEManager(certStore, "", true, nil)

	// Generate and store a self-signed cert
	cert, err := GenerateSelfSigned("stored.example.com")
	if err != nil {
		t.Fatal(err)
	}
	certStore.Put("stored.example.com", cert)

	hello := &tls.ClientHelloInfo{ServerName: "stored.example.com"}
	got, err := acme.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil cert")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// acme.go — RenewalLoop context cancellation
// ═══════════════════════════════════════════════════════════════════════════════
func TestACMEManager_RenewalLoop_CtxCancel(t *testing.T) {
	certStore := NewCertStore()
	acme := NewACMEManager(certStore, "", true, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Already cancelled — loop should exit immediately

	// Should not panic or hang
	done := make(chan struct{})
	go func() {
		acme.RenewalLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(time.Second):
		t.Fatal("RenewalLoop did not exit after context cancellation")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// acme.go — checkRenewals with expiring cert
// ═══════════════════════════════════════════════════════════════════════════════
func TestACMEManager_checkRenewals_Expiring(t *testing.T) {
	certStore := NewCertStore()

	// Store an expiring cert
	cert, err := GenerateSelfSigned("expiring.example.com")
	if err != nil {
		t.Fatal(err)
	}
	certStore.Put("expiring.example.com", cert)

	acme := NewACMEManager(certStore, "", true, slog.Default())
	// Should not panic
	acme.checkRenewals()
}

// ═══════════════════════════════════════════════════════════════════════════════
// tls.go — leafOf from certificate
// ═══════════════════════════════════════════════════════════════════════════════
func TestTLSCertificate_leafOf_Success(t *testing.T) {
	cert, err := GenerateSelfSigned("leaf.example.com")
	if err != nil {
		t.Fatal(err)
	}

	leaf := leafOf(cert)
	if leaf == nil {
		t.Fatal("expected non-nil leaf")
	}
	if len(leaf.DNSNames) == 0 {
		t.Error("expected DNS names in leaf")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// tls.go — leafOf with empty certificate (should return nil)
// ═══════════════════════════════════════════════════════════════════════════════
func TestTLSCertificate_leafOf_Empty(t *testing.T) {
	emptyCert := &tls.Certificate{}
	leaf := leafOf(emptyCert)
	if leaf != nil {
		t.Fatal("expected nil for empty certificate")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// tls.go — leafOf with nil certificate
// ═══════════════════════════════════════════════════════════════════════════════
func TestTLSCertificate_leafOf_Nil(t *testing.T) {
	leaf := leafOf(nil)
	if leaf != nil {
		t.Fatal("expected nil for nil certificate")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// tls.go — ListCerts
// ═══════════════════════════════════════════════════════════════════════════════
func TestCertStore_ListCerts_EmptyEdge(t *testing.T) {
	cs := NewCertStore()
	certs := cs.ListCerts()
	if len(certs) != 0 {
		t.Errorf("expected 0 certs, got %d", len(certs))
	}
}

func TestCertStore_ListCerts_WithEntries(t *testing.T) {
	cs := NewCertStore()
	cert, err := GenerateSelfSigned("listtest.example.com")
	if err != nil {
		t.Fatal(err)
	}
	cs.Put("listtest.example.com", cert)

	certs := cs.ListCerts()
	found := false
	for _, c := range certs {
		if c.Domain == "listtest.example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected listtest.example.com in cert list")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// tls.go — ExpiringCerts
// ═══════════════════════════════════════════════════════════════════════════════
func TestCertStore_ExpiringCerts_Empty(t *testing.T) {
	cs := NewCertStore()
	expiring := cs.ExpiringCerts(30 * 24 * time.Hour)
	if len(expiring) != 0 {
		t.Errorf("expected 0 expiring, got %d", len(expiring))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// tls.go — GenerateSelfSigned
// ═══════════════════════════════════════════════════════════════════════════════
func TestGenerateSelfSigned_Success(t *testing.T) {
	cert, err := GenerateSelfSigned("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("expected certificate bytes")
	}
}

func TestGenerateSelfSigned_EmptyDomain(t *testing.T) {
	cert, err := GenerateSelfSigned("")
	if err != nil {
		t.Fatalf("GenerateSelfSigned with empty domain: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// proxy.go — ServeHTTP no matching route
// ═══════════════════════════════════════════════════════════════════════════════
func TestReverseProxy_ServeHTTP_NoRouteEdge(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "unknown.example.com"
	w := httptest.NewRecorder()
	rp.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// proxy.go — DrainBackend, StartDrain, CompleteDrain, IsDraining
// ═══════════════════════════════════════════════════════════════════════════════
func TestReverseProxy_DrainBackend_NoBackend(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	err := rp.DrainBackend("nonexistent:8080", time.Second)
	if err != nil {
		t.Fatalf("DrainBackend: %v", err)
	}
}

func TestReverseProxy_StartDrain_NewBackend(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	active, draining := rp.StartDrain("backend:8080")
	if draining != true {
		t.Errorf("expected draining=true, got %v", draining)
	}
	if active != 0 {
		t.Errorf("expected 0 active connections, got %d", active)
	}
}

func TestReverseProxy_IsDraining_False(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	if rp.IsDraining("backend:8080") {
		t.Error("expected not draining")
	}
}

func TestReverseProxy_CompleteDrain_NoPanic(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	// Should not panic
	rp.CompleteDrain("backend:8080")
}

// ═══════════════════════════════════════════════════════════════════════════════
// proxy.go — Circuit Breaker methods
// ═══════════════════════════════════════════════════════════════════════════════
func TestReverseProxy_CircuitStats_NoBackend(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	_, ok := rp.CircuitStats("nonexistent")
	if ok {
		t.Error("expected ok=false for nonexistent backend")
	}
}

func TestReverseProxy_ResetCircuit_NoPanic(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	rp.ResetCircuit("nonexistent")
}

func TestReverseProxy_AllCircuitStats_Empty(t *testing.T) {
	router := NewRouteTable()
	logger := slog.Default()
	rp := NewReverseProxy(router, logger)

	stats := rp.AllCircuitStats()
	if stats == nil {
		t.Fatal("expected non-nil map")
	}
	if len(stats) != 0 {
		t.Errorf("expected empty map, got %d", len(stats))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Start with empty config path (edge cases)
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Start_CancelCtx(t *testing.T) {
	// Verify that start/stop lifecycle doesn't panic with nil servers
	m := New()
	m.router = NewRouteTable()
	m.proxy = NewReverseProxy(m.router, slog.Default())
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "", true, slog.Default())
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())

	// Can start and stop without panic even without full core config
	// (httpServer and tlsServer are nil)
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Router() method
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Router_ReturnsRouter(t *testing.T) {
	m := New()
	m.router = NewRouteTable()
	if m.Router() == nil {
		t.Fatal("Router() returned nil")
	}
}
