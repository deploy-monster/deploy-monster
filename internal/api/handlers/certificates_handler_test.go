package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── List Certificates ───────────────────────────────────────────────────────

func TestCertificateList_Success(t *testing.T) {
	store := newMockStore()
	handler := NewCertificateHandler(store, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/certificates", nil)
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
	rr := httptest.NewRecorder()

	handler.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
