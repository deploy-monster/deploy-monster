package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const doAPI = "https://api.digitalocean.com/v2"

// DigitalOcean implements core.VPSProvisioner for DigitalOcean.
type DigitalOcean struct {
	token  string
	client *http.Client
}

func NewDigitalOcean(apiToken string) core.VPSProvisioner {
	return &DigitalOcean{
		token:  apiToken,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DigitalOcean) Name() string { return "digitalocean" }

func (d *DigitalOcean) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	body, err := d.get(ctx, "/regions")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Regions []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"regions"`
	}
	json.Unmarshal(body, &resp)

	regions := make([]core.VPSRegion, len(resp.Regions))
	for i, r := range resp.Regions {
		regions[i] = core.VPSRegion{ID: r.Slug, Name: r.Name}
	}
	return regions, nil
}

func (d *DigitalOcean) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	body, err := d.get(ctx, "/sizes")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Sizes []struct {
			Slug     string  `json:"slug"`
			VCPUs    int     `json:"vcpus"`
			Memory   int     `json:"memory"`
			Disk     int     `json:"disk"`
			PriceHourly float64 `json:"price_hourly"`
		} `json:"sizes"`
	}
	json.Unmarshal(body, &resp)

	sizes := make([]core.VPSSize, len(resp.Sizes))
	for i, s := range resp.Sizes {
		sizes[i] = core.VPSSize{
			ID: s.Slug, Name: s.Slug,
			CPUs: s.VCPUs, MemoryMB: s.Memory, DiskGB: s.Disk,
			PriceHour: s.PriceHourly,
		}
	}
	return sizes, nil
}

func (d *DigitalOcean) Create(ctx context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	payload := map[string]any{
		"name":      opts.Name,
		"region":    opts.Region,
		"size":      opts.Size,
		"image":     opts.Image,
		"user_data": opts.UserData,
	}
	body, err := d.post(ctx, "/droplets", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Droplet struct {
			ID     int    `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"droplet"`
	}
	json.Unmarshal(body, &resp)

	return &core.VPSInstance{
		ID:     fmt.Sprintf("%d", resp.Droplet.ID),
		Name:   resp.Droplet.Name,
		Status: resp.Droplet.Status,
		Region: opts.Region,
		Size:   opts.Size,
	}, nil
}

func (d *DigitalOcean) Delete(ctx context.Context, instanceID string) error {
	_, err := d.do(ctx, http.MethodDelete, "/droplets/"+instanceID, nil)
	return err
}

func (d *DigitalOcean) Status(ctx context.Context, instanceID string) (string, error) {
	body, err := d.get(ctx, "/droplets/"+instanceID)
	if err != nil {
		return "", err
	}
	var resp struct {
		Droplet struct{ Status string `json:"status"` } `json:"droplet"`
	}
	json.Unmarshal(body, &resp)
	return resp.Droplet.Status, nil
}

func (d *DigitalOcean) get(ctx context.Context, path string) ([]byte, error) {
	return d.do(ctx, http.MethodGet, path, nil)
}

func (d *DigitalOcean) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return d.do(ctx, http.MethodPost, path, payload)
}

func (d *DigitalOcean) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, doAPI+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("digitalocean API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("digitalocean API %s: HTTP %d", path, resp.StatusCode)
	}
	return respBody, nil
}
