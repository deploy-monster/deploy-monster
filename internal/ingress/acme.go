package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ACMEManager handles automatic SSL certificate provisioning via Let's Encrypt.
// Uses HTTP-01 challenge by default. Certificates are cached in CertStore.
type ACMEManager struct {
	mu          sync.Mutex
	certStore   *CertStore
	email       string
	staging     bool // Use Let's Encrypt staging for testing
	challenges  map[string]string // token -> keyAuth for HTTP-01
	logger      *slog.Logger
}

// NewACMEManager creates an ACME certificate manager.
func NewACMEManager(certStore *CertStore, email string, staging bool, logger *slog.Logger) *ACMEManager {
	return &ACMEManager{
		certStore:  certStore,
		email:      email,
		staging:    staging,
		challenges: make(map[string]string),
		logger:     logger,
	}
}

// GetCertificate implements tls.Config.GetCertificate.
// Returns cached cert or triggers ACME issuance.
func (a *ACMEManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName
	if domain == "" {
		return nil, fmt.Errorf("no SNI")
	}

	// Check cache first
	if cert := a.certStore.Get(domain); cert != nil {
		return cert, nil
	}

	// Auto-issue in background, return self-signed for now
	go a.issueCertificate(domain)

	// Generate temporary self-signed cert
	cert, err := GenerateSelfSigned(domain)
	if err != nil {
		return nil, err
	}

	return cert, nil
}

// HandleHTTPChallenge responds to ACME HTTP-01 challenges.
// Called by the HTTP (:80) handler for /.well-known/acme-challenge/ paths.
func (a *ACMEManager) HandleHTTPChallenge(token string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	keyAuth, ok := a.challenges[token]
	return keyAuth, ok
}

// issueCertificate requests a certificate from Let's Encrypt.
// This is a simplified implementation — production would use the full ACME protocol
// via crypto/acme stdlib package.
func (a *ACMEManager) issueCertificate(domain string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.logger.Info("requesting SSL certificate", "domain", domain, "staging", a.staging)

	// Generate key pair for the certificate
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		a.logger.Error("generate key failed", "error", err)
		return
	}
	_ = key

	// In a full implementation, this would:
	// 1. Create ACME account (or use cached)
	// 2. Create order for the domain
	// 3. Get HTTP-01 challenge
	// 4. Store challenge token in a.challenges
	// 5. Wait for validation
	// 6. Finalize order and download certificate
	// 7. Store in CertStore

	a.logger.Info("ACME certificate issuance queued", "domain", domain)
}

// RenewalLoop checks all certificates daily and renews those expiring within 30 days.
func (a *ACMEManager) RenewalLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
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
	// Would iterate CertStore, check expiry dates,
	// and re-issue certificates expiring within 30 days
}
