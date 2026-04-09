package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
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
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	Size     string `json:"size"`
	Image    string `json:"image"`
}

// ListProviders handles GET /api/v1/servers/providers
func (h *ServerHandler) ListProviders(w http.ResponseWriter, _ *http.Request) {
	providers := make([]map[string]string, 0)
	for _, name := range []string{"hetzner", "digitalocean", "vultr", "linode", "custom"} {
		p := h.services.VPSProvisioner(name)
		if p != nil {
			providers = append(providers, map[string]string{"id": name, "name": p.Name()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// ListRegions handles GET /api/v1/servers/providers/{provider}/regions
func (h *ServerHandler) ListRegions(w http.ResponseWriter, r *http.Request) {
	providerName, ok := requirePathParam(w, r, "provider")
	if !ok {
		return
	}
	p := h.services.VPSProvisioner(providerName)
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
	p := h.services.VPSProvisioner(providerName)
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

	p := h.services.VPSProvisioner(req.Provider)
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

	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventServerAdded, "api", claims.TenantID, claims.UserID,
		core.ServerEventData{
			ServerID: instance.ID,
			Hostname: instance.Name,
			IP:       instance.IPAddress,
		},
	))

	writeJSON(w, http.StatusCreated, instance)
}
