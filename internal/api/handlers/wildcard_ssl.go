package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WildcardSSLHandler manages wildcard SSL certificates via DNS-01 challenge.
type WildcardSSLHandler struct {
	store core.Store
}

func NewWildcardSSLHandler(store core.Store) *WildcardSSLHandler {
	return &WildcardSSLHandler{store: store}
}

// WildcardCertConfig defines a wildcard SSL request.
type WildcardCertConfig struct {
	Domain      string `json:"domain"`       // e.g., example.com
	Wildcard    string `json:"wildcard"`      // *.example.com
	DNSProvider string `json:"dns_provider"` // cloudflare, route53
	Status      string `json:"status"`       // pending, active, failed
}

// Request handles POST /api/v1/certificates/wildcard
func (h *WildcardSSLHandler) Request(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Domain      string `json:"domain"`
		DNSProvider string `json:"dns_provider"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain required")
		return
	}

	writeJSON(w, http.StatusAccepted, WildcardCertConfig{
		Domain:      req.Domain,
		Wildcard:    "*." + req.Domain,
		DNSProvider: req.DNSProvider,
		Status:      "pending",
	})
}
