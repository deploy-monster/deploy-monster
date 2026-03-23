package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CertificateHandler manages SSL/TLS certificates.
type CertificateHandler struct {
	store core.Store
}

func NewCertificateHandler(store core.Store) *CertificateHandler {
	return &CertificateHandler{store: store}
}

// CertInfo represents certificate information returned by the API.
type CertInfo struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	Issuer    string    `json:"issuer"`
	ExpiresAt time.Time `json:"expires_at"`
	AutoRenew bool      `json:"auto_renew"`
	Status    string    `json:"status"` // active, expired, pending
}

// List handles GET /api/v1/certificates
func (h *CertificateHandler) List(w http.ResponseWriter, r *http.Request) {
	// Would query ssl_certs table joined with domains
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

type uploadCertRequest struct {
	DomainID string `json:"domain_id"`
	CertPEM  string `json:"cert_pem"`
	KeyPEM   string `json:"key_pem"`
}

// Upload handles POST /api/v1/certificates
// Allows uploading custom SSL certificates.
func (h *CertificateHandler) Upload(w http.ResponseWriter, r *http.Request) {
	var req uploadCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DomainID == "" || req.CertPEM == "" || req.KeyPEM == "" {
		writeError(w, http.StatusBadRequest, "domain_id, cert_pem, and key_pem are required")
		return
	}

	// Validate cert/key pair would happen here (tls.X509KeyPair)

	writeJSON(w, http.StatusCreated, map[string]string{
		"status":    "uploaded",
		"domain_id": req.DomainID,
		"issuer":    "custom",
	})
}
