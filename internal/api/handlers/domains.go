package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DomainHandler handles domain management endpoints.
type DomainHandler struct {
	store  core.Store
	events *core.EventBus
}

// NewDomainHandler creates a new domain handler.
func NewDomainHandler(store core.Store, events *core.EventBus) *DomainHandler {
	return &DomainHandler{store: store, events: events}
}

type createDomainRequest struct {
	AppID       string `json:"app_id"`
	FQDN        string `json:"fqdn"`
	DNSProvider string `json:"dns_provider"`
}

// List handles GET /api/v1/domains
func (h *DomainHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Get app ID filter if provided
	appID := r.URL.Query().Get("app_id")

	var domains []core.Domain
	var err error

	if appID != "" {
		domains, err = h.store.ListDomainsByApp(r.Context(), appID)
	} else {
		domains, err = h.store.ListAllDomains(r.Context())
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": domains, "total": len(domains)})
}

// Create handles POST /api/v1/domains
func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AppID == "" || req.FQDN == "" {
		writeError(w, http.StatusBadRequest, "app_id and fqdn are required")
		return
	}

	// Check if domain already exists
	if _, err := h.store.GetDomainByFQDN(r.Context(), req.FQDN); err == nil {
		writeError(w, http.StatusConflict, "domain already exists")
		return
	}

	dnsProvider := req.DNSProvider
	if dnsProvider == "" {
		dnsProvider = "manual"
	}

	domain := &core.Domain{
		AppID:       req.AppID,
		FQDN:        req.FQDN,
		Type:        "custom",
		DNSProvider: dnsProvider,
	}

	if err := h.store.CreateDomain(r.Context(), domain); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create domain")
		return
	}

	h.events.Publish(r.Context(), core.NewEvent(
		core.EventDomainAdded, "api",
		core.DomainEventData{
			DomainID: domain.ID,
			FQDN:     domain.FQDN,
			AppID:    domain.AppID,
		},
	))

	writeJSON(w, http.StatusCreated, domain)
}

// Delete handles DELETE /api/v1/domains/{id}
func (h *DomainHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := h.store.DeleteDomain(r.Context(), id); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "domain not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete domain")
		return
	}

	h.events.Publish(r.Context(), core.NewEvent(
		core.EventDomainRemoved, "api",
		map[string]string{"id": id},
	))

	w.WriteHeader(http.StatusNoContent)
}
