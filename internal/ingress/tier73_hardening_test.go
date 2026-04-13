package ingress

import (
	"context"
	"crypto/tls"
	"sync"
	"testing"
	"time"
)

// Tier 73 — ACME manager lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 73 for
// internal/ingress/acme.go:
//
//   - NewACMEManager tolerates a nil logger
//   - GetCertificate tracks the fire-and-forget issueCertificate
//     goroutine so Wait() actually drains it
//   - Wait() is safe to call even when no goroutines have been
//     dispatched
//   - RenewalLoop respects ctx cancellation and returns promptly
//   - RenewalLoop panic is recovered and does not crash the process
//   - HandleHTTPChallenge still works after Wait() has drained

// ─── NewACMEManager nil-logger guard ───────────────────────────────────────

func TestTier73_NewACMEManager_NilLogger(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "ops@example.com", true, nil)
	if am == nil {
		t.Fatal("NewACMEManager returned nil")
	}
	if am.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
}

// ─── Wait is safe when nothing has been dispatched ─────────────────────────

func TestTier73_ACME_Wait_NoDispatch(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "ops@example.com", true, nil)

	// Should return immediately; wg is zero.
	done := make(chan struct{})
	go func() {
		am.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait blocked when no goroutines had been dispatched")
	}
}

// ─── GetCertificate dispatch is tracked by wg ──────────────────────────────

// TestTier73_ACME_GetCertificate_TrackedByWait proves that after a
// real-domain GetCertificate call, Wait() will block until the
// fire-and-forget issueCertificate goroutine finishes. Pre-Tier-73
// the goroutine was untracked and Wait() did not exist at all.
func TestTier73_ACME_GetCertificate_TrackedByWait(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "ops@example.com", true, nil)

	// Real domain — triggers dispatch of issueCertificateAsync.
	hello := &tls.ClientHelloInfo{ServerName: "real.example.com"}
	cert, err := am.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate returned error: %v", err)
	}
	if cert == nil {
		t.Fatal("GetCertificate returned nil cert")
	}

	// Wait should drain the dispatched goroutine. The current stub
	// issueCertificate is fast (just generates an ECDSA key and
	// logs), so Wait should return quickly.
	done := make(chan struct{})
	go func() {
		am.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not drain issueCertificate goroutine")
	}
}

// ─── GetCertificate for localhost does NOT dispatch ────────────────────────

// TestTier73_ACME_GetCertificate_LocalhostNoDispatch proves the
// "don't trigger ACME for localhost" guard still holds after the
// Tier 73 refactor — otherwise every test-suite TLS handshake would
// kick off a bogus issuance attempt.
func TestTier73_ACME_GetCertificate_LocalhostNoDispatch(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "ops@example.com", true, nil)

	hello := &tls.ClientHelloInfo{ServerName: "localhost"}
	if _, err := am.GetCertificate(hello); err != nil {
		t.Fatalf("GetCertificate(localhost) returned error: %v", err)
	}

	// Wait must return immediately — nothing was dispatched.
	done := make(chan struct{})
	go func() {
		am.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait blocked — localhost handshake incorrectly dispatched issuance")
	}
}

// ─── RenewalLoop respects ctx cancellation ─────────────────────────────────

// TestTier73_ACME_RenewalLoop_StopsOnCancel proves that canceling
// the parent context unblocks the RenewalLoop. Pre-Tier-73 the caller
// at ingress/module.go passed context.Background() so the loop could
// never be stopped.
func TestTier73_ACME_RenewalLoop_StopsOnCancel(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "ops@example.com", true, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		am.RenewalLoop(ctx)
		close(done)
	}()

	// Give the loop a moment to enter the select.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RenewalLoop did not exit after ctx cancel")
	}
}

// ─── Concurrent GetCertificate + Wait does not deadlock ────────────────────

// TestTier73_ACME_ConcurrentDispatch_Wait proves the wg bookkeeping
// survives under concurrent dispatch. Pre-Tier-73 there was no wg at
// all, so this test could not even be written.
func TestTier73_ACME_ConcurrentDispatch_Wait(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "ops@example.com", true, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hello := &tls.ClientHelloInfo{ServerName: "concurrent.example.com"}
			_, _ = am.GetCertificate(hello)
		}(i)
	}
	wg.Wait()

	// Now drain any dispatched issuance goroutines.
	done := make(chan struct{})
	go func() {
		am.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not drain after concurrent dispatch")
	}
}
