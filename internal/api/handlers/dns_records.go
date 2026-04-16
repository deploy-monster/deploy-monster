package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
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
