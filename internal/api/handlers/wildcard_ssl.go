package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// WildcardSSLHandler manages wildcard SSL certificates via DNS-01 challenge.
type WildcardSSLHandler struct {
	bolt core.BoltStorer
}

func NewWildcardSSLHandler(bolt core.BoltStorer) *WildcardSSLHandler {
	return &WildcardSSLHandler{bolt: bolt}
}

// WildcardCertConfig defines a wildcard SSL request.
type WildcardCertConfig struct {
	ID          string `json:"id"`
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

	certID := core.GenerateID()
	cfg := WildcardCertConfig{
		ID:          certID,
		Domain:      req.Domain,
		Wildcard:    "*." + req.Domain,
		DNSProvider: req.DNSProvider,
		Status:      "pending",
	}

	// Store the wildcard cert request
	if err := h.bolt.Set("wildcard_ssl", certID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save wildcard certificate request")
		return
	}

	// Also index by domain for lookups
	_ = h.bolt.Set("wildcard_ssl_domain", req.Domain, cfg, 0)

	writeJSON(w, http.StatusAccepted, cfg)
}
