package providers

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
	err := cf.DeleteRecord(context.Background(), "rec-123")
	if err == nil {
		t.Fatal("expected error from DeleteRecord (stub)")
	}
	if !strings.Contains(err.Error(), "delete requires zone context") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCloudflare_Verify(t *testing.T) {
	cf := NewCloudflare("test-token")
	ok, err := cf.Verify(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if !ok {
		t.Error("Verify() should return true")
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
// Route53 tests
// ===========================================================================

func TestNewRoute53(t *testing.T) {
	r := NewRoute53("ak", "sk", "us-east-1")
	if r == nil {
		t.Fatal("NewRoute53 returned nil")
	}
	if r.accessKey != "ak" {
		t.Errorf("accessKey = %q, want %q", r.accessKey, "ak")
	}
	if r.secretKey != "sk" {
		t.Errorf("secretKey = %q, want %q", r.secretKey, "sk")
	}
	if r.region != "us-east-1" {
		t.Errorf("region = %q, want %q", r.region, "us-east-1")
	}
	if r.client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestRoute53_Name(t *testing.T) {
	r := NewRoute53("ak", "sk", "eu-west-1")
	if r.Name() != "route53" {
		t.Errorf("Name() = %q, want %q", r.Name(), "route53")
	}
}

func TestRoute53_CreateRecord_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/hostedzone") && !strings.Contains(r.URL.Path, "/rrset") {
			// findHostedZone
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z123</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/rrset") {
			// changeRecord
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "CREATE") {
				t.Errorf("expected CREATE action in XML body")
			}
			if !strings.Contains(string(body), "app.example.com") {
				t.Errorf("expected record name in XML body")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "app.example.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err != nil {
		t.Fatalf("CreateRecord() error: %v", err)
	}
}

func TestRoute53_CreateRecord_ZoneNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones></HostedZones>
</ListHostedZonesResponse>`)
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "app.nowhere.com",
		Value: "1.2.3.4",
	})
	if err == nil {
		t.Fatal("expected error for zone not found")
	}
	if !strings.Contains(err.Error(), "no Route53 hosted zone found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRoute53_UpdateRecord_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z456</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
			return
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "UPSERT") {
				t.Errorf("expected UPSERT action for update, got body: %s", string(body))
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.UpdateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "app.example.com",
		Value: "5.6.7.8",
		TTL:   600,
	})
	if err != nil {
		t.Fatalf("UpdateRecord() error: %v", err)
	}
}

func TestRoute53_UpdateRecord_ZeroTTL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z789</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
			return
		}
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			// Should have TTL 300 (default)
			if !strings.Contains(string(body), "<TTL>300</TTL>") {
				t.Errorf("expected default TTL 300, got body: %s", string(body))
			}
			w.WriteHeader(http.StatusOK)
			return
		}
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.UpdateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "app.example.com",
		Value: "5.6.7.8",
		TTL:   0, // Should default to 300
	})
	if err != nil {
		t.Fatalf("UpdateRecord() with TTL=0 error: %v", err)
	}
}

func TestRoute53_DeleteRecord(t *testing.T) {
	r := NewRoute53("ak", "sk", "us-east-1")
	err := r.DeleteRecord(context.Background(), "rec-123")
	if err == nil {
		t.Fatal("expected error from DeleteRecord (stub)")
	}
	if !strings.Contains(err.Error(), "Route53 delete requires full record context") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRoute53_Verify(t *testing.T) {
	r := NewRoute53("ak", "sk", "us-east-1")
	ok, err := r.Verify(context.Background(), "app.example.com")
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if !ok {
		t.Error("Verify() should return true")
	}
}

func TestRoute53_changeRecord_HTTPError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// findHostedZone
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z111</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
			return
		}
		// changeRecord — return 400
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`<ErrorResponse><Error><Code>InvalidInput</Code></Error></ErrorResponse>`))
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "bad.example.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("expected HTTP 400 error, got: %v", err)
	}
}

func TestRoute53_changeRecord_NetworkError(t *testing.T) {
	// findHostedZone succeeds, changeRecord fails with network error
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z222</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
			return
		}
		// Close connection to simulate network error
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "fail.example.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestRoute53_findHostedZone_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z-AAA</Id>
      <Name>other.com</Name>
    </HostedZone>
    <HostedZone>
      <Id>/hostedzone/Z-BBB</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	zoneID, err := r.findHostedZone(context.Background(), "sub.example.com")
	if err != nil {
		t.Fatalf("findHostedZone() error: %v", err)
	}
	if zoneID != "/hostedzone/Z-BBB" {
		t.Errorf("findHostedZone() = %q, want %q", zoneID, "/hostedzone/Z-BBB")
	}
}

func TestRoute53_findHostedZone_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z-AAA</Id>
      <Name>other.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	_, err := r.findHostedZone(context.Background(), "app.nomatch.com")
	if err == nil {
		t.Fatal("expected error when no hosted zone matches")
	}
	if !strings.Contains(err.Error(), "no Route53 hosted zone found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRoute53_findHostedZone_NetworkError(t *testing.T) {
	r := NewRoute53("ak", "sk", "us-east-1")
	r.client.Transport = &failTransport{}

	_, err := r.findHostedZone(context.Background(), "app.example.com")
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestRoute53_ImplementsDNSProvider(t *testing.T) {
	var _ core.DNSProvider = (*Route53)(nil)
}

func TestRoute53_findHostedZone_XMLParsing(t *testing.T) {
	// Verify our test XML matches what xml.Unmarshal expects
	xmlData := `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/ZABC</Id>
      <Name>test.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`

	var result struct {
		XMLName     xml.Name `xml:"ListHostedZonesResponse"`
		HostedZones struct {
			Zones []struct {
				ID   string `xml:"Id"`
				Name string `xml:"Name"`
			} `xml:"HostedZone"`
		} `xml:"HostedZones"`
	}
	if err := xml.Unmarshal([]byte(xmlData), &result); err != nil {
		t.Fatalf("XML unmarshal error: %v", err)
	}
	if len(result.HostedZones.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result.HostedZones.Zones))
	}
	if result.HostedZones.Zones[0].ID != "/hostedzone/ZABC" {
		t.Errorf("zone ID = %q, want /hostedzone/ZABC", result.HostedZones.Zones[0].ID)
	}
}

// ===========================================================================
// Edge case tests for http.NewRequestWithContext error paths
// ===========================================================================

func TestCloudflare_do_InvalidMethod(t *testing.T) {
	cf := NewCloudflare("test-token")
	// Invalid HTTP method triggers NewRequestWithContext error
	_, err := cf.do(context.Background(), "INVALID METHOD", "/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid HTTP method")
	}
}

func TestRoute53_changeRecord_InvalidContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z111</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
	}))
	defer srv.Close()

	r := NewRoute53("ak", "sk", "us-east-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	// Use a cancelled context to trigger the post request error
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := r.CreateRecord(ctx, core.DNSRecord{
		Type:  "A",
		Name:  "app.example.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestRoute53_findHostedZone_InvalidContext(t *testing.T) {
	r := NewRoute53("ak", "sk", "us-east-1")

	// Use a nil-like invalid context — cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.findHostedZone(ctx, "app.example.com")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ===========================================================================
// Test helpers
// ===========================================================================

// rewriteTransport intercepts HTTP requests and redirects them to a local test server.
type rewriteTransport struct {
	base string // test server URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Replace the destination with our test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.base, "http://")
	return http.DefaultTransport.RoundTrip(req)
}

// failTransport always returns an error.
type failTransport struct{}

func (t *failTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated network failure")
}
