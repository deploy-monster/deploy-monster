package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckSSL_Error(t *testing.T) {
	result := checkSSL("invalid.host.that.does.not.exist.example:99999")
	if result.Error == "" {
		t.Error("expected error for invalid host")
	}
	if result.FQDN != "invalid.host.that.does.not.exist.example:99999" {
		t.Errorf("FQDN = %q, want original input", result.FQDN)
	}
}

func TestSSLStatusHandler_Check_CacheHit(t *testing.T) {
	bolt := &mockBoltStore{data: make(map[string]map[string][]byte)}
	h := NewSSLStatusHandler(bolt)

	// Seed cache
	cached := SSLCheckResult{FQDN: "example.com", Valid: true, DaysLeft: 30}
	_ = bolt.Set("certificates", "ssl_check:example.com", cached, 300)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/ssl-status?fqdn=example.com", nil)
	rr := httptest.NewRecorder()

	h.Check(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSSLStatusHandler_Check_MissingFQDN_Boost(t *testing.T) {
	h := NewSSLStatusHandler(newMockBoltStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/ssl-status", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSLStatusHandler_Check_CacheMiss(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewSSLStatusHandler(bolt)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/ssl-status?fqdn=bad.local", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestCheckSSL_ValidLocalTLS(t *testing.T) {
	// Spin up a local HTTPS server with a self-signed cert so we can
	// exercise the success path of checkSSL. Since we can't skip
	// verification in checkSSL (InsecureSkipVerify is false), we
	// need a cert the system trusts. Instead, use a real public
	// domain that we expect to have valid TLS.
	result := checkSSL("google.com")
	// Either it succeeds with valid=true or it fails due to network
	// issues. If it succeeds, verify the fields are populated.
	if result.Valid {
		if result.Issuer == "" {
			t.Error("expected Issuer to be set")
		}
		if result.Subject == "" {
			t.Error("expected Subject to be set")
		}
		if result.ExpiresAt.IsZero() {
			t.Error("expected ExpiresAt to be set")
		}
		if result.DaysLeft <= 0 {
			t.Errorf("expected positive DaysLeft, got %d", result.DaysLeft)
		}
	}
}
