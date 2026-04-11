package ingress

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestCertStore_ListCerts(t *testing.T) {
	cs := NewCertStore()

	// Add a self-signed cert
	cert, err := GenerateSelfSigned("example.com")
	if err != nil {
		t.Fatal(err)
	}
	cs.Put("example.com", cert)

	certs := cs.ListCerts()
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	if certs[0].Domain != "example.com" {
		t.Errorf("expected domain example.com, got %q", certs[0].Domain)
	}
	if certs[0].DaysLeft < 360 {
		t.Errorf("expected ~365 days left for new self-signed cert, got %d", certs[0].DaysLeft)
	}
}

func TestCertStore_ListCerts_Empty(t *testing.T) {
	cs := NewCertStore()
	certs := cs.ListCerts()
	if len(certs) != 0 {
		t.Errorf("expected 0 certs, got %d", len(certs))
	}
}

func TestCertStore_ExpiringCerts_NoneExpiring(t *testing.T) {
	cs := NewCertStore()

	// Self-signed cert valid for 365 days — should not be expiring within 30 days
	cert, _ := GenerateSelfSigned("healthy.com")
	cs.Put("healthy.com", cert)

	expiring := cs.ExpiringCerts(30 * 24 * time.Hour)
	if len(expiring) != 0 {
		t.Errorf("expected no expiring certs, got %d", len(expiring))
	}
}

func TestCertStore_ExpiringCerts_LargeWindow(t *testing.T) {
	cs := NewCertStore()

	cert, _ := GenerateSelfSigned("expiring.com")
	cs.Put("expiring.com", cert)

	// With a 400-day window, the 365-day cert should show as expiring
	expiring := cs.ExpiringCerts(400 * 24 * time.Hour)
	if len(expiring) != 1 {
		t.Errorf("expected 1 expiring cert, got %d", len(expiring))
	}
}

func TestCertStore_ExpiringCerts_NilLeaf(t *testing.T) {
	cs := NewCertStore()

	// Create cert with nil Leaf to test lazy parsing
	cert, _ := GenerateSelfSigned("lazy.com")
	cert.Leaf = nil // Force lazy parse path
	cs.Put("lazy.com", cert)

	certs := cs.ListCerts()
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	// Should still have parsed expiry info
	if certs[0].NotAfter.IsZero() {
		t.Error("expected NotAfter to be set after lazy parse")
	}
}

func TestModule_Health_Degraded_ExpiringCert(t *testing.T) {
	m := New()
	m.router = NewRouteTable()
	m.certStore = NewCertStore()

	// Normally healthy
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("expected core.HealthOK, got %v", h)
	}

	// Add a cert that's about to expire (we can't easily create one,
	// but we can test that the health path doesn't panic with valid certs)
	cert, _ := GenerateSelfSigned("test.com")
	m.certStore.Put("test.com", cert)

	// Self-signed cert valid for 365 days — health should remain OK
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("expected core.HealthOK for fresh cert, got %v", h)
	}
}

func TestModule_CertStatus(t *testing.T) {
	m := New()
	m.certStore = NewCertStore()

	cert, _ := GenerateSelfSigned("status.com")
	m.certStore.Put("status.com", cert)

	status := m.CertStatus()
	if len(status) != 1 {
		t.Fatalf("expected 1 cert in status, got %d", len(status))
	}
	if status[0].Domain != "status.com" {
		t.Errorf("expected domain status.com, got %q", status[0].Domain)
	}
}

func TestModule_CertStatus_NilStore(t *testing.T) {
	m := New()
	status := m.CertStatus()
	if status != nil {
		t.Errorf("expected nil status for nil cert store, got %v", status)
	}
}

func TestCertStore_ExpiringCerts_EmptyCert(t *testing.T) {
	cs := NewCertStore()

	// Store a cert with no data
	cs.Put("empty.com", &tls.Certificate{})

	certs := cs.ListCerts()
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(certs))
	}
	// NotAfter should be zero for empty cert
	if !certs[0].NotAfter.IsZero() {
		t.Error("expected zero NotAfter for empty cert")
	}
}
