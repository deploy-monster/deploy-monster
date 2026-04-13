package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

const acmeRenewalInterval = 24 * time.Hour

// ACMEManager handles automatic SSL certificate provisioning via Let's Encrypt.
// It wraps golang.org/x/crypto/acme/autocert for HTTP-01 challenges.
type ACMEManager struct {
	mu        sync.Mutex
	mgr       *autocert.Manager
	certStore *CertStore
	email     string
	staging   bool
	logger    *slog.Logger
	wg        sync.WaitGroup
}

// NewACMEManager creates a certificate manager. If email is empty, ACME is
// disabled and the manager only serves from the cert store or self-signed.
func NewACMEManager(certStore *CertStore, email string, staging bool, logger *slog.Logger) *ACMEManager {
	if logger == nil {
		logger = slog.Default()
	}
	a := &ACMEManager{
		certStore: certStore,
		email:     email,
		staging:   staging,
		logger:    logger,
	}
	if email != "" {
		directoryURL := acme.LetsEncryptURL
		if staging {
			directoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
		}
		a.mgr = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      &autocertCache{store: certStore},
			HostPolicy: autocert.HostWhitelist(), // populated via SetDomains
			Email:      email,
			Client: &acme.Client{
				DirectoryURL: directoryURL,
			},
		}
	}
	return a
}

// SetDomains updates the allowed host whitelist for autocert.
// Call this after config is loaded (e.g. from server.domain).
func (a *ACMEManager) SetDomains(domains ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mgr != nil {
		a.mgr.HostPolicy = autocert.HostWhitelist(domains...)
	}
}

// GetCertificate implements tls.Config.GetCertificate.
func (a *ACMEManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName
	if domain == "" {
		domain = "localhost"
	}

	// Try autocert first for real domains when ACME is enabled.
	if a.mgr != nil && domain != "localhost" && domain != "127.0.0.1" {
		cert, err := a.mgr.GetCertificate(hello)
		if err == nil && cert != nil {
			return cert, nil
		}
		a.logger.Debug("autocert failed, falling back", "domain", domain, "error", err)
	}

	// Fallback to in-memory cert store.
	if cert := a.certStore.Get(domain); cert != nil {
		return cert, nil
	}

	// Final fallback: temporary self-signed cert.
	return GenerateSelfSigned(domain)
}

// HTTPHandler returns an http.Handler that serves ACME HTTP-01 challenges.
// If ACME is disabled it returns the provided fallback handler unchanged.
func (a *ACMEManager) HTTPHandler(fallback http.Handler) http.Handler {
	if a.mgr != nil {
		return a.mgr.HTTPHandler(fallback)
	}
	return fallback
}

// Wait blocks until in-flight goroutines finish. Currently a no-op because
// autocert manages its own concurrency, but kept for lifecycle symmetry.
func (a *ACMEManager) Wait() {
	a.wg.Wait()
}

// RenewalLoop checks all certificates daily and renews those expiring within 30 days.
// With autocert, renewal is automatic; this loop exists for logging/monitoring only.
func (a *ACMEManager) RenewalLoop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("panic in ACME renewal loop", "error", r)
		}
	}()

	ticker := time.NewTicker(acmeRenewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.checkRenewals()
		case <-ctx.Done():
			return
		}
	}
}

func (a *ACMEManager) checkRenewals() {
	a.logger.Debug("checking certificate renewals")
	for _, info := range a.certStore.ExpiringCerts(30 * 24 * time.Hour) {
		a.logger.Info("certificate nearing expiry", "domain", info.Domain, "days_left", info.DaysLeft)
	}
}

// autocertCache wraps CertStore to satisfy autocert.Cache.
type autocertCache struct {
	store *CertStore
}

func (c *autocertCache) Get(ctx context.Context, key string) ([]byte, error) {
	cert := c.store.Get(key)
	if cert == nil {
		return nil, autocert.ErrCacheMiss
	}
	var out []byte
	for _, der := range cert.Certificate {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	if key, ok := cert.PrivateKey.(*ecdsa.PrivateKey); ok {
		if keyDER, err := x509.MarshalECPrivateKey(key); err == nil {
			out = append(out, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})...)
		}
	}
	return out, nil
}

func (c *autocertCache) Put(ctx context.Context, key string, data []byte) error {
	cert, err := tls.X509KeyPair(data, data)
	if err != nil {
		return err
	}
	c.store.Put(key, &cert)
	return nil
}

func (c *autocertCache) Delete(ctx context.Context, key string) error {
	c.store.Remove(key)
	return nil
}
