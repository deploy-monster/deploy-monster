package ingress

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DefaultDNS01PropagationTimeout is how long we wait for a TXT
// record to become visible to public resolvers before giving up on
// a DNS-01 challenge. 2 minutes comfortably accommodates the TTL of
// every mainstream provider (Route53 propagates in ~60s, Cloudflare
// in ~30s, generic SOA-driven updates in ~120s).
const DefaultDNS01PropagationTimeout = 2 * time.Minute

// DefaultDNS01PollInterval is how often the solver re-queries
// resolvers while waiting for the TXT record to become visible.
const DefaultDNS01PollInterval = 5 * time.Second

// DNS01ChallengeRecordName is the fixed sub-label that ACME requires
// for DNS-01 challenges per RFC 8555 §8.4. The solver prepends this
// to the authorised domain to build the record FQDN.
const DNS01ChallengeRecordName = "_acme-challenge"

// DNS01Solver drives the DNS-01 ACME challenge lifecycle: compute
// the TXT value from a key authorization, publish it via a
// core.DNSProvider, wait for it to propagate to public resolvers,
// and clean up afterwards. It is deliberately decoupled from any
// concrete ACME client so it can be plugged into either the
// existing stubbed ACMEManager or a future stdlib x/crypto/acme
// implementation.
//
// A DNS01Solver is safe for concurrent use from multiple goroutines;
// the underlying provider is responsible for serialising its own
// record updates.
type DNS01Solver struct {
	provider core.DNSProvider
	logger   *slog.Logger

	// propagationTimeout caps the wait for TXT record visibility.
	// Zero falls back to DefaultDNS01PropagationTimeout.
	propagationTimeout time.Duration

	// pollInterval is how often we re-query resolvers during the
	// propagation wait. Zero falls back to DefaultDNS01PollInterval.
	pollInterval time.Duration

	// resolver is the DNS client used for propagation checks. Nil
	// uses the default system resolver. Tests swap this with a stub
	// that answers deterministically.
	resolver dnsTXTLookup
}

// dnsTXTLookup is the narrow slice of net.Resolver the solver uses.
// Scoped to a single method so tests don't have to implement the
// entire net.Resolver surface.
type dnsTXTLookup interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// NewDNS01Solver constructs a solver backed by the supplied DNS
// provider. The provider is typically resolved from
// core.Services.DNSProvider at module wire-up time. A nil logger is
// tolerated and replaced with slog.Default() so the solver matches
// the Tier 73 style used elsewhere in this package.
func NewDNS01Solver(provider core.DNSProvider, logger *slog.Logger) *DNS01Solver {
	if logger == nil {
		logger = slog.Default()
	}
	return &DNS01Solver{
		provider:           provider,
		logger:             logger,
		propagationTimeout: DefaultDNS01PropagationTimeout,
		pollInterval:       DefaultDNS01PollInterval,
		resolver:           net.DefaultResolver,
	}
}

// Present publishes the DNS-01 challenge TXT record for the given
// domain. The ACME client supplies the token and keyAuth from its
// challenge object; the solver derives the TXT value per RFC 8555
// §8.4 (base64url of sha256(keyAuth)) and writes it through the
// configured DNS provider. Present returns only after the provider
// has acknowledged the record AND public resolvers confirm its
// visibility — the ACME client can safely call `/challenge` right
// after Present returns because any TTL-driven propagation lag has
// already been absorbed.
func (s *DNS01Solver) Present(ctx context.Context, domain, token, keyAuth string) error {
	if s.provider == nil {
		return fmt.Errorf("dns-01: no DNS provider configured")
	}
	if domain == "" {
		return fmt.Errorf("dns-01: domain is required")
	}
	if keyAuth == "" {
		return fmt.Errorf("dns-01: keyAuth is required")
	}

	fqdn, value := DNS01ChallengeRecord(domain, keyAuth)
	record := core.DNSRecord{
		Type:  "TXT",
		Name:  fqdn,
		Value: value,
		TTL:   60, // keep short so the cleanup after validation is visible fast
	}

	s.logger.Info("dns-01: publishing challenge record",
		"domain", domain,
		"fqdn", fqdn,
		"provider", s.provider.Name(),
	)
	if err := s.provider.CreateRecord(ctx, record); err != nil {
		return fmt.Errorf("dns-01: create TXT %s via %s: %w",
			fqdn, s.provider.Name(), err)
	}

	if err := s.waitPropagation(ctx, fqdn, value); err != nil {
		// Record was created but never became visible. Try to clean
		// up so the zone doesn't accumulate orphaned challenge TXTs,
		// but don't mask the original error — the caller needs to
		// know propagation failed, not that cleanup also failed.
		if cleanupErr := s.provider.DeleteRecord(context.Background(), record); cleanupErr != nil {
			s.logger.Warn("dns-01: propagation failed and cleanup also failed",
				"fqdn", fqdn, "cleanup_error", cleanupErr,
			)
		}
		return err
	}

	s.logger.Info("dns-01: challenge record visible to resolvers",
		"domain", domain, "fqdn", fqdn,
	)
	return nil
}

// CleanUp removes the challenge TXT record. Should be called after
// the ACME server has validated the challenge regardless of
// outcome, because a stale _acme-challenge TXT can interfere with
// subsequent renewals if it contains a value from an old order. The
// caller should pass the SAME keyAuth as the matching Present call
// so the solver can recompute the record value it needs to delete.
func (s *DNS01Solver) CleanUp(ctx context.Context, domain, token, keyAuth string) error {
	if s.provider == nil {
		return fmt.Errorf("dns-01: no DNS provider configured")
	}
	fqdn, value := DNS01ChallengeRecord(domain, keyAuth)
	record := core.DNSRecord{
		Type:  "TXT",
		Name:  fqdn,
		Value: value,
		TTL:   60,
	}
	s.logger.Info("dns-01: cleaning up challenge record",
		"domain", domain, "fqdn", fqdn,
	)
	if err := s.provider.DeleteRecord(ctx, record); err != nil {
		return fmt.Errorf("dns-01: delete TXT %s via %s: %w",
			fqdn, s.provider.Name(), err)
	}
	return nil
}

// waitPropagation polls DNS resolvers until the challenge TXT value
// appears, or the context / propagation timeout fires. The returned
// error distinguishes timeout from context cancellation so the ACME
// client's retry logic can decide whether to back off.
func (s *DNS01Solver) waitPropagation(ctx context.Context, fqdn, wantValue string) error {
	deadline := time.Now().Add(s.propagationTimeout)

	waitCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	for {
		if s.checkTXT(waitCtx, fqdn, wantValue) {
			return nil
		}
		select {
		case <-time.After(s.pollInterval):
		case <-waitCtx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("dns-01: propagation cancelled for %s: %w", fqdn, err)
			}
			return fmt.Errorf("dns-01: TXT %s did not propagate within %v",
				fqdn, s.propagationTimeout)
		}
	}
}

// checkTXT returns true once the resolver sees the expected value.
// Multiple TXT records can exist on the same name (cleanup lag,
// other services) so any match counts.
func (s *DNS01Solver) checkTXT(ctx context.Context, fqdn, wantValue string) bool {
	values, err := s.resolver.LookupTXT(ctx, fqdn)
	if err != nil {
		s.logger.Debug("dns-01: lookup error during propagation",
			"fqdn", fqdn, "error", err,
		)
		return false
	}
	for _, v := range values {
		// Some resolvers (and some providers) wrap TXT content in
		// double quotes on the wire. Strip defensively so a quoted
		// match doesn't look like a miss.
		if strings.Trim(v, `"`) == wantValue {
			return true
		}
	}
	return false
}

// DNS01ChallengeRecord computes the (fqdn, value) pair for a given
// domain + ACME keyAuth per RFC 8555 §8.4:
//
//   - fqdn   = "_acme-challenge." + domain
//   - value  = base64url(sha256(keyAuth)), no padding
//
// Exposed as a package-level helper so callers that already know a
// challenge token (e.g. tests, or an alternative ACME client) can
// reproduce the exact record shape the solver uses.
func DNS01ChallengeRecord(domain, keyAuth string) (fqdn, value string) {
	sum := sha256.Sum256([]byte(keyAuth))
	value = base64.RawURLEncoding.EncodeToString(sum[:])

	domain = strings.TrimSuffix(domain, ".")
	if strings.HasPrefix(domain, DNS01ChallengeRecordName+".") {
		// Caller already prepended the prefix — don't double it.
		fqdn = domain
	} else {
		fqdn = DNS01ChallengeRecordName + "." + domain
	}
	return fqdn, value
}
