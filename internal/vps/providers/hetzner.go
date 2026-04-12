package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const hetznerAPI = "https://api.hetzner.cloud/v1"

// hetznerPageSize is the per-page count requested from paginated endpoints.
// 50 is the Hetzner Cloud API default and keeps each response small.
const hetznerPageSize = 50

// Hetzner implements core.VPSProvisioner for Hetzner Cloud.
type Hetzner struct {
	token  string
	client *http.Client
	cb     *core.CircuitBreaker
}

// NewHetzner creates a Hetzner Cloud provisioner.
func NewHetzner(apiToken string) core.VPSProvisioner {
	return &Hetzner{
		token:  apiToken,
		client: core.NewHTTPClient(30 * time.Second),
		cb:     core.NewCircuitBreaker("hetzner", core.DefaultCircuitBreakerConfig()),
	}
}

func (h *Hetzner) Name() string { return "hetzner" }

// hetznerLocation mirrors the relevant fields of a location entry in the
// `/locations` response. Kept as a named type so ListRegions can accumulate
// across pages.
type hetznerLocation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	City        string `json:"city"`
}

type hetznerLocationsPage struct {
	Locations []hetznerLocation `json:"locations"`
	Meta      hetznerMeta       `json:"meta"`
}

type hetznerServerType struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Cores       int     `json:"cores"`
	Memory      float64 `json:"memory"`
	Disk        int     `json:"disk"`
}

type hetznerServerTypesPage struct {
	ServerTypes []hetznerServerType `json:"server_types"`
	Meta        hetznerMeta         `json:"meta"`
}

// hetznerMeta matches the `meta.pagination` shape used by Hetzner list endpoints.
// `next_page` is null once the caller has reached the final page.
type hetznerMeta struct {
	Pagination struct {
		Page     int  `json:"page"`
		NextPage *int `json:"next_page"`
	} `json:"pagination"`
}

func (h *Hetzner) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	var all []core.VPSRegion
	page := 1
	for i := 0; i < vpsMaxPages; i++ {
		body, err := h.get(ctx, fmt.Sprintf("/locations?page=%d&per_page=%d", page, hetznerPageSize))
		if err != nil {
			return nil, err
		}
		var resp hetznerLocationsPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("hetzner API: decode locations: %w", err)
		}
		for _, loc := range resp.Locations {
			all = append(all, core.VPSRegion{
				ID:   loc.Name,
				Name: loc.Description + " (" + loc.City + ")",
			})
		}
		if resp.Meta.Pagination.NextPage == nil || *resp.Meta.Pagination.NextPage == 0 {
			return all, nil
		}
		page = *resp.Meta.Pagination.NextPage
	}
	return all, nil
}

func (h *Hetzner) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	var all []core.VPSSize
	page := 1
	for i := 0; i < vpsMaxPages; i++ {
		body, err := h.get(ctx, fmt.Sprintf("/server_types?page=%d&per_page=%d", page, hetznerPageSize))
		if err != nil {
			return nil, err
		}
		var resp hetznerServerTypesPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("hetzner API: decode server_types: %w", err)
		}
		for _, st := range resp.ServerTypes {
			all = append(all, core.VPSSize{
				ID:       st.Name,
				Name:     st.Description,
				CPUs:     st.Cores,
				MemoryMB: int(st.Memory * 1024),
				DiskGB:   st.Disk,
			})
		}
		if resp.Meta.Pagination.NextPage == nil || *resp.Meta.Pagination.NextPage == 0 {
			return all, nil
		}
		page = *resp.Meta.Pagination.NextPage
	}
	return all, nil
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
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("hetzner API: decode create response: %w", err)
	}

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
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("hetzner API: decode status response: %w", err)
	}
	return resp.Server.Status, nil
}

func (h *Hetzner) get(ctx context.Context, path string) ([]byte, error) {
	return h.do(ctx, http.MethodGet, path, nil)
}

func (h *Hetzner) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return h.do(ctx, http.MethodPost, path, payload)
}

func (h *Hetzner) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	data, err := vpsMarshalPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("hetzner API: marshal payload: %w", err)
	}
	return vpsDoRequest(ctx, h.client, h.cb, "hetzner", method, hetznerAPI+path, h.token, data)
}
