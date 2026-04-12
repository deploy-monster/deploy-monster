package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// MarketplaceHandler handles marketplace endpoints.
type MarketplaceHandler struct {
	registry *marketplace.TemplateRegistry
}

func NewMarketplaceHandler(registry *marketplace.TemplateRegistry) *MarketplaceHandler {
	return &MarketplaceHandler{registry: registry}
}

// List handles GET /api/v1/marketplace
func (h *MarketplaceHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0, "categories": []string{}})
		return
	}

	category := r.URL.Query().Get("category")
	query := r.URL.Query().Get("q")

	var templates []*marketplace.Template
	if query != "" {
		templates = h.registry.Search(query)
	} else {
		templates = h.registry.List(category)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":       templates,
		"total":      len(templates),
		"categories": h.registry.Categories(),
	})
}

// Get handles GET /api/v1/marketplace/{slug}
func (h *MarketplaceHandler) Get(w http.ResponseWriter, r *http.Request) {
	slug, ok := requirePathParam(w, r, "slug")
	if !ok {
		return
	}
	tmpl := h.registry.Get(slug)
	if tmpl == nil {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, tmpl)
}
