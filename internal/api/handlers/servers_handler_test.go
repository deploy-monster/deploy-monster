package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Mock VPS Provisioner ────────────────────────────────────────────────────

type testVPSProvisioner struct {
	name       string
	regions    []core.VPSRegion
	sizes      []core.VPSSize
	createErr  error
	regionsErr error
	sizesErr   error
	created    *core.VPSInstance
}

func (p *testVPSProvisioner) Name() string { return p.name }

func (p *testVPSProvisioner) ListRegions(_ context.Context) ([]core.VPSRegion, error) {
	if p.regionsErr != nil {
		return nil, p.regionsErr
	}
	return p.regions, nil
}

func (p *testVPSProvisioner) ListSizes(_ context.Context, _ string) ([]core.VPSSize, error) {
	if p.sizesErr != nil {
		return nil, p.sizesErr
	}
	return p.sizes, nil
}

func (p *testVPSProvisioner) Create(_ context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	if p.createErr != nil {
		return nil, p.createErr
	}
	inst := &core.VPSInstance{
		ID:        core.GenerateID(),
		Name:      opts.Name,
		IPAddress: "203.0.113.1",
		Status:    "active",
		Region:    opts.Region,
		Size:      opts.Size,
	}
	p.created = inst
	return inst, nil
}

func (p *testVPSProvisioner) Delete(_ context.Context, _ string) error { return nil }
func (p *testVPSProvisioner) Status(_ context.Context, _ string) (string, error) {
	return "running", nil
}

// ─── List Providers ──────────────────────────────────────────────────────────

func TestListProviders_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", &testVPSProvisioner{name: "Hetzner Cloud"})
	services.RegisterVPSProvisioner("digitalocean", &testVPSProvisioner{name: "DigitalOcean"})

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers", nil)
	rr := httptest.NewRecorder()

	handler.ListProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) < 2 {
		t.Errorf("expected at least 2 providers, got %d", len(data))
	}
}

func TestListProviders_Empty(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers", nil)
	rr := httptest.NewRecorder()

	handler.ListProviders(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected 0 providers, got %d", len(data))
	}
}

// ─── List Regions ────────────────────────────────────────────────────────────

func TestListRegions_Success(t *testing.T) {
	prov := &testVPSProvisioner{
		name: "Hetzner Cloud",
		regions: []core.VPSRegion{
			{ID: "fsn1", Name: "Falkenstein"},
			{ID: "nbg1", Name: "Nuremberg"},
		},
	}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers/hetzner/regions", nil)
	req.SetPathValue("provider", "hetzner")
	rr := httptest.NewRecorder()

	handler.ListRegions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 regions, got %d", len(data))
	}
}

func TestListRegions_ProviderNotFound(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers/unknown/regions", nil)
	req.SetPathValue("provider", "unknown")
	rr := httptest.NewRecorder()

	handler.ListRegions(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "provider not found")
}

func TestListRegions_ProviderError(t *testing.T) {
	prov := &testVPSProvisioner{
		name:       "Hetzner Cloud",
		regionsErr: errors.New("api timeout"),
	}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers/hetzner/regions", nil)
	req.SetPathValue("provider", "hetzner")
	rr := httptest.NewRecorder()

	handler.ListRegions(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── List Sizes ──────────────────────────────────────────────────────────────

func TestListSizes_Success(t *testing.T) {
	prov := &testVPSProvisioner{
		name: "Hetzner Cloud",
		sizes: []core.VPSSize{
			{ID: "cx11", Name: "CX11", CPUs: 1},
			{ID: "cx21", Name: "CX21", CPUs: 2},
		},
	}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers/hetzner/sizes?region=fsn1", nil)
	req.SetPathValue("provider", "hetzner")
	rr := httptest.NewRecorder()

	handler.ListSizes(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 sizes, got %d", len(data))
	}
}

func TestListSizes_ProviderNotFound(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers/unknown/sizes", nil)
	req.SetPathValue("provider", "unknown")
	rr := httptest.NewRecorder()

	handler.ListSizes(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestListSizes_ProviderError(t *testing.T) {
	prov := &testVPSProvisioner{
		name:     "Hetzner Cloud",
		sizesErr: errors.New("api error"),
	}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers/providers/hetzner/sizes", nil)
	req.SetPathValue("provider", "hetzner")
	rr := httptest.NewRecorder()

	handler.ListSizes(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// ─── Provision ───────────────────────────────────────────────────────────────

func TestProvision_Success(t *testing.T) {
	prov := &testVPSProvisioner{name: "Hetzner Cloud"}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	body, _ := json.Marshal(provisionRequest{
		Provider: "hetzner",
		Name:     "my-server",
		Region:   "fsn1",
		Size:     "cx11",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Provision(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var inst core.VPSInstance
	json.Unmarshal(rr.Body.Bytes(), &inst)

	if inst.Name != "my-server" {
		t.Errorf("expected name my-server, got %q", inst.Name)
	}
	if inst.IPAddress == "" {
		t.Error("expected non-empty IP address")
	}
	if inst.ID == "" {
		t.Error("expected non-empty instance ID")
	}
}

func TestProvision_DefaultImage(t *testing.T) {
	prov := &testVPSProvisioner{name: "Hetzner Cloud"}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	body, _ := json.Marshal(provisionRequest{
		Provider: "hetzner",
		Name:     "server-no-image",
		Region:   "fsn1",
		Size:     "cx11",
		// Image intentionally omitted — should default to ubuntu-24.04
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Provision(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestProvision_NoClaims(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	body, _ := json.Marshal(provisionRequest{Provider: "hetzner", Name: "x", Region: "y", Size: "z"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Provision(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestProvision_InvalidJSON(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader([]byte("bad")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Provision(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestProvision_MissingFields(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	tests := []struct {
		name string
		body provisionRequest
	}{
		{"missing provider", provisionRequest{Name: "s", Region: "r", Size: "z"}},
		{"missing name", provisionRequest{Provider: "hetzner", Region: "r", Size: "z"}},
		{"missing region", provisionRequest{Provider: "hetzner", Name: "s", Size: "z"}},
		{"missing size", provisionRequest{Provider: "hetzner", Name: "s", Region: "r"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(body))
			req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
			rr := httptest.NewRecorder()

			handler.Provision(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
			assertErrorMessage(t, rr, "provider, name, region, and size are required")
		})
	}
}

func TestProvision_UnknownProvider(t *testing.T) {
	services := core.NewServices()
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	body, _ := json.Marshal(provisionRequest{
		Provider: "unknown",
		Name:     "server",
		Region:   "us-east",
		Size:     "small",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Provision(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unknown provider: unknown")
}

func TestProvision_ProviderError(t *testing.T) {
	prov := &testVPSProvisioner{
		name:      "Hetzner Cloud",
		createErr: errors.New("insufficient credits"),
	}
	services := core.NewServices()
	services.RegisterVPSProvisioner("hetzner", prov)

	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewServerHandler(store, services, events)

	body, _ := json.Marshal(provisionRequest{
		Provider: "hetzner",
		Name:     "server",
		Region:   "fsn1",
		Size:     "cx11",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/servers/provision", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Provision(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}
