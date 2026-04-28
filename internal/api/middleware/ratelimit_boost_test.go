package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestSafeClientIP_NoTrustXFF(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	got := safeClientIP(req, false)
	if got != "192.168.1.100" {
		t.Errorf("got %q, want 192.168.1.100", got)
	}
}

func TestSafeClientIP_TrustXFF_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Real-IP", "203.0.113.50")
	got := safeClientIP(req, true)
	if got != "203.0.113.50" {
		t.Errorf("got %q, want 203.0.113.50", got)
	}
}

func TestSafeClientIP_TrustXFF_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 70.41.3.18")
	got := safeClientIP(req, true)
	if got != "198.51.100.10" {
		t.Errorf("got %q, want 198.51.100.10", got)
	}
}

func TestSafeClientIP_TrustXFF_InvalidHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	req.Header.Set("X-Real-IP", "not-an-ip")
	req.Header.Set("X-Forwarded-For", "also-not-ip")
	got := safeClientIP(req, true)
	if got != "192.168.1.100" {
		t.Errorf("got %q, want 192.168.1.100 (fallback)", got)
	}
}

func TestSafeClientIP_TrustXFF_NoHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	got := safeClientIP(req, true)
	if got != "192.168.1.100" {
		t.Errorf("got %q, want 192.168.1.100 (fallback)", got)
	}
}

func TestSafeClientIP_TrustXFF_PrivateIPFiltered(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.100:54321"
	req.Header.Set("X-Real-IP", "10.0.0.5")
	got := safeClientIP(req, true)
	// 10.0.0.5 is private, so validateIP returns empty, falls back to RemoteAddr
	if got != "192.168.1.100" {
		t.Errorf("got %q, want 192.168.1.100 (private IP filtered)", got)
	}
}
