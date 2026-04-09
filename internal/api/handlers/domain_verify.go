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

	var req struct {
		FQDN string `json:"fqdn"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// If FQDN not provided in body, try to look it up from stored records
	if req.FQDN == "" {
		var record domainVerifyRecord
		if err := h.bolt.Get("domain_verify", domainID, &record); err == nil && record.FQDN != "" {
			req.FQDN = record.FQDN
		}
	}

	if req.FQDN == "" {
		writeError(w, http.StatusBadRequest, "fqdn required")
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
	var req struct {
		FQDNs []string `json:"fqdns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	results := make([]VerifyResult, len(req.FQDNs))
	for i, fqdn := range req.FQDNs {
		results[i] = verifyDNS(fqdn)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"total":   len(results),
	})
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
