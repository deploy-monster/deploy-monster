package handlers

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── domain_verify.go ────────────────────
// DomainVerifyHandler manages DNS verification for domains.
type DomainVerifyHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewDomainVerifyHandler(store core.Store, kv core.KVStorer) *DomainVerifyHandler {
	return &DomainVerifyHandler{store: store, kv: kv}
}

// VerifyResult holds the DNS verification result for a domain.
type VerifyResult struct {
	FQDN      string   `json:"fqdn"`
	Verified  bool     `json:"verified"`
	Records   []string `json:"records,omitempty"`
	Error     string   `json:"error,omitempty"`
	CheckedAt string   `json:"checked_at"`
}

// domainVerifyRecord persisted in KV storage for audit/history.
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
	if !decodeOptionalJSONInto(w, r, &req) { // FQDN may be omitted
		return
	}
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
	if err := h.kv.Set("domain_verify", domainID, record, 0); err != nil {
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
	if !decodeJSONInto(w, r, &req) {
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
		if errors.Is(err, core.ErrNotFound) {
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
	if len(apps) == 0 {
		return make(map[string]bool), true
	}
	// Batch-fetch domains for all apps in a single query to avoid N+1.
	appIDs := make([]string, len(apps))
	for i, app := range apps {
		appIDs[i] = app.ID
	}
	domainsByApp, err := h.store.ListDomainsByAppIDs(r.Context(), appIDs, tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	allowed := make(map[string]bool)
	for _, app := range apps {
		for _, domain := range domainsByApp[app.ID] {
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

// ──────────────────── dns_records.go ────────────────────
// DNSRecordHandler manages individual DNS records.
type DNSRecordHandler struct {
	services *core.Services
	events   *core.EventBus
}

func NewDNSRecordHandler(services *core.Services) *DNSRecordHandler {
	return &DNSRecordHandler{services: services}
}

// SetEvents sets the event bus for audit event emission.
func (h *DNSRecordHandler) SetEvents(events *core.EventBus) { h.events = events }

// List handles GET /api/v1/dns/records?domain=example.com
func (h *DNSRecordHandler) List(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain query param required")
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = "cloudflare"
	}
	p := h.services.DNSProvider(provider)
	if p == nil {
		// No DNS provider configured — return empty list
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	verified, err := p.Verify(r.Context(), domain)
	if err != nil {
		internalErrorCtx(r.Context(), w, "DNS lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"domain":   domain,
		"provider": provider,
		"verified": verified,
		"data":     []any{},
		"total":    0,
	})
}

// Create handles POST /api/v1/dns/records
func (h *DNSRecordHandler) Create(w http.ResponseWriter, r *http.Request) {
	var record core.DNSRecord
	if !decodeJSONInto(w, r, &record) {
		return
	}
	if record.Name == "" || record.Value == "" || record.Type == "" {
		writeError(w, http.StatusBadRequest, "name, type, and value required")
		return
	}
	const maxNameLen = 253
	const maxValueLen = 2048
	if len(record.Name) > maxNameLen {
		writeError(w, http.StatusBadRequest, "name exceeds 253 characters")
		return
	}
	if len(record.Value) > maxValueLen {
		writeError(w, http.StatusBadRequest, "value exceeds 2048 characters")
		return
	}
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = "cloudflare"
	}
	p := h.services.DNSProvider(provider)
	if p == nil {
		writeError(w, http.StatusBadRequest, "DNS provider not configured: "+provider)
		return
	}
	if err := p.CreateRecord(r.Context(), record); err != nil {
		internalErrorCtx(r.Context(), w, "failed to create DNS record", err)
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

// Delete handles DELETE /api/v1/dns/records/{id}?name=...
func (h *DNSRecordHandler) Delete(w http.ResponseWriter, r *http.Request) {
	recordID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	name := r.URL.Query().Get("name")
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = "cloudflare"
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name query param required")
		return
	}
	p := h.services.DNSProvider(provider)
	if p == nil {
		writeError(w, http.StatusBadRequest, "DNS provider not configured")
		return
	}
	record := core.DNSRecord{ID: recordID, Name: name}
	if err := p.DeleteRecord(r.Context(), record); err != nil {
		internalErrorCtx(r.Context(), w, "failed to delete DNS record", err)
		return
	}
	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventDNSRecordDeleted, "api",
			map[string]string{"id": recordID, "name": name}))
	}
	w.WriteHeader(http.StatusNoContent)
}

// ──────────────────── ssl_status.go ────────────────────
// SSLStatusHandler checks SSL certificate status for domains.
type SSLStatusHandler struct {
	kv core.KVStorer
}

func NewSSLStatusHandler(kv core.KVStorer) *SSLStatusHandler {
	return &SSLStatusHandler{kv: kv}
}

// SSLCheckResult holds SSL verification details.
type SSLCheckResult struct {
	FQDN      string    `json:"fqdn"`
	Valid     bool      `json:"valid"`
	Issuer    string    `json:"issuer,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	DaysLeft  int       `json:"days_left,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// Check handles GET /api/v1/domains/{id}/ssl-status
func (h *SSLStatusHandler) Check(w http.ResponseWriter, r *http.Request) {
	fqdn := r.URL.Query().Get("fqdn")
	if fqdn == "" {
		writeError(w, http.StatusBadRequest, "fqdn query param required")
		return
	}
	// Check cache first (cache for 5 minutes)
	var cached SSLCheckResult
	if err := h.kv.Get("certificates", "ssl_check:"+fqdn, &cached); err == nil {
		writeJSON(w, http.StatusOK, cached)
		return
	}
	result := checkSSL(fqdn)
	// Cache the result for 5 minutes
	if err := h.kv.Set("certificates", "ssl_check:"+fqdn, result, 300); err != nil {
		slog.Error("failed to cache SSL check result", "fqdn", fqdn, "error", err)
	}
	writeJSON(w, http.StatusOK, result)
}
func checkSSL(fqdn string) SSLCheckResult {
	result := SSLCheckResult{
		FQDN:      fqdn,
		CheckedAt: time.Now(),
	}
	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp", fqdn+":443",
		&tls.Config{InsecureSkipVerify: false},
	)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer conn.Close()
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) > 0 {
		cert := certs[0]
		result.Valid = true
		result.Issuer = cert.Issuer.CommonName
		result.Subject = cert.Subject.CommonName
		result.ExpiresAt = cert.NotAfter
		result.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	}
	return result
}

// ──────────────────── wildcard_ssl.go ────────────────────
// WildcardSSLHandler manages wildcard SSL certificates via DNS-01 challenge.
type WildcardSSLHandler struct {
	kv core.KVStorer
}

func NewWildcardSSLHandler(kv core.KVStorer) *WildcardSSLHandler {
	return &WildcardSSLHandler{kv: kv}
}

// WildcardCertConfig defines a wildcard SSL request.
type WildcardCertConfig struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	Domain      string `json:"domain"`       // e.g., example.com
	Wildcard    string `json:"wildcard"`     // *.example.com
	DNSProvider string `json:"dns_provider"` // cloudflare, route53
	Status      string `json:"status"`       // pending, active, failed
}

// Request handles POST /api/v1/certificates/wildcard
func (h *WildcardSSLHandler) Request(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Domain      string `json:"domain"`
		DNSProvider string `json:"dns_provider"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Domain == "" {
		writeError(w, http.StatusBadRequest, "domain required")
		return
	}
	certID := core.GenerateID()
	cfg := WildcardCertConfig{
		ID:          certID,
		TenantID:    claims.TenantID,
		Domain:      req.Domain,
		Wildcard:    "*." + req.Domain,
		DNSProvider: req.DNSProvider,
		Status:      "pending",
	}
	// Store the wildcard cert request
	if err := h.kv.Set("wildcard_ssl", claims.TenantID+":"+certID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save wildcard certificate request")
		return
	}
	// Also index by domain for lookups
	if err := h.kv.Set("wildcard_ssl_domain", claims.TenantID+":"+req.Domain, cfg, 0); err != nil {
		slog.Error("failed to index wildcard cert by domain", "domain", req.Domain, "error", err)
	}
	writeJSON(w, http.StatusAccepted, cfg)
}

// ──────────────────── certificates.go ────────────────────
// CertificateHandler manages SSL/TLS certificates.
type CertificateHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewCertificateHandler(store core.Store, kv core.KVStorer) *CertificateHandler {
	return &CertificateHandler{store: store, kv: kv}
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
	if err := h.kv.Get("certificates", "all", &cs); err != nil {
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
	if !decodeJSONInto(w, r, &req) {
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
	if cert.Leaf == nil && len(cert.Certificate) > 0 {
		if leaf, parseErr := x509.ParseCertificate(cert.Certificate[0]); parseErr == nil {
			cert.Leaf = leaf
		}
	}
	domain, ok := h.requireTenantCertificateDomain(w, r, req.DomainID, claims.TenantID)
	if !ok {
		return
	}
	// Verify the certificate domain matches the tenant-owned domain (UPLOAD-001).
	if err := validateCertDomain([]byte(req.CertPEM), domain.FQDN); err != nil {
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
		Domain:    domain.FQDN,
		Issuer:    issuer,
		ExpiresAt: expiresAt,
		AutoRenew: false,
		Status:    "active",
	}
	// Store cert data in KV storage.
	certData := map[string]string{
		"cert_pem": req.CertPEM,
		"key_pem":  req.KeyPEM,
	}
	if err := h.kv.Set("certificates", "data:"+info.ID, certData, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store certificate")
		return
	}
	// Add to cert list
	var cs certStore
	_ = h.kv.Get("certificates", "all", &cs)
	cs.Certs = append(cs.Certs, info)
	if err := h.kv.Set("certificates", "all", cs, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update certificate list")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":        info.ID,
		"domain_id": domain.ID,
		"issuer":    issuer,
		"status":    "active",
	})
}
func (h *CertificateHandler) requireTenantCertificateDomain(w http.ResponseWriter, r *http.Request, domainRef, tenantID string) (*core.Domain, bool) {
	domain, err := h.store.GetDomain(r.Context(), domainRef)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			domain, err = h.store.GetDomainByFQDN(r.Context(), domainRef)
		}
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				writeError(w, http.StatusNotFound, "domain not found")
				return nil, false
			}
			writeError(w, http.StatusInternalServerError, "failed to lookup domain")
			return nil, false
		}
	}
	app, err := h.store.GetApp(r.Context(), domain.AppID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lookup application")
		return nil, false
	}
	if app.TenantID != tenantID {
		writeError(w, http.StatusForbidden, "access denied")
		return nil, false
	}
	return domain, true
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

// ──────────────────── domains.go ────────────────────
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
		domains, err = h.store.ListDomainsByApp(r.Context(), appID, app.TenantID)
	} else {
		tenantApps, _, aerr := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 10000, 0)
		if aerr != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, app := range tenantApps {
			appDomains, derr := h.store.ListDomainsByApp(r.Context(), app.ID, claims.TenantID)
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
	if !decodeJSONInto(w, r, &req) {
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
	publishEvent(r.Context(), h.events, core.NewEvent(
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
	if err := h.store.DeleteDomain(r.Context(), id, claims.TenantID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "domain not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete domain")
		return
	}
	publishEvent(r.Context(), h.events, core.NewEvent(
		core.EventDomainRemoved, "api",
		map[string]string{"id": id},
	))
	w.WriteHeader(http.StatusNoContent)
}
