package handlers

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
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
	TenantID  string    `json:"tenant_id"` // required for tenant isolation
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
func (h *CertificateHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var cs certStore
	if err := h.bolt.Get("certificates", "all", &cs); err != nil {
		// No certs stored yet — return empty list
		cs.Certs = []CertInfo{}
	}

	if cs.Certs == nil {
		cs.Certs = []CertInfo{}
	}

	// Filter out expired certs from active status and apply tenant isolation
	now := time.Now()
	filtered := make([]CertInfo, 0, len(cs.Certs))
	for i := range cs.Certs {
		if cs.Certs[i].ExpiresAt.Before(now) && cs.Certs[i].Status == "active" {
			cs.Certs[i].Status = "expired"
		}
		// Tenant isolation: only show certs belonging to this tenant
		if cs.Certs[i].TenantID == claims.TenantID {
			filtered = append(filtered, cs.Certs[i])
		}
	}
	cs.Certs = filtered

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
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

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

	// Verify the certificate domain matches domain_id (UPLOAD-001)
	if err := validateCertDomain([]byte(req.CertPEM), req.DomainID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
		TenantID:  claims.TenantID,
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

// validateCertDomain checks that the certificate's domains (SAN + CN) cover
// the domain_id being registered. This prevents uploading a certificate for
// a domain the user doesn't own (e.g., uploading an evil.com cert for
// example.com). Wildcard certs (*.example.com) are accepted if the
// domain_id is a subdomain of the wildcard pattern.
func validateCertDomain(certPEM []byte, domainID string) error {
	if domainID == "" {
		return nil
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return &validationError{msg: "certificate is not valid PEM"}
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return &validationError{msg: "failed to parse certificate: " + err.Error()}
	}

	if len(cert.DNSNames) == 0 && cert.Subject.CommonName == "" {
		return &validationError{msg: "certificate has no DNS names (SAN or CN)"}
	}

	if !certMatchesDomain(cert, domainID) {
		return &validationError{msg: "certificate does not match domain: " + domainID}
	}
	return nil
}

// certMatchesDomain returns true if the certificate covers the given domain.
// It checks SANs first, then CN as a fallback. Wildcard certs (*.example.com)
// match any subdomain of example.com.
func certMatchesDomain(cert *x509.Certificate, domain string) bool {
	domain = strings.ToLower(domain)

	// Check SANs (most reliable)
	for _, san := range cert.DNSNames {
		san = strings.ToLower(san)
		if san == domain {
			return true
		}
		if strings.HasPrefix(san, "*.") {
			pattern := san[2:]
			if strings.HasSuffix(domain, pattern) && domain != pattern {
				return true
			}
		}
	}

	// Fallback: check CommonName
	if cert.Subject.CommonName != "" {
		cn := strings.ToLower(cert.Subject.CommonName)
		if cn == domain {
			return true
		}
		if strings.HasPrefix(cn, "*.") {
			pattern := cn[2:]
			if strings.HasSuffix(domain, pattern) && domain != pattern {
				return true
			}
		}
	}

	return false
}

// validationError is already declared in branding.go
