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
		app, appErr := h.store.GetApp(r.Context(), appID)
		if appErr != nil {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		if app.TenantID != claims.TenantID {
			writeError(w, http.StatusForbidden, "access denied")
			return
		}
		domains, err = h.store.ListDomainsByApp(r.Context(), appID)
	} else {
		tenantApps, _, aerr := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 10000, 0)
		if aerr != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, app := range tenantApps {
			appDomains, derr := h.store.ListDomainsByApp(r.Context(), app.ID)
			if derr != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			domains = append(domains, appDomains...)
		}
		err = nil
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	pg := parsePagination(r)
	page, total := paginateSlice(domains, pg)
	writePaginatedJSON(w, page, total, pg)
}

// Create handles POST /api/v1/domains
func (h *DomainHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.AppID == "" || req.FQDN == "" {
		writeError(w, http.StatusBadRequest, "app_id and fqdn are required")
		return
	}

	// SECURITY: Verify the app belongs to this tenant
	app, err := h.store.GetApp(r.Context(), req.AppID)
	if err != nil {
		writeError(w, http.StatusNotFound, "application not found")
		return
	}
	if app.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var fieldErrs []FieldError
	if len(req.FQDN) > 253 {
		fieldErrs = append(fieldErrs, FieldError{Field: "fqdn", Message: "must be 253 characters or fewer"})
	}
	if len(req.DNSProvider) > 50 {
		fieldErrs = append(fieldErrs, FieldError{Field: "dns_provider", Message: "must be 50 characters or fewer"})
	}
	if len(fieldErrs) > 0 {
		writeValidationErrors(w, "field validation failed", fieldErrs)
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
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	// SECURITY: Verify the domain belongs to an app owned by this tenant
	domain, err := h.store.GetDomain(r.Context(), id)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "domain not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	app, err := h.store.GetApp(r.Context(), domain.AppID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if app.TenantID != claims.TenantID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

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
