package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Route53.changeRecord — HTTP error branch, TTL default, findHostedZone error
// These tests cover the remaining uncovered lines in route53.go:
// - changeRecord: HTTP client.Do error (line 97)
// - changeRecord: HTTP 400+ response (line 103)
// - findHostedZone: HTTP client.Do error (line 117)
// =============================================================================

func TestFinal_Route53_CreateRecord_FindHostedZoneError(t *testing.T) {
	// Server that returns an empty response for hosted zone listing
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<ListHostedZonesResponse><HostedZones></HostedZones></ListHostedZonesResponse>`))
	}))
	defer srv.Close()

	r53 := NewRoute53("test", "test", "us-east-1")
	r53.client.Transport = &rewriteTransport{base: srv.URL}

	err := r53.CreateRecord(context.Background(), core.DNSRecord{
		Name:  "test.example.com",
		Type:  "A",
		Value: "1.2.3.4",
		TTL:   0, // tests TTL default to 300
	})

	if err == nil {
		t.Error("expected error from findHostedZone (no matching zone)")
	}
}

func TestFinal_Route53_ChangeRecord_HTTPError(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// findHostedZone response with a matching zone
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
				<HostedZones>
					<HostedZone><Id>/hostedzone/Z123</Id><Name>example.com.</Name></HostedZone>
				</HostedZones>
			</ListHostedZonesResponse>`))
			return
		}
		// changeRecord response: HTTP 400 error
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`error`))
	}))
	defer srv.Close()

	r53 := NewRoute53("test", "test", "us-east-1")
	r53.client.Transport = &rewriteTransport{base: srv.URL}

	err := r53.CreateRecord(context.Background(), core.DNSRecord{
		Name:  "test.example.com.",
		Type:  "A",
		Value: "1.2.3.4",
		TTL:   600,
	})

	if err == nil {
		t.Error("expected HTTP error from changeRecord")
	}
}

func TestFinal_Route53_FindHostedZone_NetworkError(t *testing.T) {
	r53 := NewRoute53("test", "test", "us-east-1")
	r53.client.Transport = &failTransport{}

	err := r53.CreateRecord(context.Background(), core.DNSRecord{
		Name:  "test.example.com",
		Type:  "A",
		Value: "1.2.3.4",
	})

	if err == nil {
		t.Error("expected network error")
	}
}
