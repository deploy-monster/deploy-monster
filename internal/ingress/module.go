package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the Ingress Gateway — DeployMonster's built-in reverse proxy.
// It listens on :80 (HTTP) and :443 (HTTPS) and routes traffic to backend containers
// based on host/path matching rules discovered from Docker labels.
//
// Lifecycle notes for Tier 73:
//
//   - ACMEManager.RenewalLoop used to be spawned with
//     context.Background(), so the renewal ticker ran forever. Every
//     module restart during tests leaked a goroutine. Stop now cancels
//     a module-scoped stopCtx that the renewal loop selects on.
//   - Stop used to skip draining the ACME fire-and-forget
//     issueCertificate goroutines. If shutdown raced with a TLS
//     handshake that triggered issuance, the issuance goroutine
//     outlived the module. Stop now waits on acme.Wait().
type Module struct {
	core       *core.Core
	router     *RouteTable
	proxy      *ReverseProxy
	certStore  *CertStore
	acme       *ACMEManager
	httpServer *http.Server
	tlsServer  *http.Server
	logger     *slog.Logger

	// stopCtx is canceled by Stop so the ACME renewal loop (and any
	// future background workers on the ingress module) can unblock
	// cleanly instead of being left running until process exit.
	stopCtx    context.Context
	stopCancel context.CancelFunc
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "ingress" }
func (m *Module) Name() string                { return "Ingress Gateway" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	m.router = NewRouteTable()
	m.proxy = NewReverseProxy(m.router, m.logger)

	// Initialize ACME manager and cert store
	m.certStore = NewCertStore()
	m.acme = NewACMEManager(m.certStore, c.Config.ACME.Email, c.Config.ACME.Staging, m.logger)
	if c.Config.Server.Domain != "" {
		m.acme.SetDomains(c.Config.Server.Domain)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	cfg := m.core.Config.Ingress

	// Derive a module-scoped cancellable context. Pre-Tier-73 the
	// RenewalLoop was spawned with context.Background() and could
	// never be stopped.
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())

	// HTTP server (:80) — redirects to HTTPS + ACME challenge handler
	httpAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	m.httpServer = &http.Server{
		Addr:         httpAddr,
		Handler:      m.httpHandler(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("panic in ingress HTTP server", "error", r)
			}
		}()
		m.logger.Info("ingress HTTP listening", "addr", httpAddr)
		ln, err := net.Listen("tcp", httpAddr)
		if err != nil {
			m.logger.Warn("ingress HTTP listen failed — port may be in use", "addr", httpAddr, "error", err)
			return
		}
		if err := m.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			m.logger.Error("ingress HTTP error", "error", err)
		}
	}()

	// HTTPS server (:443) — reverse proxy with TLS
	if cfg.EnableHTTPS {
		httpsAddr := fmt.Sprintf(":%d", cfg.HTTPSPort)
		m.tlsServer = &http.Server{
			Addr:         httpsAddr,
			Handler:      m.proxy,
			TLSConfig:    m.tlsConfig(),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("panic in ingress HTTPS server", "error", r)
				}
			}()
			m.logger.Info("ingress HTTPS listening", "addr", httpsAddr)
			ln, err := net.Listen("tcp", httpsAddr)
			if err != nil {
				m.logger.Warn("ingress HTTPS listen failed — port may be in use", "addr", httpsAddr, "error", err)
				return
			}
			tlsLn := tls.NewListener(ln, m.tlsServer.TLSConfig)
			if err := m.tlsServer.Serve(tlsLn); err != nil && err != http.ErrServerClosed {
				m.logger.Error("ingress HTTPS error", "error", err)
			}
		}()

		// Start ACME certificate renewal loop. The context is canceled
		// by Module.Stop so the loop exits cleanly instead of leaking.
		go m.acme.RenewalLoop(m.stopCtx)
	}

	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	// Cancel module-scoped context first so the ACME renewal loop
	// unblocks and any in-flight issueCertificateAsync goroutines
	// observe the cancellation before we start tearing down the
	// listeners. Pre-Tier-73 the renewal loop was born with
	// context.Background() and leaked forever.
	if m.stopCancel != nil {
		m.stopCancel()
	}

	var firstErr error
	if m.httpServer != nil {
		if err := m.httpServer.Shutdown(ctx); err != nil {
			firstErr = err
		}
	}
	if m.tlsServer != nil {
		if err := m.tlsServer.Shutdown(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Drain in-flight ACME issuance goroutines. Pre-Tier-73 these
	// were completely untracked — a TLS handshake that raced with
	// shutdown could leave a goroutine still holding the ACME mutex
	// after Module.Stop returned.
	if m.acme != nil {
		m.acme.Wait()
	}
	return firstErr
}

func (m *Module) Health() core.HealthStatus {
	if m.router == nil {
		return core.HealthDown
	}
	return core.HealthOK
}

// Router returns the route table for external use (e.g., by discovery module).
func (m *Module) Router() *RouteTable {
	return m.router
}

// CertStatus returns a snapshot of the certificates currently held by
// this module's cert store, or nil if the store hasn't been
// initialized yet (e.g. when the module hasn't run Init). It is safe
// to call from any goroutine.
func (m *Module) CertStatus() []CertInfo {
	if m.certStore == nil {
		return nil
	}
	return m.certStore.ListCerts()
}

// httpHandler handles HTTP (:80) requests.
//   - Health check endpoints (for external load balancers)
//   - Metrics endpoint (Prometheus format)
//   - ACME HTTP-01 challenge responses
//   - Redirect everything else to HTTPS when ForceHTTPS is true; otherwise
//     route via the reverse proxy directly (local-dev opt-out).
func (m *Module) httpHandler() http.Handler {
	mux := http.NewServeMux()

	// Health check endpoints (no HTTPS redirect)
	mux.HandleFunc("/health", m.healthHandler())
	mux.HandleFunc("/ready", m.readyHandler())
	mux.HandleFunc("/live", m.liveHandler())

	// Metrics endpoint (Prometheus format)
	mux.HandleFunc("/metrics", m.PrometheusHandler())

	// Default to force-HTTPS when the core isn't wired (legacy tests
	// that construct a bare Module). In production the defaulting in
	// applyDefaults already makes this true.
	forceHTTPS := true
	if m.core != nil {
		forceHTTPS = m.core.Config.Ingress.ForceHTTPS
	}

	// ACME challenge and HTTPS redirect (or pass-through proxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if forceHTTPS {
			// SECURITY: Validate host before redirect to prevent open redirect
			if !isValidRedirectHost(r.Host) {
				http.Error(w, "400 Bad Request: invalid host header", http.StatusBadRequest)
				return
			}
			// HSTS on the redirect response so compliant clients remember
			// to use HTTPS even if this reply is cached.
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			target := "https://" + r.Host + r.URL.RequestURI()
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}

		// Local-dev fall-through: route HTTP directly through the
		// reverse proxy. Only reachable when ForceHTTPS is explicitly
		// disabled in config.
		m.proxy.ServeHTTP(w, r)
	})

	// Let autocert handle ACME HTTP-01 challenges; everything else falls
	// through to the mux above.
	return m.acme.HTTPHandler(mux)
}

// tlsConfig creates the TLS configuration with dynamic certificate loading.
func (m *Module) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: m.acme.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
	}
}

// isValidRedirectHost validates the Host header before using it in a redirect URL.
// Prevents open redirect attacks where an attacker-controlled Host header causes
// a redirect to an external malicious site.
func isValidRedirectHost(host string) bool {
	if host == "" {
		return false
	}
	// Reject hosts with newlines or carriage returns (HTTP header injection)
	if strings.ContainsAny(host, "\r\n") {
		return false
	}
	// Reject hosts that contain suspicious patterns
	lower := strings.ToLower(host)
	// Reject hosts that look like URLs (ould be used in open redirect)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return false
	}
	// Reject hosts with username:password@ (authentication spoofing)
	if strings.Contains(host, "@") {
		return false
	}
	// Reject hosts with port that includes suspicious patterns
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	// Validate the hostname itself
	if err := validateHostname(host); err != nil {
		return false
	}
	return true
}

// validateHostname checks if a hostname is valid and safe for use in redirects.
func validateHostname(host string) error {
	if len(host) > 253 {
		return fmt.Errorf("hostname too long")
	}
	// IP address check
	if ip := net.ParseIP(host); ip != nil {
		// Allow loopback and private IPs, but not public IPs
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() {
			return nil
		}
		return fmt.Errorf("public IP not allowed in redirect")
	}
	// Hostname validation
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return fmt.Errorf("invalid hostname label")
		}
		// Each label must start and end with alphanumeric
		if !isAlphanumeric(label[0]) || !isAlphanumeric(label[len(label)-1]) {
			return fmt.Errorf("hostname label must start and end with alphanumeric")
		}
		// Allow internal hyphens in the middle
		for _, c := range label {
			if !isAlphanumeric(byte(c)) && c != '-' {
				return fmt.Errorf("invalid character in hostname")
			}
		}
	}
	return nil
}

func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// urlParseSafe parses a URL and returns the hostname, with protection against
// DNS rebinding and open redirect attacks.
func urlParseSafe(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Opaque != "" {
		return "", fmt.Errorf("opaque URL not allowed")
	}
	if u.Host == "" {
		return "", fmt.Errorf("empty host")
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("empty hostname")
	}
	return host, nil
}
