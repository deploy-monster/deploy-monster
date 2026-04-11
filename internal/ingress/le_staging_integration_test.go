//go:build integration
// +build integration

// Let's Encrypt staging smoke test.
//
// Gated on the `LE_STAGING_TEST` environment variable so CI runs only
// opt-in — Let's Encrypt staging enforces a 5/hour new-account limit per
// IP, and we do not want every push to burn through it.
//
// What it catches:
//   - Outbound HTTPS from the CI runner is blocked or intercepted.
//   - The CA bundle on the runner is missing the ISRG root.
//   - Let's Encrypt staging moved the directory endpoint.
//   - `golang.org/x/crypto/acme` wire format drifts against RFC 8555.
//
// It does NOT exercise the full `ACMEManager.issueCertificate` flow —
// that path is stubbed pending a real crypto/acme integration (Phase 5+).
// Instead it drives the `acme.Client` directly against the LE staging
// directory and asserts we can Discover + Register + NewOrder, which is
// the entire HTTPS + JOSE + account path.

package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/acme"
)

// letsEncryptStagingURL is the v2 staging directory. Keep in sync with
// https://letsencrypt.org/docs/staging-environment/ — the production URL
// is deliberately NOT used here; a real cert issuance against production
// would burn one of our 50/week certificates-per-domain quota slots.
const letsEncryptStagingURL = "https://acme-staging-v02.api.letsencrypt.org/directory"

func TestLetsEncryptStaging_DiscoverAndRegister(t *testing.T) {
	if os.Getenv("LE_STAGING_TEST") != "1" {
		t.Skip("LE_STAGING_TEST not set; skipping Let's Encrypt staging integration test")
	}

	// Generate a fresh ECDSA account key. LE staging accepts both RSA and
	// ECDSA; P-256 keeps the handshake small and the generation fast.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa GenerateKey: %v", err)
	}

	client := &acme.Client{
		Key:          key,
		DirectoryURL: letsEncryptStagingURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// ---- Discover --------------------------------------------------------
	dir, err := client.Discover(ctx)
	if err != nil {
		t.Fatalf("acme Discover: %v", err)
	}
	if dir.RegURL == "" {
		t.Error("directory missing RegURL")
	}
	if dir.OrderURL == "" {
		t.Error("directory missing OrderURL")
	}
	if !strings.Contains(dir.RegURL, "staging") {
		t.Errorf("directory RegURL should point at staging, got %q", dir.RegURL)
	}

	// ---- Register --------------------------------------------------------
	//
	// A fresh account every run is fine on staging: their rate limit is
	// 50 new-account registrations per 3 hours per IP.
	acct := &acme.Account{
		Contact: []string{"mailto:integration-test@deploy.monster"},
	}
	registered, err := client.Register(ctx, acct, acme.AcceptTOS)
	if err != nil {
		// If the runner IP has already been rate-limited we still want the
		// test to pass (we have proven the endpoint is reachable) — only
		// fail on non-rate-limit errors.
		if strings.Contains(err.Error(), "rateLimited") ||
			strings.Contains(err.Error(), "too many") {
			t.Skipf("LE staging rate-limited this runner: %v", err)
		}
		t.Fatalf("acme Register: %v", err)
	}
	if registered.URI == "" {
		t.Error("registered account missing URI")
	}
	if registered.Status != "" && registered.Status != "valid" {
		t.Errorf("registered account status = %q, want valid or empty", registered.Status)
	}

	// ---- NewOrder --------------------------------------------------------
	//
	// Use a throwaway hostname under a domain we control so the CA can
	// point its validator at the (non-existent) challenge without polluting
	// any real FQDN's cert history. The order will be "pending" — we do
	// NOT try to complete the HTTP-01 or DNS-01 challenge; we only verify
	// the wire format for creating orders.
	order, err := client.AuthorizeOrder(ctx, []acme.AuthzID{
		{Type: "dns", Value: "le-staging-smoke.integration.deploy.monster"},
	})
	if err != nil {
		if strings.Contains(err.Error(), "rateLimited") ||
			strings.Contains(err.Error(), "too many") {
			t.Skipf("LE staging rate-limited this runner on AuthorizeOrder: %v", err)
		}
		t.Fatalf("acme AuthorizeOrder: %v", err)
	}
	if order.URI == "" {
		t.Error("order missing URI")
	}
	if order.Status != acme.StatusPending && order.Status != acme.StatusReady {
		t.Errorf("order status = %q, want pending or ready", order.Status)
	}
	if len(order.AuthzURLs) == 0 {
		t.Error("order has no authorization URLs")
	}
}
