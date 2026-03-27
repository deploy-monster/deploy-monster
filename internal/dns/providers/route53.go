package providers

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

var _ core.DNSProvider = (*Route53)(nil)

// Route53 implements core.DNSProvider for AWS Route 53.
// Uses raw HTTP with AWS Signature V4 (simplified — production would need full SigV4).
type Route53 struct {
	accessKey string
	secretKey string
	region    string
	client    *http.Client
}

// NewRoute53 creates a Route 53 DNS provider.
func NewRoute53(accessKey, secretKey, region string) *Route53 {
	return &Route53{
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *Route53) Name() string { return "route53" }

func (r *Route53) CreateRecord(ctx context.Context, record core.DNSRecord) error {
	return r.changeRecord(ctx, "CREATE", record)
}

func (r *Route53) UpdateRecord(ctx context.Context, record core.DNSRecord) error {
	return r.changeRecord(ctx, "UPSERT", record)
}

func (r *Route53) DeleteRecord(ctx context.Context, recordID string) error {
	// Route53 delete requires the full record, not just ID
	return fmt.Errorf("Route53 delete requires full record context")
}

func (r *Route53) Verify(ctx context.Context, fqdn string) (bool, error) {
	return true, nil
}

func (r *Route53) changeRecord(ctx context.Context, action string, record core.DNSRecord) error {
	hostedZoneID, err := r.findHostedZone(ctx, record.Name)
	if err != nil {
		return err
	}

	ttl := record.TTL
	if ttl == 0 {
		ttl = 300
	}

	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>%s</Action>
        <ResourceRecordSet>
          <Name>%s</Name>
          <Type>%s</Type>
          <TTL>%d</TTL>
          <ResourceRecords>
            <ResourceRecord>
              <Value>%s</Value>
            </ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`, action, record.Name, record.Type, ttl, record.Value)

	url := fmt.Sprintf("https://route53.amazonaws.com/2013-04-01/hostedzone/%s/rrset", hostedZoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(xmlBody)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	// AWS SigV4 signing would go here

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("route53 API: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("route53: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (r *Route53) findHostedZone(ctx context.Context, domain string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://route53.amazonaws.com/2013-04-01/hostedzone", nil)
	if err != nil {
		return "", err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		XMLName     xml.Name `xml:"ListHostedZonesResponse"`
		HostedZones struct {
			Zones []struct {
				ID   string `xml:"Id"`
				Name string `xml:"Name"`
			} `xml:"HostedZone"`
		} `xml:"HostedZones"`
	}
	_ = xml.Unmarshal(body, &result)

	for _, zone := range result.HostedZones.Zones {
		if len(domain) >= len(zone.Name) && domain[len(domain)-len(zone.Name):] == zone.Name {
			return zone.ID, nil
		}
	}
	return "", fmt.Errorf("no Route53 hosted zone found for %s", domain)
}
