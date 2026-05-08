package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	vpsproviders "github.com/deploy-monster/deploy-monster/internal/vps/providers"
)

// ServerHandler manages server/VPS provisioning.
type ServerHandler struct {
	store    core.Store
	services *core.Services
	events   *core.EventBus
}

func NewServerHandler(store core.Store, services *core.Services, events *core.EventBus) *ServerHandler {
	return &ServerHandler{store: store, services: services, events: events}
}

type provisionRequest struct {
	Provider  string `json:"provider"`
	Name      string `json:"name"`
	Hostname  string `json:"hostname"`
	Region    string `json:"region"`
	Size      string `json:"size"`
	Image     string `json:"image"`
	IPAddress string `json:"ip_address"`
}

type serverNode struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	IPAddress string    `json:"ip_address"`
	Provider  string    `json:"provider"`
	Region    string    `json:"region"`
	Size      string    `json:"size"`
	Status    string    `json:"status"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// ListProviders handles GET /api/v1/servers/providers
func (h *ServerHandler) ListProviders(w http.ResponseWriter, _ *http.Request) {
	providers := make([]map[string]string, 0)
	for _, name := range []string{"hetzner", "digitalocean", "vultr", "linode", "custom"} {
		p := h.provider(name)
		if p != nil {
			providers = append(providers, map[string]string{"id": name, "name": p.Name()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// List handles GET /api/v1/servers.
// Always includes a synthetic "local" entry for the master process so the UI
// can render the platform's own node alongside any provisioned worker hosts.
func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "local"
	}

	out := []serverNode{{
		ID:        "local",
		Hostname:  hostname,
		Provider:  "local",
		Region:    "local",
		Size:      "local",
		Status:    "active",
		Role:      "master",
		CreatedAt: time.Now(),
	}}

	stored, err := h.store.ListServersByTenant(r.Context(), claims.TenantID)
	if err != nil {
		ctxLogger(r.Context()).Error("servers list: store query failed", "error", err)
	}
	for _, srv := range stored {
		out = append(out, serverNode{
			ID:        srv.ID,
			Hostname:  srv.Hostname,
			IPAddress: srv.IPAddress,
			Provider:  srv.ProviderType,
			Region:    srv.Region,
			Size:      srv.Size,
			Status:    srv.Status,
			Role:      srv.Role,
			CreatedAt: srv.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  out,
		"total": len(out),
	})
}

// ListRegions handles GET /api/v1/servers/providers/{provider}/regions
func (h *ServerHandler) ListRegions(w http.ResponseWriter, r *http.Request) {
	providerName, ok := requirePathParam(w, r, "provider")
	if !ok {
		return
	}
	p := h.provider(providerName)
	if p == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	regions, err := p.ListRegions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list regions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": regions})
}

// ListSizes handles GET /api/v1/servers/providers/{provider}/sizes
func (h *ServerHandler) ListSizes(w http.ResponseWriter, r *http.Request) {
	providerName, ok := requirePathParam(w, r, "provider")
	if !ok {
		return
	}
	p := h.provider(providerName)
	if p == nil {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}

	sizes, err := p.ListSizes(r.Context(), r.URL.Query().Get("region"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list sizes")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": sizes})
}

// Create handles POST /api/v1/servers.
func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	h.Provision(w, r)
}

// Provision handles POST /api/v1/servers/provision
func (h *ServerHandler) Provision(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req provisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		req.Name = req.Hostname
	}
	if req.Provider == "custom" {
		if req.Region == "" {
			req.Region = "custom"
		}
		if req.Size == "" {
			req.Size = "custom"
		}
		if req.IPAddress == "" {
			writeError(w, http.StatusBadRequest, "ip_address is required for custom servers")
			return
		}
	}

	if req.Provider == "" || req.Name == "" || req.Region == "" || req.Size == "" {
		writeError(w, http.StatusBadRequest, "provider, name, region, and size are required")
		return
	}

	var fieldErrs []FieldError
	if len(req.Name) > 100 {
		fieldErrs = append(fieldErrs, FieldError{Field: "name", Message: "must be 100 characters or fewer"})
	}
	if len(req.Provider) > 50 {
		fieldErrs = append(fieldErrs, FieldError{Field: "provider", Message: "must be 50 characters or fewer"})
	}
	if len(req.Region) > 50 {
		fieldErrs = append(fieldErrs, FieldError{Field: "region", Message: "must be 50 characters or fewer"})
	}
	if len(req.Size) > 50 {
		fieldErrs = append(fieldErrs, FieldError{Field: "size", Message: "must be 50 characters or fewer"})
	}
	if len(req.Image) > 100 {
		fieldErrs = append(fieldErrs, FieldError{Field: "image", Message: "must be 100 characters or fewer"})
	}
	if len(fieldErrs) > 0 {
		writeValidationErrors(w, "field validation failed", fieldErrs)
		return
	}

	p := h.provider(req.Provider)
	if p == nil {
		writeError(w, http.StatusBadRequest, "unknown provider: "+req.Provider)
		return
	}

	image := req.Image
	if image == "" {
		image = "ubuntu-24.04"
	}

	instance, err := p.Create(r.Context(), core.VPSCreateOpts{
		Name:   req.Name,
		Region: req.Region,
		Size:   req.Size,
		Image:  image,
	})
	if err != nil {
		internalErrorCtx(r.Context(), w, "provisioning failed", err)
		return
	}
	if instance.IPAddress == "" && req.IPAddress != "" {
		instance.IPAddress = req.IPAddress
	}

	// Persist so List/Get reflect the provisioned host. Custom (BYOH) servers
	// are immediately "active"; cloud-provisioned ones start "provisioning"
	// and the agent bootstrap flow flips them to "active" later.
	status := "provisioning"
	if req.Provider == "custom" {
		status = "active"
	}
	srv := &core.Server{
		ID:           instance.ID,
		TenantID:     claims.TenantID,
		Hostname:     instance.Name,
		IPAddress:    instance.IPAddress,
		Role:         "worker",
		ProviderType: req.Provider,
		ProviderRef:  instance.ID,
		Region:       req.Region,
		Size:         req.Size,
		SSHPort:      22,
		Status:       status,
		AgentStatus:  "unknown",
	}
	if err := h.store.CreateServer(r.Context(), srv); err != nil {
		ctxLogger(r.Context()).Error("servers provision: persist failed", "error", err, "instance_id", instance.ID)
		internalErrorCtx(r.Context(), w, "failed to persist server record", err)
		return
	}

	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventServerAdded, "api", claims.TenantID, claims.UserID,
		core.ServerEventData{
			ServerID: srv.ID,
			Hostname: srv.Hostname,
			IP:       srv.IPAddress,
		},
	))

	writeJSON(w, http.StatusCreated, serverNode{
		ID:        srv.ID,
		Hostname:  srv.Hostname,
		IPAddress: srv.IPAddress,
		Provider:  srv.ProviderType,
		Region:    srv.Region,
		Size:      srv.Size,
		Status:    srv.Status,
		Role:      srv.Role,
		CreatedAt: time.Now(),
	})
}

// Delete handles DELETE /api/v1/servers/{id}.
func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	if id == "local" {
		writeError(w, http.StatusBadRequest, "cannot delete the master node")
		return
	}
	srv, err := h.store.GetServer(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "server not found")
		return
	}
	if err != nil {
		internalErrorCtx(r.Context(), w, "lookup failed", err)
		return
	}
	if srv.TenantID == "" {
		writeError(w, http.StatusForbidden, "shared server cannot be deleted from tenant scope")
		return
	}
	if srv.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "not your server")
		return
	}
	if err := h.store.DeleteServer(r.Context(), id); err != nil {
		internalErrorCtx(r.Context(), w, "delete failed", err)
		return
	}
	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventServerRemoved, "api", claims.TenantID, claims.UserID,
		core.ServerEventData{ServerID: id, Hostname: srv.Hostname, IP: srv.IPAddress},
	))
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServerHandler) provider(name string) core.VPSProvisioner {
	if h.services != nil {
		if p := h.services.VPSProvisioner(name); p != nil {
			return p
		}
	}
	if name == "custom" {
		if factory, ok := vpsproviders.Registry[name]; ok {
			return factory("")
		}
	}
	return nil
}
