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

const linodeAPI = "https://api.linode.com/v4"

// Linode implements core.VPSProvisioner for Akamai/Linode.
type Linode struct {
	token  string
	client *http.Client
}

func NewLinode(apiToken string) core.VPSProvisioner {
	return &Linode{
		token:  apiToken,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (l *Linode) Name() string { return "linode" }

func (l *Linode) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	body, err := l.get(ctx, "/regions")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"data"`
	}
	json.Unmarshal(body, &resp)

	regions := make([]core.VPSRegion, len(resp.Data))
	for i, r := range resp.Data {
		regions[i] = core.VPSRegion{ID: r.ID, Name: r.Label}
	}
	return regions, nil
}

func (l *Linode) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	body, err := l.get(ctx, "/linode/types")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			VCPUs  int    `json:"vcpus"`
			Memory int    `json:"memory"`
			Disk   int    `json:"disk"`
			Price  struct {
				Hourly float64 `json:"hourly"`
			} `json:"price"`
		} `json:"data"`
	}
	json.Unmarshal(body, &resp)

	sizes := make([]core.VPSSize, len(resp.Data))
	for i, s := range resp.Data {
		sizes[i] = core.VPSSize{
			ID: s.ID, Name: s.Label,
			CPUs: s.VCPUs, MemoryMB: s.Memory, DiskGB: s.Disk / 1024,
			PriceHour: s.Price.Hourly,
		}
	}
	return sizes, nil
}

func (l *Linode) Create(ctx context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	payload := map[string]any{
		"label":     opts.Name,
		"region":    opts.Region,
		"type":      opts.Size,
		"image":     opts.Image,
		"root_pass": core.GeneratePassword(24),
		"metadata":  map[string]string{"user_data": opts.UserData},
	}
	body, err := l.post(ctx, "/linode/instances", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		ID     int      `json:"id"`
		Label  string   `json:"label"`
		IPv4   []string `json:"ipv4"`
		Status string   `json:"status"`
	}
	json.Unmarshal(body, &resp)

	ip := ""
	if len(resp.IPv4) > 0 {
		ip = resp.IPv4[0]
	}

	return &core.VPSInstance{
		ID: fmt.Sprintf("%d", resp.ID), Name: resp.Label,
		IPAddress: ip, Status: resp.Status,
		Region: opts.Region, Size: opts.Size,
	}, nil
}

func (l *Linode) Delete(ctx context.Context, instanceID string) error {
	_, err := l.do(ctx, http.MethodDelete, "/linode/instances/"+instanceID, nil)
	return err
}

func (l *Linode) Status(ctx context.Context, instanceID string) (string, error) {
	body, err := l.get(ctx, "/linode/instances/"+instanceID)
	if err != nil {
		return "", err
	}
	var resp struct {
		Status string `json:"status"`
	}
	json.Unmarshal(body, &resp)
	return resp.Status, nil
}

func (l *Linode) get(ctx context.Context, path string) ([]byte, error) {
	return l.do(ctx, http.MethodGet, path, nil)
}

func (l *Linode) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return l.do(ctx, http.MethodPost, path, payload)
}

func (l *Linode) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, linodeAPI+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+l.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linode API: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("linode API %s: HTTP %d", path, resp.StatusCode)
	}
	return respBody, nil
}
