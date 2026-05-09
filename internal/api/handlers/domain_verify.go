package handlers

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DomainVerifyHandler manages DNS verification for domains.
type DomainVerifyHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewDomainVerifyHandler(store core.Store, bolt core.BoltStorer) *DomainVerifyHandler {
	return &DomainVerifyHandler{store: store, bolt: bolt}
}

// VerifyResult holds the DNS verification result for a domain.
type VerifyResult struct {
	FQDN      string   `json:"fqdn"`
	Verified  bool     `json:"verified"`
	Records   []string `json:"records,omitempty"`
	Error     string   `json:"error,omitempty"`
	CheckedAt string   `json:"checked_at"`
}

// domainVerifyRecord persisted in BBolt for audit/history.
type domainVerifyRecord struct {
	DomainID  string `json:"domain_id"`
	FQDN      string `json:"fqdn"`
	Verified  bool   `json:"verified"`
	CheckedAt string `json:"checked_at"`
}

// Verify handles POST /api/v1/domains/{id}/verify
func (h *DomainVerifyHandler) Verify(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	domainID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

	domain, ok := h.requireTenantDomain(w, r, domainID, claims.TenantID)
	if !ok {
		return
	}

	var req struct {
		FQDN string `json:"fqdn"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // Intentionally lenient: FQDN may be omitted

	if req.FQDN == "" {
		req.FQDN = domain.FQDN
	}
	if req.FQDN == "" {
		writeError(w, http.StatusBadRequest, "fqdn required")
		return
	}
	if req.FQDN != domain.FQDN {
		writeError(w, http.StatusForbidden, "fqdn does not belong to this domain")
		return
	}

	result := verifyDNS(req.FQDN)

	// Persist the verification result
	record := domainVerifyRecord{
		DomainID:  domainID,
		FQDN:      req.FQDN,
		Verified:  result.Verified,
		CheckedAt: result.CheckedAt,
	}
	if err := h.bolt.Set("domain_verify", domainID, record, 0); err != nil {
		slog.Error("failed to persist domain verification", "domain_id", domainID, "error", err)
	}

	writeJSON(w, http.StatusOK, result)
}

// BatchVerify handles POST /api/v1/domains/verify-batch
func (h *DomainVerifyHandler) BatchVerify(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		FQDNs []string `json:"fqdns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	allowed, ok := h.tenantDomainSet(w, r, claims.TenantID)
	if !ok {
		return
	}

	results := make([]VerifyResult, 0, len(req.FQDNs))
	for _, fqdn := range req.FQDNs {
		if !allowed[fqdn] {
			writeError(w, http.StatusForbidden, "fqdn does not belong to this tenant")
			return
		}
		results = append(results, verifyDNS(fqdn))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   len(results),
	})
}

func (h *DomainVerifyHandler) requireTenantDomain(w http.ResponseWriter, r *http.Request, domainID, tenantID string) (*core.Domain, bool) {
	domain, err := h.store.GetDomain(r.Context(), domainID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "domain not found")
			return nil, false
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}

	app, err := h.store.GetApp(r.Context(), domain.AppID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if app.TenantID != tenantID {
		writeError(w, http.StatusForbidden, "access denied")
		return nil, false
	}

	return domain, true
}

func (h *DomainVerifyHandler) tenantDomainSet(w http.ResponseWriter, r *http.Request, tenantID string) (map[string]bool, bool) {
	apps, _, err := h.store.ListAppsByTenant(r.Context(), tenantID, 10000, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	allowed := make(map[string]bool)
	for _, app := range apps {
		domains, err := h.store.ListDomainsByApp(r.Context(), app.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return nil, false
		}
		for _, domain := range domains {
			allowed[domain.FQDN] = true
		}
	}
	return allowed, true
}

func verifyDNS(fqdn string) VerifyResult {
	result := VerifyResult{
		FQDN:      fqdn,
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	ips, err := net.LookupHost(fqdn)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Records = ips
	result.Verified = len(ips) > 0
	return result
}
