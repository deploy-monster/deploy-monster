package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// rewriteTransport rewrites requests to hit the test server instead of the real Cloudflare API.
type rewriteTransport struct {
	base string
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(rt.base, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

// failTransport always returns an error.
type failTransport struct{}

func (ft *failTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated network failure")
}

// ===========================================================================
// Cloudflare tests
// ===========================================================================

func TestNewCloudflare(t *testing.T) {
	cf := NewCloudflare("test-token-abc")
	if cf == nil {
		t.Fatal("NewCloudflare returned nil")
	}
	if cf.token != "test-token-abc" {
		t.Errorf("token = %q, want %q", cf.token, "test-token-abc")
	}
	if cf.client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestCloudflare_Name(t *testing.T) {
	cf := NewCloudflare("token")
	if cf.Name() != "cloudflare" {
		t.Errorf("Name() = %q, want %q", cf.Name(), "cloudflare")
	}
}

func TestCloudflare_CreateRecord_Success(t *testing.T) {
	// Mock server: /zones returns a zone, then POST /zones/{id}/dns_records succeeds
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected auth header 'Bearer test-token', got %q", auth)
		}

		if strings.Contains(r.URL.Path, "/zones") && r.URL.RawQuery != "" {
			// findZone call
			json.NewEncoder(w).Encode(map[string]any{
				"result": []map[string]any{
					{"id": "zone-123", "name": "example.com"},
				},
			})
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/dns_records") {
			// CreateRecord call
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			json.Unmarshal(body, &payload)
			if payload["name"] != "app.example.com" {
				t.Errorf("expected record name 'app.example.com', got %v", payload["name"])
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	// Override the API URL by using a custom do method
	// Since cfAPI is a const, we override by making the test server URL match
	// We need to inject the server URL into requests — use a transport wrapper
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.CreateRecord(context.Background(), core.DNSRecord{
		Type:    "A",
		Name:    "app.example.com",
		Value:   "1.2.3.4",
		TTL:     300,
		Proxied: false,
	})
	if err != nil {
		t.Fatalf("CreateRecord() error: %v", err)
	}
}

func TestCloudflare_CreateRecord_ZoneNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty result for zones
		json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{},
		})
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "app.nowhere.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err == nil {
		t.Fatal("expected error for zone not found")
	}
	if !strings.Contains(err.Error(), "no Cloudflare zone found") {
		t.Errorf("expected 'no Cloudflare zone found' error, got: %v", err)
	}
}

func TestCloudflare_CreateRecord_APIError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// findZone — return a zone
			json.NewEncoder(w).Encode(map[string]any{
				"result": []map[string]any{
					{"id": "zone-123", "name": "example.com"},
				},
			})
			return
		}
		// CreateRecord call — return 400 error
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errors":[{"message":"invalid record"}]}`))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "bad.example.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("expected HTTP 400 error, got: %v", err)
	}
}

func TestCloudflare_UpdateRecord_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "per_page") {
			json.NewEncoder(w).Encode(map[string]any{
				"result": []map[string]any{
					{"id": "zone-123", "name": "example.com"},
				},
			})
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/dns_records/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.UpdateRecord(context.Background(), core.DNSRecord{
		ID:      "rec-456",
		Type:    "A",
		Name:    "app.example.com",
		Value:   "5.6.7.8",
		TTL:     600,
		Proxied: true,
	})
	if err != nil {
		t.Fatalf("UpdateRecord() error: %v", err)
	}
}

func TestCloudflare_UpdateRecord_ZoneNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{},
		})
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.UpdateRecord(context.Background(), core.DNSRecord{
		ID:    "rec-1",
		Type:  "A",
		Name:  "app.nowhere.com",
		Value: "1.2.3.4",
	})
	if err == nil {
		t.Fatal("expected error for zone not found")
	}
}

func TestCloudflare_DeleteRecord(t *testing.T) {
	cf := NewCloudflare("test-token")
	err := cf.DeleteRecord(context.Background(), core.DNSRecord{ID: "rec-123", Name: "nonexistent.example.com"})
	if err == nil {
		t.Fatal("expected error from DeleteRecord (zone not found)")
	}
}

func TestCloudflare_DeleteRecord_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "per_page") {
			json.NewEncoder(w).Encode(map[string]any{
				"result": []map[string]any{
					{"id": "zone-123", "name": "example.com"},
				},
			})
			return
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/dns_records/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	err := cf.DeleteRecord(context.Background(), core.DNSRecord{
		ID:   "rec-456",
		Name: "app.example.com",
	})
	if err != nil {
		t.Fatalf("DeleteRecord() error: %v", err)
	}
}

func TestCloudflare_Verify(t *testing.T) {
	cf := NewCloudflare("test-token")
	ok, err := cf.Verify(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if !ok {
		t.Error("Verify() should return true for resolvable domain")
	}
	// Non-existent domain should return false
	ok2, _ := cf.Verify(context.Background(), "this-should-not-exist-12345.invalid")
	if ok2 {
		t.Error("Verify() should return false for non-existent domain")
	}
}

func TestCloudflare_findZone_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{
				{"id": "zone-aaa", "name": "other.com"},
				{"id": "zone-bbb", "name": "example.com"},
			},
		})
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	zoneID, err := cf.findZone(context.Background(), "sub.example.com")
	if err != nil {
		t.Fatalf("findZone() error: %v", err)
	}
	if zoneID != "zone-bbb" {
		t.Errorf("findZone() = %q, want %q", zoneID, "zone-bbb")
	}
}

func TestCloudflare_findZone_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"result": []map[string]any{
				{"id": "zone-aaa", "name": "other.com"},
			},
		})
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	_, err := cf.findZone(context.Background(), "app.nomatch.com")
	if err == nil {
		t.Fatal("expected error when no zone matches")
	}
	if !strings.Contains(err.Error(), "no Cloudflare zone found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCloudflare_findZone_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	_, err := cf.findZone(context.Background(), "app.example.com")
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected HTTP 500 error, got: %v", err)
	}
}

func TestCloudflare_do_NilPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(body) > 0 {
				t.Errorf("expected nil body for nil payload, got %d bytes", len(body))
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cf := NewCloudflare("test-token")
	cf.client = srv.Client()
	cf.client.Transport = &rewriteTransport{base: srv.URL}

	_, err := cf.do(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("do() with nil payload error: %v", err)
	}
}

func TestCloudflare_do_NetworkError(t *testing.T) {
	cf := NewCloudflare("test-token")
	// Use a transport that always fails
	cf.client.Transport = &failTransport{}

	_, err := cf.do(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "cloudflare API") {
		t.Errorf("expected 'cloudflare API' error prefix, got: %v", err)
	}
}

func TestCloudflare_ImplementsDNSProvider(t *testing.T) {
	var _ core.DNSProvider = (*Cloudflare)(nil)
}

// ===========================================================================
