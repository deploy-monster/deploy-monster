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

const vultrAPI = "https://api.vultr.com/v2"

// Vultr implements core.VPSProvisioner for Vultr.
type Vultr struct {
	token  string
	client *http.Client
}

func NewVultr(apiToken string) core.VPSProvisioner {
	return &Vultr{
		token:  apiToken,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (v *Vultr) Name() string { return "vultr" }

func (v *Vultr) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	body, err := v.get(ctx, "/regions")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Regions []struct {
			ID   string `json:"id"`
			City string `json:"city"`
		} `json:"regions"`
	}
	json.Unmarshal(body, &resp)

	regions := make([]core.VPSRegion, len(resp.Regions))
	for i, r := range resp.Regions {
		regions[i] = core.VPSRegion{ID: r.ID, Name: r.City}
	}
	return regions, nil
}

func (v *Vultr) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	body, err := v.get(ctx, "/plans")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Plans []struct {
			ID        string  `json:"id"`
			VCPUs     int     `json:"vcpu_count"`
			RAM       int     `json:"ram"`
			Disk      int     `json:"disk"`
			CostHour  float64 `json:"monthly_cost"`
		} `json:"plans"`
	}
	json.Unmarshal(body, &resp)

	sizes := make([]core.VPSSize, len(resp.Plans))
	for i, p := range resp.Plans {
		sizes[i] = core.VPSSize{
			ID: p.ID, Name: p.ID,
			CPUs: p.VCPUs, MemoryMB: p.RAM, DiskGB: p.Disk,
			PriceHour: p.CostHour / 720, // Monthly to hourly
		}
	}
	return sizes, nil
}

func (v *Vultr) Create(ctx context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	payload := map[string]any{
		"label":    opts.Name,
		"region":   opts.Region,
		"plan":     opts.Size,
		"os_id":    opts.Image,
		"user_data": opts.UserData,
	}
	body, err := v.post(ctx, "/instances", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Instance struct {
			ID       string `json:"id"`
			Label    string `json:"label"`
			MainIP   string `json:"main_ip"`
			Status   string `json:"status"`
		} `json:"instance"`
	}
	json.Unmarshal(body, &resp)

	return &core.VPSInstance{
		ID: resp.Instance.ID, Name: resp.Instance.Label,
		IPAddress: resp.Instance.MainIP, Status: resp.Instance.Status,
		Region: opts.Region, Size: opts.Size,
	}, nil
}

func (v *Vultr) Delete(ctx context.Context, instanceID string) error {
	_, err := v.do(ctx, http.MethodDelete, "/instances/"+instanceID, nil)
	return err
}

func (v *Vultr) Status(ctx context.Context, instanceID string) (string, error) {
	body, err := v.get(ctx, "/instances/"+instanceID)
	if err != nil {
		return "", err
	}
	var resp struct {
		Instance struct{ Status string `json:"status"` } `json:"instance"`
	}
	json.Unmarshal(body, &resp)
	return resp.Instance.Status, nil
}

func (v *Vultr) get(ctx context.Context, path string) ([]byte, error) {
	return v.do(ctx, http.MethodGet, path, nil)
}

func (v *Vultr) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return v.do(ctx, http.MethodPost, path, payload)
}

func (v *Vultr) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, vultrAPI+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+v.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vultr API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("vultr API %s: HTTP %d", path, resp.StatusCode)
	}
	return respBody, nil
}
