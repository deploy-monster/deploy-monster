package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const cfAPI = "https://api.cloudflare.com/client/v4"

// Compile-time check.
var _ core.DNSProvider = (*Cloudflare)(nil)

// Cloudflare implements core.DNSProvider for Cloudflare DNS.
type Cloudflare struct {
	token  string
	client *http.Client
}

// NewCloudflare creates a Cloudflare DNS provider.
func NewCloudflare(apiToken string) *Cloudflare {
	return &Cloudflare{
		token:  apiToken,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Cloudflare) Name() string { return "cloudflare" }

func (c *Cloudflare) CreateRecord(ctx context.Context, record core.DNSRecord) error {
	zoneID, err := c.findZone(ctx, record.Name)
	if err != nil {
		return fmt.Errorf("cloudflare create record: %w", err)
	}

	payload := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Value,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}

	if _, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), payload); err != nil {
		return fmt.Errorf("cloudflare create record %s %s: %w", record.Type, record.Name, err)
	}
	return nil
}

func (c *Cloudflare) UpdateRecord(ctx context.Context, record core.DNSRecord) error {
	zoneID, err := c.findZone(ctx, record.Name)
	if err != nil {
		return fmt.Errorf("cloudflare update record: %w", err)
	}

	payload := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Value,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}

	if _, err := c.do(ctx, http.MethodPut, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), payload); err != nil {
		return fmt.Errorf("cloudflare update record %s: %w", record.ID, err)
	}
	return nil
}

func (c *Cloudflare) DeleteRecord(ctx context.Context, record core.DNSRecord) error {
	zoneID, err := c.findZone(ctx, record.Name)
	if err != nil {
		return fmt.Errorf("cloudflare delete record: %w", err)
	}
	if _, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), nil); err != nil {
		return fmt.Errorf("cloudflare delete record %s: %w", record.ID, err)
	}
	return nil
}

func (c *Cloudflare) Verify(ctx context.Context, fqdn string) (bool, error) {
	// SSRF protection: resolve the FQDN and reject if it points to an internal IP.
	// This prevents attackers from using DNS verification to probe cloud metadata
	// endpoints (e.g., 169.254.169.254 for AWS, metadata.google.internal for GCP).
	resolver := &net.Resolver{}
	ips, err := resolver.LookupHost(ctx, fqdn)
	if err != nil {
		return false, nil
	}
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil && isPublicIP(ip) {
			return true, nil
		}
	}
	// All resolved IPs are private/non-routable — reject the domain
	return false, nil
}

// isPublicIP returns true if the IP is a routable public address.
// Uses Go 1.17+'s built-in IsPrivate() which covers 10.x, 172.16-31.x, 192.168.x,
// loopback, link-local, and multicast. Additionally checks for cloud metadata ranges.
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// IsPrivate() covers RFC 1918 (10.x, 172.16-31.x, 192.168.x), loopback, link-local, multicast
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	// Reject cloud metadata endpoints (169.254.169.254, 169.254.169.253, etc.)
	metadataRanges := []string{
		"169.254.169.254/32", // AWS, Azure, GCP metadata
		"169.254.169.253/32", // Azure backup
		"metadata.google.internal/32", // GCP metadata
	}
	ip4 := ip.To4()
	if ip4 != nil {
		for _, r := range metadataRanges {
			_, cidr, _ := net.ParseCIDR(r)
			if cidr.Contains(ip) {
				return false
			}
		}
	}
	return true
}

// findZone looks up the Cloudflare zone ID for a domain.
func (c *Cloudflare) findZone(ctx context.Context, name string) (string, error) {
	// Extract root domain from FQDN
	body, err := c.do(ctx, http.MethodGet, "/zones?per_page=50", nil)
	if err != nil {
		return "", fmt.Errorf("list zones: %w", err)
	}

	var resp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("cloudflare zone parse: %w", err)
	}

	// Find the zone that matches the domain
	for _, zone := range resp.Result {
		if len(name) >= len(zone.Name) && name[len(name)-len(zone.Name):] == zone.Name {
			return zone.ID, nil
		}
	}

	return "", fmt.Errorf("no Cloudflare zone found for %s", name)
}

func (c *Cloudflare) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var result []byte
	err := core.Retry(ctx, core.DefaultRetryConfig(), func() error {
		var body io.Reader
		if payload != nil {
			data, _ := json.Marshal(payload)
			body = bytes.NewReader(data)
		}

		req, err := http.NewRequestWithContext(ctx, method, cfAPI+path, body)
		if err != nil {
			return fmt.Errorf("build %s %s: %w", method, path, err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("cloudflare API: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 500 {
			// Transient server error — retry
			return fmt.Errorf("cloudflare API %s: HTTP %d: %s", path, resp.StatusCode, string(respBody))
		}
		if resp.StatusCode >= 400 {
			// Client error — do not retry
			result = nil
			return core.ErrNoRetry(fmt.Errorf("cloudflare API %s: HTTP %d: %s", path, resp.StatusCode, string(respBody)))
		}
		result = respBody
		return nil
	})
	return result, err
}
