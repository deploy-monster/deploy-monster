package handlers

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CertificateHandler manages SSL/TLS certificates.
type CertificateHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewCertificateHandler(store core.Store, bolt core.BoltStorer) *CertificateHandler {
	return &CertificateHandler{store: store, bolt: bolt}
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

// certStore wraps the persisted list of certificates.
type certStore struct {
	Certs []CertInfo `json:"certs"`
}

// List handles GET /api/v1/certificates
func (h *CertificateHandler) List(w http.ResponseWriter, _ *http.Request) {
	var cs certStore
	_ = h.bolt.Get("certificates", "all", &cs)

	if cs.Certs == nil {
		cs.Certs = []CertInfo{}
	}

	// Filter out expired certs from active status
	now := time.Now()
	for i := range cs.Certs {
		if cs.Certs[i].ExpiresAt.Before(now) && cs.Certs[i].Status == "active" {
			cs.Certs[i].Status = "expired"
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": cs.Certs, "total": len(cs.Certs)})
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

	// Validate cert/key pair
	cert, err := tls.X509KeyPair([]byte(req.CertPEM), []byte(req.KeyPEM))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid certificate/key pair")
		return
	}

	// Extract certificate info
	leaf := cert.Leaf
	issuer := "custom"
	var expiresAt time.Time
	if leaf != nil {
		issuer = leaf.Issuer.CommonName
		expiresAt = leaf.NotAfter
	}

	info := CertInfo{
		ID:        core.GenerateID(),
		Domain:    req.DomainID,
		Issuer:    issuer,
		ExpiresAt: expiresAt,
		AutoRenew: false,
		Status:    "active",
	}

	// Store cert data in BBolt
	certData := map[string]string{
		"cert_pem": req.CertPEM,
		"key_pem":  req.KeyPEM,
	}
	if err := h.bolt.Set("certificates", "data:"+info.ID, certData, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store certificate")
		return
	}

	// Add to cert list
	var cs certStore
	_ = h.bolt.Get("certificates", "all", &cs)
	cs.Certs = append(cs.Certs, info)
	if err := h.bolt.Set("certificates", "all", cs, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update certificate list")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        info.ID,
		"domain_id": req.DomainID,
		"issuer":    issuer,
		"status":    "active",
	})
}
