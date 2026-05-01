package handlers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

// ─── List Certificates ───────────────────────────────────────────────────────

func TestCertificateList_Success(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/certificates", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total, ok := resp["total"].(float64)
	if !ok || int(total) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
}

// ─── Upload Certificate ──────────────────────────────────────────────────────

func TestCertificateUpload_InvalidCertKeyPair(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body, _ := json.Marshal(uploadCertRequest{
		DomainID: "domain1",
		CertPEM:  "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----",
		KeyPEM:   "-----BEGIN PRIVATE KEY-----\nMIIE...\n-----END PRIVATE KEY-----",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	// Now validates cert/key pair — dummy PEM data fails validation
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCertificateUpload_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader([]byte("bad")))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestCertificateUpload_MissingDomainID(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body, _ := json.Marshal(uploadCertRequest{
		CertPEM: "cert",
		KeyPEM:  "key",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "domain_id, cert_pem, and key_pem are required")
}

func TestCertificateUpload_MissingCertPEM(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body, _ := json.Marshal(uploadCertRequest{
		DomainID: "domain1",
		KeyPEM:   "key",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "domain_id, cert_pem, and key_pem are required")
}

func TestCertificateUpload_MissingKeyPEM(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body, _ := json.Marshal(uploadCertRequest{
		DomainID: "domain1",
		CertPEM:  "cert",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "domain_id, cert_pem, and key_pem are required")
}

func TestCertificateUpload_AllFieldsMissing(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body, _ := json.Marshal(uploadCertRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

// testCertForDomain generates a self-signed certificate for the given domain.
func testCertForDomain(domain string) (certPEM, keyPEM string) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:      []string{domain},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return string(certBuf), string(keyBuf)
}

func TestCertificateUpload_DomainMismatch(t *testing.T) {
	// Upload a cert for example.com but claim domain_id = evil.com
	cert, key := testCertForDomain("example.com")
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	body := map[string]string{
		"domain_id": "evil.com",
		"cert_pem":  cert,
		"key_pem":   key,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(bodyBytes))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for domain mismatch, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "certificate does not match domain: evil.com")
}

func TestCertificateUpload_WildcardCertMatchesSubdomain(t *testing.T) {
	// Wildcard cert *.example.com should match app.example.com
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "*.example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"*.example.com"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	body := map[string]string{
		"domain_id": "app.example.com",
		"cert_pem":  cert,
		"key_pem":   keyPEM,
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/certificates", bytes.NewReader(bodyBytes))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for wildcard match, got %d: %s", rr.Code, rr.Body.String())
	}
}
