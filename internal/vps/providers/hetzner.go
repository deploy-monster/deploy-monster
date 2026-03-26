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

const hetznerAPI = "https://api.hetzner.cloud/v1"

// Hetzner implements core.VPSProvisioner for Hetzner Cloud.
type Hetzner struct {
	token  string
	client *http.Client
}

// NewHetzner creates a Hetzner Cloud provisioner.
func NewHetzner(apiToken string) core.VPSProvisioner {
	return &Hetzner{
		token:  apiToken,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *Hetzner) Name() string { return "hetzner" }

func (h *Hetzner) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	body, err := h.get(ctx, "/locations")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Locations []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			City        string `json:"city"`
		} `json:"locations"`
	}
	json.Unmarshal(body, &resp)

	regions := make([]core.VPSRegion, len(resp.Locations))
	for i, loc := range resp.Locations {
		regions[i] = core.VPSRegion{ID: loc.Name, Name: loc.Description + " (" + loc.City + ")"}
	}
	return regions, nil
}

func (h *Hetzner) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	body, err := h.get(ctx, "/server_types")
	if err != nil {
		return nil, err
	}

	var resp struct {
		ServerTypes []struct {
			Name        string  `json:"name"`
			Description string  `json:"description"`
			Cores       int     `json:"cores"`
			Memory      float64 `json:"memory"`
			Disk        int     `json:"disk"`
			Prices      []struct {
				PriceHourly struct {
					Gross string `json:"gross"`
				} `json:"price_hourly"`
			} `json:"prices"`
		} `json:"server_types"`
	}
	json.Unmarshal(body, &resp)

	sizes := make([]core.VPSSize, len(resp.ServerTypes))
	for i, st := range resp.ServerTypes {
		sizes[i] = core.VPSSize{
			ID:       st.Name,
			Name:     st.Description,
			CPUs:     st.Cores,
			MemoryMB: int(st.Memory * 1024),
			DiskGB:   st.Disk,
		}
	}
	return sizes, nil
}

func (h *Hetzner) Create(ctx context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	payload := map[string]any{
		"name":        opts.Name,
		"server_type": opts.Size,
		"location":    opts.Region,
		"image":       opts.Image,
		"user_data":   opts.UserData,
	}

	body, err := h.post(ctx, "/servers", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Server struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			Status    string `json:"status"`
			PublicNet struct {
				IPv4 struct {
					IP string `json:"ip"`
				} `json:"ipv4"`
			} `json:"public_net"`
		} `json:"server"`
	}
	json.Unmarshal(body, &resp)

	return &core.VPSInstance{
		ID:        fmt.Sprintf("%d", resp.Server.ID),
		Name:      resp.Server.Name,
		IPAddress: resp.Server.PublicNet.IPv4.IP,
		Status:    resp.Server.Status,
		Region:    opts.Region,
		Size:      opts.Size,
	}, nil
}

func (h *Hetzner) Delete(ctx context.Context, instanceID string) error {
	_, err := h.do(ctx, http.MethodDelete, "/servers/"+instanceID, nil)
	return err
}

func (h *Hetzner) Status(ctx context.Context, instanceID string) (string, error) {
	body, err := h.get(ctx, "/servers/"+instanceID)
	if err != nil {
		return "", err
	}
	var resp struct {
		Server struct {
			Status string `json:"status"`
		} `json:"server"`
	}
	json.Unmarshal(body, &resp)
	return resp.Server.Status, nil
}

func (h *Hetzner) get(ctx context.Context, path string) ([]byte, error) {
	return h.do(ctx, http.MethodGet, path, nil)
}

func (h *Hetzner) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return h.do(ctx, http.MethodPost, path, payload)
}

func (h *Hetzner) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, _ := json.Marshal(payload)
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, hetznerAPI+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hetzner API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("hetzner API %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
