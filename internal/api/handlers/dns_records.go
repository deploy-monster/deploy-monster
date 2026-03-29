package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DNSRecordHandler manages individual DNS records.
type DNSRecordHandler struct {
	services *core.Services
}

func NewDNSRecordHandler(services *core.Services) *DNSRecordHandler {
	return &DNSRecordHandler{services: services}
}

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
		writeError(w, http.StatusInternalServerError, "DNS lookup failed: "+err.Error())
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
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if record.Name == "" || record.Value == "" || record.Type == "" {
		writeError(w, http.StatusBadRequest, "name, type, and value required")
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
		writeError(w, http.StatusInternalServerError, "failed to create DNS record: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, record)
}

// Delete handles DELETE /api/v1/dns/records/{id}?name=...
func (h *DNSRecordHandler) Delete(w http.ResponseWriter, r *http.Request) {
	recordID := r.PathValue("id")
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
		writeError(w, http.StatusInternalServerError, "failed: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
