package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DomainVerifyHandler manages DNS verification for domains.
type DomainVerifyHandler struct {
	store core.Store
}

func NewDomainVerifyHandler(store core.Store) *DomainVerifyHandler {
	return &DomainVerifyHandler{store: store}
}

// VerifyResult holds the DNS verification result for a domain.
type VerifyResult struct {
	FQDN       string   `json:"fqdn"`
	Verified   bool     `json:"verified"`
	Records    []string `json:"records,omitempty"`
	Error      string   `json:"error,omitempty"`
	CheckedAt  string   `json:"checked_at"`
}

// Verify handles POST /api/v1/domains/{id}/verify
func (h *DomainVerifyHandler) Verify(w http.ResponseWriter, r *http.Request) {
	domainID := r.PathValue("id")

	// Would look up domain by ID from store
	_ = domainID

	var req struct {
		FQDN string `json:"fqdn"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.FQDN == "" {
		writeError(w, http.StatusBadRequest, "fqdn required")
		return
	}

	result := verifyDNS(req.FQDN)
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
