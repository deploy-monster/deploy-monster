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
		return err
	}

	payload := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Value,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}

	_, err = c.do(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), payload)
	return err
}

func (c *Cloudflare) UpdateRecord(ctx context.Context, record core.DNSRecord) error {
	zoneID, err := c.findZone(ctx, record.Name)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"type":    record.Type,
		"name":    record.Name,
		"content": record.Value,
		"ttl":     record.TTL,
		"proxied": record.Proxied,
	}

	_, err = c.do(ctx, http.MethodPut, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), payload)
	return err
}

func (c *Cloudflare) DeleteRecord(ctx context.Context, record core.DNSRecord) error {
	zoneID, err := c.findZone(ctx, record.Name)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodDelete, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), nil)
	return err
}

func (c *Cloudflare) Verify(ctx context.Context, fqdn string) (bool, error) {
	resolver := &net.Resolver{}
	_, err := resolver.LookupHost(ctx, fqdn)
	if err != nil {
		return false, nil
	}
	return true, nil
}

// findZone looks up the Cloudflare zone ID for a domain.
func (c *Cloudflare) findZone(ctx context.Context, name string) (string, error) {
	// Extract root domain from FQDN
	body, err := c.do(ctx, http.MethodGet, "/zones?per_page=50", nil)
	if err != nil {
		return "", err
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
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("cloudflare API: %w", err)
		}
		defer resp.Body.Close()

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
