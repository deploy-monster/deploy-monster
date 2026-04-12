package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/enterprise"
)

// BrandingHandler serves white-label branding configuration.
type BrandingHandler struct {
	store *enterprise.BrandingStore
}

func NewBrandingHandler() *BrandingHandler {
	return &BrandingHandler{
		store: enterprise.NewBrandingStore(),
	}
}

// Get handles GET /api/v1/branding
// Returns the current branding config for the frontend.
func (h *BrandingHandler) Get(w http.ResponseWriter, _ *http.Request) {
	branding := h.store.GetPlatform()
	writeJSON(w, http.StatusOK, branding)
}

// Update handles PATCH /api/v1/admin/branding
// Updates platform-level branding (super admin only).
func (h *BrandingHandler) Update(w http.ResponseWriter, r *http.Request) {
	var b enterprise.Branding
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	h.store.SetPlatform(&b)
	writeJSON(w, http.StatusOK, &b)
}
