package ingress

import (
	"crypto/tls"
	"log/slog"
	"testing"
)

func TestNewACMEManager(t *testing.T) {
	cs := NewCertStore()
	logger := slog.Default()

	am := NewACMEManager(cs, "test@example.com", true, logger)

	if am == nil {
		t.Fatal("expected non-nil ACMEManager")
	}
	if am.certStore != cs {
		t.Error("expected certStore to be set")
	}
	if am.email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got %q", am.email)
	}
	if !am.staging {
		t.Error("expected staging to be true")
	}
	if am.challenges == nil {
		t.Error("expected challenges map to be initialized")
	}
}

func TestACMEManager_HandleHTTPChallenge_NotFound(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	_, ok := am.HandleHTTPChallenge("nonexistent-token")
	if ok {
		t.Error("expected challenge not found")
	}
}

func TestACMEManager_HandleHTTPChallenge_Found(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Manually insert a challenge
	am.mu.Lock()
	am.challenges["test-token"] = "test-key-auth"
	am.mu.Unlock()

	keyAuth, ok := am.HandleHTTPChallenge("test-token")
	if !ok {
		t.Error("expected challenge to be found")
	}
	if keyAuth != "test-key-auth" {
		t.Errorf("expected keyAuth 'test-key-auth', got %q", keyAuth)
	}
}

func TestACMEManager_GetCertificate_CachedCert(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Put a certificate in the store
	cert, err := GenerateSelfSigned("cached.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	cs.Put("cached.example.com", cert)

	hello := &tls.ClientHelloInfo{ServerName: "cached.example.com"}
	got, err := am.GetCertificate(hello)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != cert {
		t.Error("expected cached certificate to be returned")
	}
}

func TestACMEManager_GetCertificate_NoSNI(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	hello := &tls.ClientHelloInfo{ServerName: ""}
	cert, err := am.GetCertificate(hello)
	if err != nil {
		t.Errorf("expected no error for empty SNI, got: %v", err)
	}
	if cert == nil {
		t.Error("expected self-signed localhost certificate")
	}
}

func TestACMEManager_GetCertificate_SelfSignedFallback(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Request cert for domain not in store - should return self-signed
	hello := &tls.ClientHelloInfo{ServerName: "new.example.com"}
	got, err := am.GetCertificate(hello)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected self-signed certificate")
	}
	if len(got.Certificate) == 0 {
		t.Error("expected certificate DER bytes")
	}
}

func TestACMEManager_checkRenewals(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Just verify it doesn't panic
	am.checkRenewals()
}

func TestACMEManager_issueCertificate(t *testing.T) {
	cs := NewCertStore()
	am := NewACMEManager(cs, "test@example.com", true, slog.Default())

	// Just verify it doesn't panic
	am.issueCertificate("test.example.com")
}
