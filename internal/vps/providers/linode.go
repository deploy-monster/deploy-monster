package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const linodeAPI = "https://api.linode.com/v4"

// linodePageSize is the per-page count requested from paginated endpoints.
// Linode's documented maximum is 500; 100 keeps response sizes modest.
const linodePageSize = 100

// Linode implements core.VPSProvisioner for Akamai/Linode.
type Linode struct {
	token  string
	client *http.Client
	cb     *core.CircuitBreaker
}

func NewLinode(apiToken string) core.VPSProvisioner {
	return &Linode{
		token:  apiToken,
		client: core.NewHTTPClient(30 * time.Second),
		cb:     core.NewCircuitBreaker("linode", core.DefaultCircuitBreakerConfig()),
	}
}

func (l *Linode) Name() string { return "linode" }

// linodePageMeta matches the top-level pagination fields returned by Linode
// list endpoints. The caller has reached the final page when `Page >= Pages`.
type linodePageMeta struct {
	Page  int `json:"page"`
	Pages int `json:"pages"`
}

type linodeRegionEntry struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type linodeRegionsPage struct {
	Data []linodeRegionEntry `json:"data"`
	linodePageMeta
}

type linodeTypeEntry struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	VCPUs  int    `json:"vcpus"`
	Memory int    `json:"memory"`
	Disk   int    `json:"disk"`
	Price  struct {
		Hourly float64 `json:"hourly"`
	} `json:"price"`
}

type linodeTypesPage struct {
	Data []linodeTypeEntry `json:"data"`
	linodePageMeta
}

func (l *Linode) ListRegions(ctx context.Context) ([]core.VPSRegion, error) {
	var all []core.VPSRegion
	page := 1
	for i := 0; i < vpsMaxPages; i++ {
		body, err := l.get(ctx, fmt.Sprintf("/regions?page=%d&page_size=%d", page, linodePageSize))
		if err != nil {
			return nil, err
		}
		var resp linodeRegionsPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("linode API: decode regions: %w", err)
		}
		for _, r := range resp.Data {
			all = append(all, core.VPSRegion{ID: r.ID, Name: r.Label})
		}
		if resp.Page >= resp.Pages {
			return all, nil
		}
		page = resp.Page + 1
	}
	return all, nil
}

func (l *Linode) ListSizes(ctx context.Context, _ string) ([]core.VPSSize, error) {
	var all []core.VPSSize
	page := 1
	for i := 0; i < vpsMaxPages; i++ {
		body, err := l.get(ctx, fmt.Sprintf("/linode/types?page=%d&page_size=%d", page, linodePageSize))
		if err != nil {
			return nil, err
		}
		var resp linodeTypesPage
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("linode API: decode types: %w", err)
		}
		for _, s := range resp.Data {
			all = append(all, core.VPSSize{
				ID: s.ID, Name: s.Label,
				CPUs: s.VCPUs, MemoryMB: s.Memory, DiskGB: s.Disk / 1024,
				PriceHour: s.Price.Hourly,
			})
		}
		if resp.Page >= resp.Pages {
			return all, nil
		}
		page = resp.Page + 1
	}
	return all, nil
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
	// Linode's authorized_keys field takes literal public-key strings rather
	// than an opaque key-ID reference — SSHKeyID here is treated as the
	// OpenSSH-formatted public key. Omitted when empty so the fallback stays
	// the generated root_pass above. Callers that need to reference a Linode
	// SSH key by ID should resolve it to the public-key material before
	// calling Create (the /profile/sshkeys list endpoint returns both).
	if opts.SSHKeyID != "" {
		payload["authorized_keys"] = []string{opts.SSHKeyID}
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
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("linode API: decode create response: %w", err)
	}

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
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("linode API: decode status response: %w", err)
	}
	return resp.Status, nil
}

func (l *Linode) get(ctx context.Context, path string) ([]byte, error) {
	return l.do(ctx, http.MethodGet, path, nil)
}

func (l *Linode) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return l.do(ctx, http.MethodPost, path, payload)
}

func (l *Linode) do(ctx context.Context, method, path string, payload any) ([]byte, error) {
	data, err := vpsMarshalPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("linode API: marshal payload: %w", err)
	}
	return vpsDoRequest(ctx, l.client, l.cb, "linode", method, linodeAPI+path, l.token, data)
}
