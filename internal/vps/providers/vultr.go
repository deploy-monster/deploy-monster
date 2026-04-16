package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const vultrAPI = "https://api.vultr.com/v2"

// vultrPageSize is the per-page count requested from paginated list endpoints.
// Vultr's documented maximum is 500; 100 keeps request payloads small.
const vultrPageSize = 100

// Vultr implements core.VPSProvisioner for Vultr.
type Vultr struct {
	token  string
	client *http.Client
	cb     *core.CircuitBreaker
}

func NewVultr(apiToken string) core.VPSProvisioner {
	return &Vultr{
		token:  apiToken,
		client: core.NewHTTPClient(30 * time.Second),
		cb:     core.NewCircuitBreaker("vultr", core.DefaultCircuitBreakerConfig()),
	}
}

func (v *Vultr) Name() string { return "vultr" }

// vultrMeta matches the top-level `meta` block returned by Vultr list endpoints.
// `links.next` is a cursor string that is empty once the caller has reached the
// final page.
type vultrMeta struct {
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}

type vultrRegionEntry struct {
	ID   string `json:"id"`
	City string `json:"city"`
}

type vultrRegionsPage struct {
	Regions []vultrRegionEntry `json:"regions"`
	Meta    vultrMeta          `json:"meta"`
}

type vultrPlanEntry struct {
	ID       string  `json:"id"`
	VCPUs    int     `json:"vcpu_count"`
	RAM      int     `json:"ram"`
	Disk     int     `json:"disk"`
	CostHour float64 `json:"monthly_cost"`
}

type vultrPlansPage struct {
	Plans []vultrPlanEntry `json:"plans"`
	Meta  vultrMeta        `json:"meta"`
}

func (v *Vultr) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	var all []core.VPSRegion
	cursor := ""
	for i := 0; i < vpsMaxPages; i++ {
		body, err := v.get(ctx, vultrListPath("/regions", cursor))
		if err != nil {
			return nil, err
		}
		var resp vultrRegionsPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("vultr API: decode regions: %w", err)
		}
		for _, r := range resp.Regions {
			all = append(all, core.VPSRegion{ID: r.ID, Name: r.City})
		}
		if resp.Meta.Links.Next == "" {
			return all, nil
		}
		cursor = resp.Meta.Links.Next
	}
	return all, nil
}

func (v *Vultr) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	var all []core.VPSSize
	cursor := ""
	for i := 0; i < vpsMaxPages; i++ {
		body, err := v.get(ctx, vultrListPath("/plans", cursor))
		if err != nil {
			return nil, err
		}
		var resp vultrPlansPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("vultr API: decode plans: %w", err)
		}
		for _, p := range resp.Plans {
			all = append(all, core.VPSSize{
				ID: p.ID, Name: p.ID,
				CPUs: p.VCPUs, MemoryMB: p.RAM, DiskGB: p.Disk,
				PriceHour: p.CostHour / 720, // Monthly to hourly
			})
		}
		if resp.Meta.Links.Next == "" {
			return all, nil
		}
		cursor = resp.Meta.Links.Next
	}
	return all, nil
}

// vultrListPath builds a Vultr list endpoint path with optional pagination
// cursor. The `per_page` parameter is always set; `cursor` is omitted for the
// first request.
func vultrListPath(base, cursor string) string {
	q := url.Values{}
	q.Set("per_page", fmt.Sprintf("%d", vultrPageSize))
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	return base + "?" + q.Encode()
}

func (v *Vultr) Create(ctx context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	payload := map[string]any{
		"label":     opts.Name,
		"region":    opts.Region,
		"plan":      opts.Size,
		"os_id":     opts.Image,
		"user_data": opts.UserData,
	}
	// Vultr takes sshkey_id as an array of SSH-key UUIDs registered at
	// /account/ssh-keys. Omitted when SSHKeyID is empty; single-element
	// array when set.
	if opts.SSHKeyID != "" {
		payload["sshkey_id"] = []string{opts.SSHKeyID}
	}
	body, err := v.post(ctx, "/instances", payload)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Instance struct {
			ID     string `json:"id"`
			Label  string `json:"label"`
			MainIP string `json:"main_ip"`
			Status string `json:"status"`
		} `json:"instance"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("vultr API: decode create response: %w", err)
	}

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
		Instance struct {
			Status string `json:"status"`
		} `json:"instance"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("vultr API: decode status response: %w", err)
	}
	return resp.Instance.Status, nil
}

func (v *Vultr) get(ctx context.Context, path string) ([]byte, error) {
	return v.do(ctx, http.MethodGet, path, nil)
}

func (v *Vultr) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return v.do(ctx, http.MethodPost, path, payload)
}

func (v *Vultr) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	data, err := vpsMarshalPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("vultr API: marshal payload: %w", err)
	}
	return vpsDoRequest(ctx, v.client, v.cb, "vultr", method, vultrAPI+path, v.token, data)
}
