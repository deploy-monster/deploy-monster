package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const doAPI = "https://api.digitalocean.com/v2"

// doPageSize is the per-page count requested from paginated endpoints.
// DigitalOcean accepts values up to 200; 100 keeps each response small but
// limits round trips.
const doPageSize = 100

// DigitalOcean implements core.VPSProvisioner for DigitalOcean.
type DigitalOcean struct {
	token  string
	client *http.Client
	cb     *core.CircuitBreaker
}

func NewDigitalOcean(apiToken string) core.VPSProvisioner {
	return &DigitalOcean{
		token:  apiToken,
		client: core.NewHTTPClient(30 * time.Second),
		cb:     core.NewCircuitBreaker("digitalocean", core.DefaultCircuitBreakerConfig()),
	}
}

func (d *DigitalOcean) Name() string { return "digitalocean" }

// doLinks matches the top-level `links.pages` pagination block returned by
// DigitalOcean list endpoints. `next` is an absolute URL; when it is empty the
// caller has reached the final page.
type doLinks struct {
	Pages struct {
		Next string `json:"next"`
	} `json:"pages"`
}

type doRegionEntry struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type doRegionsPage struct {
	Regions []doRegionEntry `json:"regions"`
	Links   doLinks         `json:"links"`
}

type doSizeEntry struct {
	Slug        string  `json:"slug"`
	VCPUs       int     `json:"vcpus"`
	Memory      int     `json:"memory"`
	Disk        int     `json:"disk"`
	PriceHourly float64 `json:"price_hourly"`
}

type doSizesPage struct {
	Sizes []doSizeEntry `json:"sizes"`
	Links doLinks       `json:"links"`
}

func (d *DigitalOcean) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	var all []core.VPSRegion
	path := fmt.Sprintf("/regions?page=1&per_page=%d", doPageSize)
	for i := 0; i < vpsMaxPages; i++ {
		body, err := d.get(ctx, path)
		if err != nil {
			return nil, err
		}
		var resp doRegionsPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("digitalocean API: decode regions: %w", err)
		}
		for _, r := range resp.Regions {
			all = append(all, core.VPSRegion{ID: r.Slug, Name: r.Name})
		}
		next, ok := doNextPath(resp.Links.Pages.Next)
		if !ok {
			return all, nil
		}
		path = next
	}
	return all, nil
}

func (d *DigitalOcean) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	var all []core.VPSSize
	path := fmt.Sprintf("/sizes?page=1&per_page=%d", doPageSize)
	for i := 0; i < vpsMaxPages; i++ {
		body, err := d.get(ctx, path)
		if err != nil {
			return nil, err
		}
		var resp doSizesPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("digitalocean API: decode sizes: %w", err)
		}
		for _, s := range resp.Sizes {
			all = append(all, core.VPSSize{
				ID: s.Slug, Name: s.Slug,
				CPUs: s.VCPUs, MemoryMB: s.Memory, DiskGB: s.Disk,
				PriceHour: s.PriceHourly,
			})
		}
		next, ok := doNextPath(resp.Links.Pages.Next)
		if !ok {
			return all, nil
		}
		path = next
	}
	return all, nil
}

// doNextPath converts a DigitalOcean `links.pages.next` absolute URL into a
// relative path suitable for appending to the API base. Returns false when the
// link is empty or unparseable.
func doNextPath(nextURL string) (string, bool) {
	if strings.TrimSpace(nextURL) == "" {
		return "", false
	}
	u, err := url.Parse(nextURL)
	if err != nil {
		return "", false
	}
	// Strip the /v2 base so the path can be concatenated with doAPI.
	p := strings.TrimPrefix(u.Path, "/v2")
	if p == "" {
		p = "/"
	}
	if u.RawQuery != "" {
		return p + "?" + u.RawQuery, true
	}
	return p, true
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
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("digitalocean API: decode create response: %w", err)
	}

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
		Droplet struct {
			Status string `json:"status"`
		} `json:"droplet"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("digitalocean API: decode status response: %w", err)
	}
	return resp.Droplet.Status, nil
}

func (d *DigitalOcean) get(ctx context.Context, path string) ([]byte, error) {
	return d.do(ctx, http.MethodGet, path, nil)
}

func (d *DigitalOcean) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return d.do(ctx, http.MethodPost, path, payload)
}

func (d *DigitalOcean) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	data, err := vpsMarshalPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("digitalocean API: marshal payload: %w", err)
	}
	return vpsDoRequest(ctx, d.client, d.cb, "digitalocean", method, doAPI+path, d.token, data)
}
