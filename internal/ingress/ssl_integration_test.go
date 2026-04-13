//go:build integration

package ingress

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSSLIntegration_SelfSignedCert verifies the full flow:
// domain → generate self-signed cert → store → serve over TLS.
func TestSSLIntegration_SelfSignedCert(t *testing.T) {
	domain := "test.deploy.monster"

	// Step 1: Generate self-signed certificate
	cert, err := GenerateSelfSigned(domain)
	if err != nil {
		t.Fatalf("GenerateSelfSigned(%q): %v", domain, err)
	}

	// Step 2: Store certificate
	store := NewCertStore()
	store.Put(domain, cert)

	// Step 3: Retrieve certificate
	retrieved := store.Get(domain)
	if retrieved == nil {
		t.Fatal("expected certificate in store")
	}

	// Step 4: Verify certificate details
	leaf, err := x509.ParseCertificate(retrieved.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	if err := leaf.VerifyHostname(domain); err != nil {
		t.Errorf("certificate should be valid for %q: %v", domain, err)
	}

	// Step 5: Use certificate in TLS server
	tlsCfg := &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return store.GetCertificate(hello)
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("secure"))
	})

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = tlsCfg
	srv.StartTLS()
	defer srv.Close()

	// Step 6: Connect with TLS client that skips verification (self-signed)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("TLS request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.TLS == nil {
		t.Error("expected TLS connection")
	}
	if resp.TLS.HandshakeComplete == false {
		t.Error("expected completed TLS handshake")
	}
}

// TestSSLIntegration_CertStoreCallback verifies GetCertificate callback
// works correctly as tls.Config.GetCertificate.
func TestSSLIntegration_CertStoreCallback(t *testing.T) {
	domains := []string{"app1.example.com", "app2.example.com", "app3.example.com"}
	store := NewCertStore()

	// Store certs for all domains
	for _, domain := range domains {
		cert, err := GenerateSelfSigned(domain)
		if err != nil {
			t.Fatalf("GenerateSelfSigned(%q): %v", domain, err)
		}
		store.Put(domain, cert)
	}

	// Verify each domain gets correct cert
	for _, domain := range domains {
		hello := &tls.ClientHelloInfo{ServerName: domain}
		cert, err := store.GetCertificate(hello)
		if err != nil {
			t.Errorf("GetCertificate(%q): %v", domain, err)
			continue
		}
		if cert == nil {
			t.Errorf("expected certificate for %q", domain)
			continue
		}

		leaf, _ := x509.ParseCertificate(cert.Certificate[0])
		if err := leaf.VerifyHostname(domain); err != nil {
			t.Errorf("cert for %q doesn't match: %v", domain, err)
		}
	}

	// Unknown domain should return error or nil
	hello := &tls.ClientHelloInfo{ServerName: "unknown.example.com"}
	cert, _ := store.GetCertificate(hello)
	// Some implementations return nil cert, some return error — both are acceptable
	if cert != nil {
		// If a cert is returned, verify it doesn't match unknown domain
		leaf, _ := x509.ParseCertificate(cert.Certificate[0])
		if leaf.VerifyHostname("unknown.example.com") == nil {
			t.Error("cert should NOT be valid for unknown domain")
		}
	}
}

// TestSSLIntegration_MultipleDomainsCertRotation verifies cert replacement works.
func TestSSLIntegration_MultipleDomainsCertRotation(t *testing.T) {
	store := NewCertStore()
	domain := "rotate.example.com"

	// Initial cert
	cert1, _ := GenerateSelfSigned(domain)
	store.Put(domain, cert1)

	// Get serial of first cert
	leaf1, _ := x509.ParseCertificate(cert1.Certificate[0])
	serial1 := leaf1.SerialNumber

	// Replace with new cert
	cert2, _ := GenerateSelfSigned(domain)
	store.Put(domain, cert2)

	// Verify new cert is served
	retrieved := store.Get(domain)
	leaf2, _ := x509.ParseCertificate(retrieved.Certificate[0])
	serial2 := leaf2.SerialNumber

	if serial1.Cmp(serial2) == 0 {
		t.Error("expected different certificate after rotation")
	}
}
