package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"log/slog"
	"sync"
	"time"
)

const acmeRenewalInterval = 24 * time.Hour

// ACMEManager handles automatic SSL certificate provisioning via Let's Encrypt.
// Uses HTTP-01 challenge by default. Certificates are cached in CertStore.
//
// Lifecycle notes for Tier 73:
//
//   - GetCertificate used to spawn `go a.issueCertificate(domain)` as
//     a fire-and-forget goroutine with no tracking whatsoever. Under
//     shutdown these could leak and hold the mu lock while running,
//     which meant a subsequent GetCertificate was blocked by a
//     previous issuance that nobody was waiting on. wg now tracks
//     every dispatched goroutine so Wait() drains them.
//   - issueCertificate had no defer/recover. A panic inside the
//     ECDSA key generation or future ACME HTTP call would crash the
//     entire ingress gateway. Each dispatch now has its own
//     defer/recover.
//   - NewACMEManager tolerates a nil logger by falling back to
//     slog.Default, matching the Tier 68/69/70/71/72 style.
//   - RenewalLoop is unchanged — it already selects on ctx.Done, but
//     the caller at ingress/module.go:119 used context.Background so
//     the loop ran forever. That caller is now plumbed with a
//     Stop-cancellable context.
type ACMEManager struct {
	mu         sync.Mutex
	certStore  *CertStore
	email      string
	staging    bool              // Use Let's Encrypt staging for testing
	challenges map[string]string // token -> keyAuth for HTTP-01
	logger     *slog.Logger

	// dns01 is the optional DNS-01 challenge solver. When nil the
	// manager falls back to the HTTP-01 path in HandleHTTPChallenge.
	// Wildcard certificates and RFC 1918 / private DNS zones can only
	// be issued via DNS-01 because Let's Encrypt cannot reach the
	// validator over HTTP.
	dns01 *DNS01Solver

	// wg tracks fire-and-forget goroutines spawned by GetCertificate so
	// Wait() can drain them on module Stop. Pre-Tier-73 these were
	// completely untracked.
	wg sync.WaitGroup
}

// NewACMEManager creates an ACME certificate manager. A nil logger is
// tolerated and replaced with slog.Default() — the pre-Tier-73 code
// would NPE inside the panic recovery branch on a nil logger.
func NewACMEManager(certStore *CertStore, email string, staging bool, logger *slog.Logger) *ACMEManager {
	if logger == nil {
		logger = slog.Default()
	}
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
		// Default to localhost for direct IP access or missing SNI
		domain = "localhost"
	}

	// Check cache first
	if cert := a.certStore.Get(domain); cert != nil {
		return cert, nil
	}

	// Auto-issue in background for real domains (not localhost). Tracked
	// by wg so Module.Stop can drain in-flight issuance attempts before
	// the parent module is torn down.
	if domain != "localhost" && domain != "127.0.0.1" {
		a.wg.Add(1)
		go a.issueCertificateAsync(domain)
	}

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

// UseDNS01 enables the DNS-01 challenge flow for future issuances.
// Passing nil reverts the manager to HTTP-01 only. Safe to call at
// any time; the switch only affects issuance attempts that start
// after the call returns.
func (a *ACMEManager) UseDNS01(solver *DNS01Solver) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dns01 = solver
}

// dns01Solver returns the currently configured DNS-01 solver, or
// nil if the manager is in HTTP-01 mode. Exposed for tests and for
// the RenewalLoop so it can pick the right challenge type.
func (a *ACMEManager) dns01Solver() *DNS01Solver {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dns01
}

// issueCertificateAsync is the wg-tracked dispatch wrapper around
// issueCertificate. Separated from the caller so the wg bookkeeping
// and panic recovery are in one place.
func (a *ACMEManager) issueCertificateAsync(domain string) {
	defer a.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			a.logger.Error("panic in ACME certificate issuance", "domain", domain, "error", r)
		}
	}()
	a.issueCertificate(domain)
}

// issueCertificate requests a certificate from Let's Encrypt.
// This is a simplified implementation — production would use the full ACME protocol
// via crypto/acme stdlib package.
func (a *ACMEManager) issueCertificate(domain string) {
	// Snapshot the solver under the lock then release it — the
	// DNS-01 solver's Present/CleanUp run network I/O and must not
	// block GetCertificate on unrelated domains.
	a.mu.Lock()
	solver := a.dns01
	a.mu.Unlock()

	a.logger.Info("requesting SSL certificate",
		"domain", domain,
		"staging", a.staging,
		"challenge", challengeTypeFor(solver),
	)

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
	// 3. For HTTP-01: stash token in a.challenges so HandleHTTPChallenge serves it
	//    For DNS-01: call solver.Present(ctx, domain, token, keyAuth) and
	//                defer solver.CleanUp(...) once validation finishes
	// 4. Wait for validation
	// 5. Finalize order and download certificate
	// 6. Store in CertStore

	a.logger.Info("ACME certificate issuance queued", "domain", domain)
}

// challengeTypeFor reports which ACME challenge type the manager
// will use for this issuance. Exposed as a pure helper so the log
// statement above reads clearly and so tests can assert the mode
// switches when UseDNS01 is called.
func challengeTypeFor(solver *DNS01Solver) string {
	if solver != nil {
		return "dns-01"
	}
	return "http-01"
}

// Wait blocks until every in-flight issueCertificate goroutine
// returns. Called by Module.Stop after the RenewalLoop context is
// cancelled so the gateway does not tear down while an issuance is
// still holding locks.
func (a *ACMEManager) Wait() {
	a.wg.Wait()
}

// RenewalLoop checks all certificates daily and renews those expiring within 30 days.
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
	// Would iterate CertStore, check expiry dates,
	// and re-issue certificates expiring within 30 days
}
