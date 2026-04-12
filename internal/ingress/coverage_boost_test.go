package ingress

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init + Start + Stop
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Init_SetsAllFields(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			ACME: core.ACMEConfig{
				Email:   "admin@example.com",
				Staging: true,
			},
		},
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.router == nil {
		t.Error("expected router to be initialized")
	}
	if m.proxy == nil {
		t.Error("expected proxy to be initialized")
	}
	if m.certStore == nil {
		t.Error("expected certStore to be initialized")
	}
	if m.acme == nil {
		t.Error("expected acme to be initialized")
	}
	if m.logger == nil {
		t.Error("expected logger to be set")
	}
	if m.core != c {
		t.Error("expected core reference to be set")
	}
}

func TestModule_Init_HealthAfterInit(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			ACME: core.ACMEConfig{Email: "test@example.com", Staging: true},
		},
	}

	m.Init(context.Background(), c)

	if m.Health() != core.HealthOK {
		t.Errorf("expected HealthOK after Init, got %v", m.Health())
	}
}

func TestModule_Init_RouterUsable(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			ACME: core.ACMEConfig{Email: "test@example.com", Staging: true},
		},
	}

	m.Init(context.Background(), c)

	// Router should be usable
	rt := m.Router()
	if rt == nil {
		t.Fatal("Router() returned nil after Init")
	}

	rt.Upsert(&RouteEntry{
		Host:       "test.com",
		PathPrefix: "/",
		Backends:   []string{"127.0.0.1:8080"},
	})

	if rt.Count() != 1 {
		t.Errorf("expected 1 route, got %d", rt.Count())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// httpHandler — ACME challenge path and HTTPS redirect
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_httpHandler_HTTPSRedirect(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "test@example.com", true, slog.Default())

	handler := m.httpHandler()

	req := httptest.NewRequest("GET", "http://example.com/some/path?q=1", nil)
	req.Host = "example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Errorf("expected 301 redirect, got %d", rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.HasPrefix(location, "https://") {
		t.Errorf("expected HTTPS redirect, got %q", location)
	}
	if !strings.Contains(location, "example.com/some/path?q=1") {
		t.Errorf("expected redirect to preserve path+query, got %q", location)
	}
}

func TestModule_httpHandler_ACMEChallenge(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "test@example.com", true, slog.Default())

	handler := m.httpHandler()

	req := httptest.NewRequest("GET", "http://example.com/.well-known/acme-challenge/test-token-abc", nil)
	req.Host = "example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// autocert returns 403 for unknown/unrequested challenge tokens
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for unknown ACME challenge, got %d", rr.Code)
	}
}

func TestModule_httpHandler_ShortACMEPath(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "test@example.com", true, slog.Default())

	handler := m.httpHandler()

	// Path exactly /.well-known/acme-challenge/ (no token)
	req := httptest.NewRequest("GET", "http://example.com/.well-known/acme-challenge/", nil)
	req.Host = "example.com"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// autocert treats the bare challenge directory as a challenge request
	// and returns 403 because no token is present
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for bare ACME path from autocert, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// tlsConfig
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_tlsConfig(t *testing.T) {
	m := New()
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "test@example.com", true, slog.Default())

	cfg := m.tlsConfig()

	if cfg == nil {
		t.Fatal("expected non-nil TLS config")
	}
	if cfg.MinVersion != 0x0303 { // tls.VersionTLS12
		t.Errorf("expected MinVersion TLS 1.2, got %x", cfg.MinVersion)
	}
	if cfg.GetCertificate == nil {
		t.Error("expected GetCertificate callback to be set")
	}
	if len(cfg.NextProtos) != 2 {
		t.Errorf("expected 2 next protos (h2, http/1.1), got %d", len(cfg.NextProtos))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ServeHTTP — StripPrefix path
// ═══════════════════════════════════════════════════════════════════════════════

func TestReverseProxy_ServeHTTP_StripPrefix(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:        "myapp.com",
		PathPrefix:  "/api",
		Backends:    []string{backendAddr},
		StripPrefix: true,
	})

	rp := NewReverseProxy(rt, slog.Default())

	req := httptest.NewRequest("GET", "http://myapp.com/api/users", nil)
	req.Host = "myapp.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotPath != "/users" {
		t.Errorf("expected stripped path '/users', got %q", gotPath)
	}
}

func TestReverseProxy_ServeHTTP_StripPrefix_RootResult(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:        "myapp.com",
		PathPrefix:  "/api",
		Backends:    []string{backendAddr},
		StripPrefix: true,
	})

	rp := NewReverseProxy(rt, slog.Default())

	// Request exactly the prefix — should result in "/"
	req := httptest.NewRequest("GET", "http://myapp.com/api", nil)
	req.Host = "myapp.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotPath != "/" {
		t.Errorf("expected root path '/' after stripping, got %q", gotPath)
	}
}

func TestReverseProxy_ServeHTTP_NoStripPrefix(t *testing.T) {
	var gotPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendAddr := strings.TrimPrefix(backend.URL, "http://")

	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:        "myapp.com",
		PathPrefix:  "/api",
		Backends:    []string{backendAddr},
		StripPrefix: false,
	})

	rp := NewReverseProxy(rt, slog.Default())

	req := httptest.NewRequest("GET", "http://myapp.com/api/users", nil)
	req.Host = "myapp.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotPath != "/api/users" {
		t.Errorf("expected original path '/api/users', got %q", gotPath)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ServeHTTP — proxy error handler (invalid backend)
// ═══════════════════════════════════════════════════════════════════════════════

func TestReverseProxy_ServeHTTP_BackendConnectionError(t *testing.T) {
	rt := NewRouteTable()
	rt.Upsert(&RouteEntry{
		Host:       "app.example.com",
		PathPrefix: "/",
		Backends:   []string{"127.0.0.1:1"}, // unreachable port
	})

	rp := NewReverseProxy(rt, slog.Default())

	req := httptest.NewRequest("GET", "http://app.example.com/", nil)
	req.Host = "app.example.com"
	rr := httptest.NewRecorder()

	rp.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for unreachable backend, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RenewalLoop — verifies cancellation
// ═══════════════════════════════════════════════════════════════════════════════

func TestACMEManager_RenewalLoop_ContextCancel(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		am.RenewalLoop(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// RenewalLoop exited as expected
	case <-context.Background().Done():
		t.Fatal("RenewalLoop did not exit on context cancel")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module Stop — with servers
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Stop_WithHTTPServer(t *testing.T) {
	m := New()
	m.httpServer = &http.Server{}

	// Create a listener and start the server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	// Replace with a real server for shutdown
	m.httpServer = &http.Server{Handler: http.DefaultServeMux}

	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// pickBackend — round-robin
// ═══════════════════════════════════════════════════════════════════════════════

func TestPickBackend_MultipleBackends(t *testing.T) {
	route := &RouteEntry{
		Backends: []string{"10.0.0.1:80", "10.0.0.2:80", "10.0.0.3:80"},
	}

	// Call multiple times and verify distribution
	seen := make(map[string]int)
	for i := 0; i < 9; i++ {
		b := pickBackend(route)
		seen[b]++
	}

	if len(seen) != 3 {
		t.Errorf("expected 3 different backends, got %d", len(seen))
	}
	for backend, count := range seen {
		if count != 3 {
			t.Errorf("backend %s hit %d times, expected 3 (round-robin)", backend, count)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// AccessLogger Middleware — unusual status code
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Stop_BothServers(t *testing.T) {
	m := New()

	// Create real HTTP servers so we can shut them down
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	m.httpServer = &http.Server{}
	m.tlsServer = &http.Server{}

	// Close the test servers so our Stop doesn't need to actually shut them down
	httpSrv.Close()
	tlsSrv.Close()

	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

func TestModule_Stop_OnlyTLSServer(t *testing.T) {
	m := New()
	m.tlsServer = &http.Server{}

	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop with only TLS server returned error: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module Start with high ports that won't conflict
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Start_HTTPOnly(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	m.router = NewRouteTable()
	m.proxy = NewReverseProxy(m.router, m.logger)
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "test@example.com", true, m.logger)

	m.core = &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Ingress: core.IngressConfig{
				HTTPPort:    0, // OS picks a free port — but Start uses fmt.Sprintf so port 0 = ":0"
				HTTPSPort:   0,
				EnableHTTPS: false,
			},
			ACME: core.ACMEConfig{Email: "test@test.com", Staging: true},
		},
	}

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give servers a moment to start
	m.Stop(context.Background())
}

func TestModule_Start_WithHTTPS(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	m.router = NewRouteTable()
	m.proxy = NewReverseProxy(m.router, m.logger)
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, "test@example.com", true, m.logger)

	m.core = &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Ingress: core.IngressConfig{
				HTTPPort:    0,
				HTTPSPort:   0,
				EnableHTTPS: true,
			},
			ACME: core.ACMEConfig{Email: "test@test.com", Staging: true},
		},
	}

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify TLS server was created
	if m.tlsServer == nil {
		t.Error("expected TLS server to be created when HTTPS enabled")
	}

	m.Stop(context.Background())
}

// ═══════════════════════════════════════════════════════════════════════════════
// GenerateSelfSigned — error branch coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestGenerateSelfSigned_Valid(t *testing.T) {
	cert, err := GenerateSelfSigned("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
	if len(cert.Certificate) == 0 {
		t.Error("expected certificate DER bytes")
	}
	if cert.PrivateKey == nil {
		t.Error("expected private key")
	}
}

func TestGenerateSelfSigned_WildcardDomain(t *testing.T) {
	cert, err := GenerateSelfSigned("*.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned wildcard: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// checkRenewals — exercises the ticker.C path logic directly
// ═══════════════════════════════════════════════════════════════════════════════

func TestACMEManager_CheckRenewals(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Call checkRenewals directly - exercises the ticker.C path logic
	am.checkRenewals()
}

func TestACMEManager_CheckRenewals_WithCachedCerts(t *testing.T) {
	cs := NewCertStore()

	// Add a certificate to the store
	cert, _ := GenerateSelfSigned("test.example.com")
	cs.Put("test.example.com", cert)

	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Call checkRenewals with cached cert
	am.checkRenewals()
}

// ═══════════════════════════════════════════════════════════════════════════════
// issueCertificate — direct invocation
// ═══════════════════════════════════════════════════════════════════════════════

func TestACMEManager_IssueCertificate(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Call issueCertificate directly
	am.issueCertificate("test.example.com")
}

func TestACMEManager_IssueCertificate_StagingMode(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", false, slog.Default()) // production mode

	am.issueCertificate("prod.example.com")
}
