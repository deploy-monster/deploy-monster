package ingress

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the Ingress Gateway — DeployMonster's built-in reverse proxy.
// It listens on :80 (HTTP) and :443 (HTTPS) and routes traffic to backend containers
// based on host/path matching rules discovered from Docker labels.
type Module struct {
	core       *core.Core
	router     *RouteTable
	proxy      *ReverseProxy
	certStore  *CertStore
	acme       *ACMEManager
	httpServer *http.Server
	tlsServer  *http.Server
	logger     *slog.Logger
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

	return nil
}

func (m *Module) Start(_ context.Context) error {
	cfg := m.core.Config.Ingress

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

		// Start ACME certificate renewal loop
		go m.acme.RenewalLoop(context.Background())
	}

	return nil
}

func (m *Module) Stop(ctx context.Context) error {
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

// httpHandler handles HTTP (:80) requests.
// - ACME HTTP-01 challenge responses
// - Redirect everything else to HTTPS
func (m *Module) httpHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ACME HTTP-01 challenge (handled by ACME module later)
		if len(r.URL.Path) > len("/.well-known/acme-challenge/") &&
			r.URL.Path[:len("/.well-known/acme-challenge/")] == "/.well-known/acme-challenge/" {
			// Will be handled by ACME module's challenge solver
			http.NotFound(w, r)
			return
		}

		// Redirect to HTTPS
		target := "https://" + r.Host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// tlsConfig creates the TLS configuration with dynamic certificate loading.
func (m *Module) tlsConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: m.acme.GetCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
	}
}
