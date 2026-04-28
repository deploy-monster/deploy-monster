package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

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

	if b.CustomCSS != "" {
		if err := validateCustomCSS(b.CustomCSS); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	h.store.SetPlatform(&b)
	writeJSON(w, http.StatusOK, &b)
}

// validateCustomCSS rejects dangerous CSS patterns to prevent XSS and data exfiltration.
func validateCustomCSS(css string) error {
	// Reject empty CSS
	css = strings.TrimSpace(css)
	if css == "" {
		return nil // empty is OK
	}

	// Reject style tag injection - attacker could escape the style context
	if strings.Contains(css, "<style") || strings.Contains(css, "</style") {
		return &validationError{"custom_css cannot contain <style> tags"}
	}

	// Reject IE expression() - allows JavaScript execution in old IE
	if strings.Contains(css, "expression(") {
		return &validationError{"custom_css cannot contain expression()"}
	}

	// Reject javascript: URLs in CSS (ould be used in url() properties)
	lower := strings.ToLower(css)
	if strings.Contains(lower, "javascript:") {
		return &validationError{"custom_css cannot contain javascript: URLs"}
	}

	// Reject data: URLs - could be used for MIME type confusion attacks
	if strings.Contains(lower, "data:") {
		return &validationError{"custom_css cannot contain data: URLs"}
	}

	// Reject import rules - could import malicious CSS
	if strings.Contains(lower, "@import") {
		return &validationError{"custom_css cannot contain @import"}
	}

	// Limit CSS size to prevent abuse
	if len(css) > 50000 {
		return &validationError{"custom_css exceeds 50KB limit"}
	}

	return nil
}

type validationError struct {
	msg string
}

func (e *validationError) Error() string {
	return e.msg
}
