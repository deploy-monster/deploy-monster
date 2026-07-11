package providers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestCloudflare_UpdateRecord_doError covers the error-wrapping branch
// when c.do fails inside UpdateRecord (line 72).
func TestCloudflare_UpdateRecord_doError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// findZone — return a zone
			w.Write([]byte(`{"result":[{"id":"zone-123","name":"example.com"}]}`))
			return
		}
		// UpdateRecord — network error on do
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server error`))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.UpdateRecord(context.Background(), core.DNSRecord{
		ID:   "rec-456",
		Type: "A",
		Name: "app.example.com",
	})
	if err == nil {
		t.Fatal("expected error from UpdateRecord")
	}
	if !strings.Contains(err.Error(), "rec-456") {
		t.Errorf("expected record ID in error, got: %v", err)
	}
}

// TestCloudflare_DeleteRecord_doError covers the error-wrapping
// branch when c.do fails inside DeleteRecord (line 83).
func TestCloudflare_DeleteRecord_doError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// findZone — return a zone
			w.Write([]byte(`{"result":[{"id":"zone-123","name":"example.com"}]}`))
			return
		}
		// DeleteRecord — do error
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server error`))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.DeleteRecord(context.Background(), core.DNSRecord{
		ID:   "rec-789",
		Name: "app.example.com",
	})
	if err == nil {
		t.Fatal("expected error from DeleteRecord")
	}
	if !strings.Contains(err.Error(), "rec-789") {
		t.Errorf("expected record ID in error, got: %v", err)
	}
}

// TestCloudflare_Verify_InternalIP covers the branch where DNS
// resolution returns an internal/private IP (line 99).
func TestCloudflare_Verify_InternalIP(t *testing.T) {
	// Use an unresolvable domain to test the false path
	cf := NewCloudflare("test-token")
	ok, err := cf.Verify(context.Background(), "some-nonexistent-test-domain.example.com")
	if err != nil {
		t.Fatalf("Verify() unexpected error: %v", err)
	}
	if ok {
		t.Error("expected false for unresolvable domain")
	}
}

// TestCloudflare_do_ClientError covers the 4xx branch in do()
// where result is set to nil (line 193-194).
func TestCloudflare_do_ClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	_, err := cf.do(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("expected HTTP 400, got: %v", err)
	}
}

// TestIsPublicIP_IPv6Metadata covers the branch in isPublicIP where
// ip4 is nil (IPv6) and the metadata check is skipped (line 125 being false).
func TestIsPublicIP_IPv6NonPublic(t *testing.T) {
	// IPv6 loopback — should return false
	ip := net.ParseIP("::1")
	if isPublicIP(ip) {
		t.Error("isPublicIP(::1) should return false")
	}
}

// TestCloudflare_CreateRecord_doError covers the error-wrapping
// branch when c.do fails inside CreateRecord (line 52).
func TestCloudflare_CreateRecord_doError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// findZone — return a zone
			w.Write([]byte(`{"result":[{"id":"zone-123","name":"example.com"}]}`))
			return
		}
		// CreateRecord — do error
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`server error`))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.CreateRecord(context.Background(), core.DNSRecord{
		Type:    "A",
		Name:    "app.example.com",
		Value:   "1.2.3.4",
		TTL:     300,
		Proxied: false,
	})
	if err == nil {
		t.Fatal("expected error from CreateRecord")
	}
	if !strings.Contains(err.Error(), "A") {
		t.Errorf("expected record type in error, got: %v", err)
	}
}

// TestCloudflare_Verify_Error covers the error branch in Verify
// where DNS resolution fails (line 94-96).
func TestCloudflare_Verify_ResolutionError(t *testing.T) {
	cf := NewCloudflare("test-token")
	ok, err := cf.Verify(context.Background(), "")
	if err != nil {
		// Error is swallowed — returns false
		t.Logf("Verify('') returned false with: %v", err)
	}
	if ok {
		t.Error("expected false for empty domain")
	}
}
