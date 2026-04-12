package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// urlRewriteTransport intercepts HTTP requests and rewrites the base URL
// to point at our httptest server. This lets us test providers whose base URLs
// are hard-coded package-level constants.
// =============================================================================

type urlRewriteTransport struct {
	target    string // httptest server URL (e.g. "http://127.0.0.1:xxxxx")
	origBases []string
	inner     http.RoundTripper
}

func (t *urlRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL.String()
	for _, base := range t.origBases {
		if strings.HasPrefix(u, base) {
			u = t.target + u[len(base):]
			break
		}
	}
	newReq := req.Clone(req.Context())
	parsed, err := req.URL.Parse(u)
	if err != nil {
		return nil, err
	}
	newReq.URL = parsed
	return t.inner.RoundTrip(newReq)
}

func rewriteClient(serverURL string, origBases ...string) *http.Client {
	return &http.Client{
		Transport: &urlRewriteTransport{
			target:    serverURL,
			origBases: origBases,
			inner:     http.DefaultTransport,
		},
	}
}

// =============================================================================
// Mock API servers
// =============================================================================

func mockHetznerServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, `{"error":{"message":"unauthorized"}}`, http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/locations" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"locations": []map[string]any{
					{"name": "fsn1", "description": "Falkenstein DC Park 1", "city": "Falkenstein"},
					{"name": "nbg1", "description": "Nuremberg DC Park 1", "city": "Nuremberg"},
				},
			})
		case r.URL.Path == "/server_types" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"server_types": []map[string]any{
					{
						"name": "cx11", "description": "CX11",
						"cores": 1, "memory": 2.0, "disk": 20,
						"prices": []map[string]any{
							{"price_hourly": map[string]string{"gross": "0.0060"}},
						},
					},
					{
						"name": "cx21", "description": "CX21",
						"cores": 2, "memory": 4.0, "disk": 40,
						"prices": []map[string]any{},
					},
				},
			})
		case r.URL.Path == "/servers" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"server": map[string]any{
					"id": 12345, "name": "my-server", "status": "initializing",
					"public_net": map[string]any{
						"ipv4": map[string]any{"ip": "1.2.3.4"},
					},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/servers/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"server": map[string]any{"status": "running"},
			})
		case strings.HasPrefix(r.URL.Path, "/servers/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func mockDOServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/regions" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"regions": []map[string]any{
					{"slug": "nyc1", "name": "New York 1"},
					{"slug": "sfo1", "name": "San Francisco 1"},
				},
			})
		case r.URL.Path == "/sizes" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"sizes": []map[string]any{
					{"slug": "s-1vcpu-1gb", "vcpus": 1, "memory": 1024, "disk": 25, "price_hourly": 0.00744},
					{"slug": "s-2vcpu-2gb", "vcpus": 2, "memory": 2048, "disk": 50, "price_hourly": 0.01488},
				},
			})
		case r.URL.Path == "/droplets" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"droplet": map[string]any{
					"id": 67890, "name": "do-test", "status": "new",
				},
			})
		case strings.HasPrefix(r.URL.Path, "/droplets/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"droplet": map[string]any{"status": "active"},
			})
		case strings.HasPrefix(r.URL.Path, "/droplets/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func mockVultrServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/regions" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"regions": []map[string]any{
					{"id": "ewr", "city": "New Jersey"},
					{"id": "ord", "city": "Chicago"},
				},
			})
		case r.URL.Path == "/plans" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"plans": []map[string]any{
					{"id": "vc2-1c-1gb", "vcpu_count": 1, "ram": 1024, "disk": 25, "monthly_cost": 5.0},
					{"id": "vc2-2c-4gb", "vcpu_count": 2, "ram": 4096, "disk": 80, "monthly_cost": 20.0},
				},
			})
		case r.URL.Path == "/instances" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"instance": map[string]any{
					"id": "vultr-abc123", "label": "vultr-test",
					"main_ip": "5.6.7.8", "status": "pending",
				},
			})
		case strings.HasPrefix(r.URL.Path, "/instances/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"instance": map[string]any{"status": "active"},
			})
		case strings.HasPrefix(r.URL.Path, "/instances/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

func mockLinodeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/regions" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "us-east", "label": "Newark, NJ"},
					{"id": "us-west", "label": "Fremont, CA"},
				},
			})
		case r.URL.Path == "/linode/types" && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "g6-nanode-1", "label": "Nanode 1GB", "vcpus": 1, "memory": 1024, "disk": 25600, "price": map[string]any{"hourly": 0.0075}},
					{"id": "g6-standard-2", "label": "Linode 4GB", "vcpus": 2, "memory": 4096, "disk": 81920, "price": map[string]any{"hourly": 0.03}},
				},
			})
		case r.URL.Path == "/linode/instances" && r.Method == http.MethodPost:
			json.NewEncoder(w).Encode(map[string]any{
				"id": 99999, "label": "linode-test",
				"ipv4": []string{"9.10.11.12"}, "status": "provisioning",
			})
		case strings.HasPrefix(r.URL.Path, "/linode/instances/") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"status": "running",
			})
		case strings.HasPrefix(r.URL.Path, "/linode/instances/") && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
}

// =============================================================================
// Hetzner — full success path tests via mock server
// =============================================================================

func TestHetznerCov_ListRegions_Success(t *testing.T) {
	srv := mockHetznerServer(t)
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	regions, err := h.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].ID != "fsn1" {
		t.Errorf("region[0].ID = %q, want %q", regions[0].ID, "fsn1")
	}
	if !strings.Contains(regions[0].Name, "Falkenstein") {
		t.Errorf("region[0].Name = %q, want to contain 'Falkenstein'", regions[0].Name)
	}
}

func TestHetznerCov_ListSizes_Success(t *testing.T) {
	srv := mockHetznerServer(t)
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	sizes, err := h.ListSizes(context.Background(), "fsn1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes, got %d", len(sizes))
	}
	if sizes[0].ID != "cx11" {
		t.Errorf("sizes[0].ID = %q", sizes[0].ID)
	}
	if sizes[0].CPUs != 1 {
		t.Errorf("sizes[0].CPUs = %d, want 1", sizes[0].CPUs)
	}
	if sizes[0].MemoryMB != 2048 {
		t.Errorf("sizes[0].MemoryMB = %d, want 2048", sizes[0].MemoryMB)
	}
	if sizes[0].DiskGB != 20 {
		t.Errorf("sizes[0].DiskGB = %d, want 20", sizes[0].DiskGB)
	}
}

func TestHetznerCov_Create_Success(t *testing.T) {
	srv := mockHetznerServer(t)
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	inst, err := h.Create(context.Background(), core.VPSCreateOpts{
		Name:     "my-server",
		Region:   "fsn1",
		Size:     "cx11",
		Image:    "ubuntu-22.04",
		UserData: "#!/bin/bash\necho hello",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "12345" {
		t.Errorf("ID = %q, want %q", inst.ID, "12345")
	}
	if inst.Name != "my-server" {
		t.Errorf("Name = %q", inst.Name)
	}
	if inst.IPAddress != "1.2.3.4" {
		t.Errorf("IPAddress = %q", inst.IPAddress)
	}
	if inst.Status != "initializing" {
		t.Errorf("Status = %q", inst.Status)
	}
	if inst.Region != "fsn1" {
		t.Errorf("Region = %q", inst.Region)
	}
	if inst.Size != "cx11" {
		t.Errorf("Size = %q", inst.Size)
	}
}

func TestHetznerCov_Status_Success(t *testing.T) {
	srv := mockHetznerServer(t)
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	status, err := h.Status(context.Background(), "12345")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "running" {
		t.Errorf("Status = %q, want %q", status, "running")
	}
}

func TestHetznerCov_Delete_Success(t *testing.T) {
	srv := mockHetznerServer(t)
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	if err := h.Delete(context.Background(), "12345"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestHetznerCov_HTTP400_Error(t *testing.T) {
	srv := mockHetznerServer(t)
	defer srv.Close()

	// Use wrong token to trigger 401 from mock
	h := &Hetzner{
		token:  "wrong-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	_, err := h.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	if !strings.Contains(err.Error(), "hetzner API") {
		t.Errorf("error = %q, want to contain 'hetzner API'", err.Error())
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want to contain '401'", err.Error())
	}
}

// =============================================================================
// DigitalOcean — full success path tests via mock server
// =============================================================================

func TestDOCov_ListRegions_Success(t *testing.T) {
	srv := mockDOServer(t)
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	regions, err := d.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].ID != "nyc1" {
		t.Errorf("region[0].ID = %q", regions[0].ID)
	}
	if regions[0].Name != "New York 1" {
		t.Errorf("region[0].Name = %q", regions[0].Name)
	}
}

func TestDOCov_ListSizes_Success(t *testing.T) {
	srv := mockDOServer(t)
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	sizes, err := d.ListSizes(context.Background(), "nyc1")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes, got %d", len(sizes))
	}
	if sizes[0].ID != "s-1vcpu-1gb" {
		t.Errorf("sizes[0].ID = %q", sizes[0].ID)
	}
	if sizes[0].CPUs != 1 {
		t.Errorf("sizes[0].CPUs = %d", sizes[0].CPUs)
	}
	if sizes[0].MemoryMB != 1024 {
		t.Errorf("sizes[0].MemoryMB = %d", sizes[0].MemoryMB)
	}
	if sizes[0].DiskGB != 25 {
		t.Errorf("sizes[0].DiskGB = %d", sizes[0].DiskGB)
	}
	if sizes[0].PriceHour != 0.00744 {
		t.Errorf("sizes[0].PriceHour = %f", sizes[0].PriceHour)
	}
}

func TestDOCov_Create_Success(t *testing.T) {
	srv := mockDOServer(t)
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	inst, err := d.Create(context.Background(), core.VPSCreateOpts{
		Name:     "do-test",
		Region:   "nyc1",
		Size:     "s-1vcpu-1gb",
		Image:    "ubuntu-22-04-x64",
		UserData: "#!/bin/bash",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "67890" {
		t.Errorf("ID = %q, want %q", inst.ID, "67890")
	}
	if inst.Name != "do-test" {
		t.Errorf("Name = %q", inst.Name)
	}
	if inst.Status != "new" {
		t.Errorf("Status = %q", inst.Status)
	}
	if inst.Region != "nyc1" {
		t.Errorf("Region = %q", inst.Region)
	}
	if inst.Size != "s-1vcpu-1gb" {
		t.Errorf("Size = %q", inst.Size)
	}
}

func TestDOCov_Status_Success(t *testing.T) {
	srv := mockDOServer(t)
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	status, err := d.Status(context.Background(), "67890")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "active" {
		t.Errorf("Status = %q, want %q", status, "active")
	}
}

func TestDOCov_Delete_Success(t *testing.T) {
	srv := mockDOServer(t)
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	if err := d.Delete(context.Background(), "67890"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDOCov_HTTP400_Error(t *testing.T) {
	srv := mockDOServer(t)
	defer srv.Close()

	d := &DigitalOcean{
		token:  "wrong-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	_, err := d.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	if !strings.Contains(err.Error(), "digitalocean API") {
		t.Errorf("error = %q, want to contain 'digitalocean API'", err.Error())
	}
}

// =============================================================================
// Vultr — full success path tests via mock server
// =============================================================================

func TestVultrCov_ListRegions_Success(t *testing.T) {
	srv := mockVultrServer(t)
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	regions, err := v.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].ID != "ewr" {
		t.Errorf("region[0].ID = %q", regions[0].ID)
	}
	if regions[0].Name != "New Jersey" {
		t.Errorf("region[0].Name = %q", regions[0].Name)
	}
}

func TestVultrCov_ListSizes_Success(t *testing.T) {
	srv := mockVultrServer(t)
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	sizes, err := v.ListSizes(context.Background(), "ewr")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes, got %d", len(sizes))
	}
	if sizes[0].ID != "vc2-1c-1gb" {
		t.Errorf("sizes[0].ID = %q", sizes[0].ID)
	}
	if sizes[0].CPUs != 1 {
		t.Errorf("sizes[0].CPUs = %d", sizes[0].CPUs)
	}
	if sizes[0].MemoryMB != 1024 {
		t.Errorf("sizes[0].MemoryMB = %d", sizes[0].MemoryMB)
	}
	// Vultr: PriceHour = monthly_cost / 720
	expectedPrice := 5.0 / 720
	if sizes[0].PriceHour != expectedPrice {
		t.Errorf("sizes[0].PriceHour = %f, want %f", sizes[0].PriceHour, expectedPrice)
	}
}

func TestVultrCov_Create_Success(t *testing.T) {
	srv := mockVultrServer(t)
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	inst, err := v.Create(context.Background(), core.VPSCreateOpts{
		Name:     "vultr-test",
		Region:   "ewr",
		Size:     "vc2-1c-1gb",
		Image:    "387",
		UserData: "cloud-init",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "vultr-abc123" {
		t.Errorf("ID = %q", inst.ID)
	}
	if inst.Name != "vultr-test" {
		t.Errorf("Name = %q", inst.Name)
	}
	if inst.IPAddress != "5.6.7.8" {
		t.Errorf("IPAddress = %q", inst.IPAddress)
	}
	if inst.Status != "pending" {
		t.Errorf("Status = %q", inst.Status)
	}
	if inst.Region != "ewr" {
		t.Errorf("Region = %q", inst.Region)
	}
}

func TestVultrCov_Status_Success(t *testing.T) {
	srv := mockVultrServer(t)
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	status, err := v.Status(context.Background(), "vultr-abc123")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "active" {
		t.Errorf("Status = %q, want %q", status, "active")
	}
}

func TestVultrCov_Delete_Success(t *testing.T) {
	srv := mockVultrServer(t)
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	if err := v.Delete(context.Background(), "vultr-abc123"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestVultrCov_HTTP400_Error(t *testing.T) {
	srv := mockVultrServer(t)
	defer srv.Close()

	v := &Vultr{
		token:  "wrong-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	_, err := v.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	if !strings.Contains(err.Error(), "vultr API") {
		t.Errorf("error = %q, want to contain 'vultr API'", err.Error())
	}
}

// =============================================================================
// Linode — full success path tests via mock server
// =============================================================================

func TestLinodeCov_ListRegions_Success(t *testing.T) {
	srv := mockLinodeServer(t)
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	regions, err := l.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].ID != "us-east" {
		t.Errorf("region[0].ID = %q", regions[0].ID)
	}
	if regions[0].Name != "Newark, NJ" {
		t.Errorf("region[0].Name = %q", regions[0].Name)
	}
}

func TestLinodeCov_ListSizes_Success(t *testing.T) {
	srv := mockLinodeServer(t)
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	sizes, err := l.ListSizes(context.Background(), "us-east")
	if err != nil {
		t.Fatalf("ListSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("expected 2 sizes, got %d", len(sizes))
	}
	if sizes[0].ID != "g6-nanode-1" {
		t.Errorf("sizes[0].ID = %q", sizes[0].ID)
	}
	if sizes[0].CPUs != 1 {
		t.Errorf("sizes[0].CPUs = %d", sizes[0].CPUs)
	}
	if sizes[0].MemoryMB != 1024 {
		t.Errorf("sizes[0].MemoryMB = %d", sizes[0].MemoryMB)
	}
	// Linode: DiskGB = disk / 1024
	if sizes[0].DiskGB != 25 {
		t.Errorf("sizes[0].DiskGB = %d, want 25", sizes[0].DiskGB)
	}
	if sizes[0].PriceHour != 0.0075 {
		t.Errorf("sizes[0].PriceHour = %f", sizes[0].PriceHour)
	}
}

func TestLinodeCov_Create_Success(t *testing.T) {
	srv := mockLinodeServer(t)
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	inst, err := l.Create(context.Background(), core.VPSCreateOpts{
		Name:     "linode-test",
		Region:   "us-east",
		Size:     "g6-nanode-1",
		Image:    "linode/ubuntu22.04",
		UserData: "#!/bin/bash",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID != "99999" {
		t.Errorf("ID = %q, want %q", inst.ID, "99999")
	}
	if inst.Name != "linode-test" {
		t.Errorf("Name = %q", inst.Name)
	}
	if inst.IPAddress != "9.10.11.12" {
		t.Errorf("IPAddress = %q", inst.IPAddress)
	}
	if inst.Status != "provisioning" {
		t.Errorf("Status = %q", inst.Status)
	}
	if inst.Region != "us-east" {
		t.Errorf("Region = %q", inst.Region)
	}
}

func TestLinodeCov_Create_NoIPv4(t *testing.T) {
	// Test the "no IPv4" branch in Linode Create
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": 11111, "label": "no-ip-test",
			"ipv4": []string{}, "status": "provisioning",
		})
	}))
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	inst, err := l.Create(context.Background(), core.VPSCreateOpts{
		Name:   "no-ip-test",
		Region: "us-east",
		Size:   "g6-nanode-1",
		Image:  "linode/ubuntu22.04",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.IPAddress != "" {
		t.Errorf("IPAddress = %q, want empty for no-ipv4 response", inst.IPAddress)
	}
}

func TestLinodeCov_Status_Success(t *testing.T) {
	srv := mockLinodeServer(t)
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	status, err := l.Status(context.Background(), "99999")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status != "running" {
		t.Errorf("Status = %q, want %q", status, "running")
	}
}

func TestLinodeCov_Delete_Success(t *testing.T) {
	srv := mockLinodeServer(t)
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	if err := l.Delete(context.Background(), "99999"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestLinodeCov_HTTP400_Error(t *testing.T) {
	srv := mockLinodeServer(t)
	defer srv.Close()

	l := &Linode{
		token:  "wrong-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	_, err := l.ListRegions(context.Background())
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	if !strings.Contains(err.Error(), "linode API") {
		t.Errorf("error = %q, want to contain 'linode API'", err.Error())
	}
}

// =============================================================================
// do() method edge cases — covers payload serialization, HTTP 4xx branches
// =============================================================================

func TestHetznerCov_DoWithPayload(t *testing.T) {
	// Test the do() method with a non-nil payload to cover the json.Marshal branch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	body, err := h.do(context.Background(), http.MethodPost, "/test", map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestHetznerCov_DoNilPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	h := &Hetzner{
		token:  "test-token",
		client: rewriteClient(srv.URL, hetznerAPI),
	}

	body, err := h.do(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestDOCov_DoHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	_, err := d.do(context.Background(), http.MethodGet, "/fail", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %q, want to contain '500'", err.Error())
	}
}

func TestVultrCov_DoHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	_, err := v.do(context.Background(), http.MethodGet, "/fail", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

func TestLinodeCov_DoHTTP500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	_, err := l.do(context.Background(), http.MethodGet, "/fail", nil)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
}

// =============================================================================
// Table-driven: all providers Create with error (HTTP 4xx from mock)
// =============================================================================

func TestAllProviders_Create_HTTP4xx(t *testing.T) {
	// Server that always returns 422
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"validation failed"}`, http.StatusUnprocessableEntity)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		provider core.VPSProvisioner
	}{
		{
			name: "hetzner",
			provider: &Hetzner{
				token:  "test-token",
				client: rewriteClient(srv.URL, hetznerAPI),
			},
		},
		{
			name: "digitalocean",
			provider: &DigitalOcean{
				token:  "test-token",
				client: rewriteClient(srv.URL, doAPI),
			},
		},
		{
			name: "vultr",
			provider: &Vultr{
				token:  "test-token",
				client: rewriteClient(srv.URL, vultrAPI),
			},
		},
		{
			name: "linode",
			provider: &Linode{
				token:  "test-token",
				client: rewriteClient(srv.URL, linodeAPI),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.provider.Create(context.Background(), core.VPSCreateOpts{
				Name: "fail-test", Region: "r1", Size: "s1", Image: "img",
			})
			if err == nil {
				t.Error("expected error for HTTP 422")
			}
		})
	}
}

// =============================================================================
// Table-driven: all providers ListRegions with error
// =============================================================================

func TestAllProviders_ListRegions_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		provider core.VPSProvisioner
	}{
		{"hetzner", &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}},
		{"digitalocean", &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}},
		{"vultr", &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}},
		{"linode", &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.provider.ListRegions(context.Background())
			if err == nil {
				t.Error("expected error for HTTP 403")
			}
		})
	}
}

// =============================================================================
// Table-driven: all providers ListSizes with error
// =============================================================================

func TestAllProviders_ListSizes_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		provider core.VPSProvisioner
	}{
		{"hetzner", &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}},
		{"digitalocean", &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}},
		{"vultr", &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}},
		{"linode", &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.provider.ListSizes(context.Background(), "region1")
			if err == nil {
				t.Error("expected error for HTTP 403")
			}
		})
	}
}

// =============================================================================
// Table-driven: all providers Status with error
// =============================================================================

func TestAllProviders_Status_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		provider core.VPSProvisioner
	}{
		{"hetzner", &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}},
		{"digitalocean", &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}},
		{"vultr", &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}},
		{"linode", &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.provider.Status(context.Background(), "12345")
			if err == nil {
				t.Error("expected error for HTTP 404")
			}
		})
	}
}

// =============================================================================
// Table-driven: all providers Delete with error
// =============================================================================

func TestAllProviders_Delete_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tests := []struct {
		name     string
		provider core.VPSProvisioner
	}{
		{"hetzner", &Hetzner{token: "t", client: rewriteClient(srv.URL, hetznerAPI)}},
		{"digitalocean", &DigitalOcean{token: "t", client: rewriteClient(srv.URL, doAPI)}},
		{"vultr", &Vultr{token: "t", client: rewriteClient(srv.URL, vultrAPI)}},
		{"linode", &Linode{token: "t", client: rewriteClient(srv.URL, linodeAPI)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Delete(context.Background(), "12345")
			if err == nil {
				t.Error("expected error for HTTP 404")
			}
		})
	}
}

// =============================================================================
// post() method coverage — ensures payload goes through json.Marshal in do()
// =============================================================================

func TestDOCov_PostWithPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"droplet":{"id":1,"name":"x","status":"new"}}`)
	}))
	defer srv.Close()

	d := &DigitalOcean{
		token:  "test-token",
		client: rewriteClient(srv.URL, doAPI),
	}

	body, err := d.post(context.Background(), "/droplets", map[string]string{"name": "x"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestVultrCov_PostWithPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"instance":{"id":"abc","label":"x","status":"pending"}}`)
	}))
	defer srv.Close()

	v := &Vultr{
		token:  "test-token",
		client: rewriteClient(srv.URL, vultrAPI),
	}

	body, err := v.post(context.Background(), "/instances", map[string]string{"label": "x"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestLinodeCov_PostWithPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"label":"x","ipv4":["1.2.3.4"],"status":"provisioning"}`)
	}))
	defer srv.Close()

	l := &Linode{
		token:  "test-token",
		client: rewriteClient(srv.URL, linodeAPI),
	}

	body, err := l.post(context.Background(), "/linode/instances", map[string]string{"label": "x"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

// =============================================================================
// do() invalid method — triggers http.NewRequestWithContext error branch
// =============================================================================

func TestHetznerCov_Do_InvalidMethod(t *testing.T) {
	h := &Hetzner{token: "t", client: http.DefaultClient}
	_, err := h.do(context.Background(), "BAD METHOD", "/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid HTTP method")
	}
}

func TestDOCov_Do_InvalidMethod(t *testing.T) {
	d := &DigitalOcean{token: "t", client: http.DefaultClient}
	_, err := d.do(context.Background(), "BAD METHOD", "/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid HTTP method")
	}
}

func TestVultrCov_Do_InvalidMethod(t *testing.T) {
	v := &Vultr{token: "t", client: http.DefaultClient}
	_, err := v.do(context.Background(), "BAD METHOD", "/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid HTTP method")
	}
}

func TestLinodeCov_Do_InvalidMethod(t *testing.T) {
	l := &Linode{token: "t", client: http.DefaultClient}
	_, err := l.do(context.Background(), "BAD METHOD", "/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid HTTP method")
	}
}
