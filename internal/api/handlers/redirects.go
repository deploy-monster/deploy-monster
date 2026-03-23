package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RedirectHandler manages per-app URL redirect/rewrite rules.
type RedirectHandler struct {
	store core.Store
}

func NewRedirectHandler(store core.Store) *RedirectHandler {
	return &RedirectHandler{store: store}
}

// RedirectRule defines a URL redirect or rewrite.
type RedirectRule struct {
	ID          string `json:"id"`
	Source      string `json:"source"`      // Path pattern: /old-path
	Destination string `json:"destination"` // Target: /new-path or https://other.com
	Type        string `json:"type"`        // redirect (301/302) or rewrite (internal)
	StatusCode  int    `json:"status_code"` // 301, 302, 307, 308
	Enabled     bool   `json:"enabled"`
}

// List handles GET /api/v1/apps/{id}/redirects
func (h *RedirectHandler) List(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// Create handles POST /api/v1/apps/{id}/redirects
func (h *RedirectHandler) Create(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var rule RedirectRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if rule.Source == "" || rule.Destination == "" {
		writeError(w, http.StatusBadRequest, "source and destination required")
		return
	}

	if rule.StatusCode == 0 {
		rule.StatusCode = 301
	}
	rule.ID = core.GenerateID()
	rule.Enabled = true

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id": appID,
		"rule":   rule,
	})
}

// Delete handles DELETE /api/v1/apps/{id}/redirects/{ruleId}
func (h *RedirectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
