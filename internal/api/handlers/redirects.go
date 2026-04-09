package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// RedirectHandler manages per-app URL redirect/rewrite rules.
type RedirectHandler struct {
	store  core.Store
	bolt   core.BoltStorer
	events *core.EventBus
}

func NewRedirectHandler(store core.Store, bolt core.BoltStorer) *RedirectHandler {
	return &RedirectHandler{store: store, bolt: bolt}
}

// SetEvents sets the event bus for audit event emission.
func (h *RedirectHandler) SetEvents(events *core.EventBus) { h.events = events }

// RedirectRule defines a URL redirect or rewrite.
type RedirectRule struct {
	ID          string `json:"id"`
	Source      string `json:"source"`      // Path pattern: /old-path
	Destination string `json:"destination"` // Target: /new-path or https://other.com
	Type        string `json:"type"`        // redirect (301/302) or rewrite (internal)
	StatusCode  int    `json:"status_code"` // 301, 302, 307, 308
	Enabled     bool   `json:"enabled"`
}

// redirectList is the persisted list of redirect rules for an app.
type redirectList struct {
	Rules []RedirectRule `json:"rules"`
}

// List handles GET /api/v1/apps/{id}/redirects
func (h *RedirectHandler) List(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var list redirectList
	if err := h.bolt.Get("redirects", appID, &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": list.Rules, "total": len(list.Rules)})
}

// Create handles POST /api/v1/apps/{id}/redirects
func (h *RedirectHandler) Create(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var rule RedirectRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if rule.Source == "" || rule.Destination == "" {
		writeError(w, http.StatusBadRequest, "source and destination required")
		return
	}
	if len(rule.Source) > 2048 {
		writeError(w, http.StatusBadRequest, "source must be 2048 characters or less")
		return
	}
	if len(rule.Destination) > 2048 {
		writeError(w, http.StatusBadRequest, "destination must be 2048 characters or less")
		return
	}

	if rule.StatusCode == 0 {
		rule.StatusCode = 301
	}
	rule.ID = core.GenerateID()
	rule.Enabled = true

	// Load existing rules
	var list redirectList
	_ = h.bolt.Get("redirects", appID, &list)

	list.Rules = append(list.Rules, rule)

	if err := h.bolt.Set("redirects", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save redirect rule")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventRedirectCreated, "api",
			map[string]string{"app_id": appID, "rule_id": rule.ID}))
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id": appID,
		"rule":   rule,
	})
}

// Delete handles DELETE /api/v1/apps/{id}/redirects/{ruleId}
func (h *RedirectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	ruleID, ok := requirePathParam(w, r, "ruleId")
	if !ok {
		return
	}

	var list redirectList
	if err := h.bolt.Get("redirects", appID, &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	filtered := make([]RedirectRule, 0, len(list.Rules))
	for _, r := range list.Rules {
		if r.ID != ruleID {
			filtered = append(filtered, r)
		}
	}
	list.Rules = filtered

	if err := h.bolt.Set("redirects", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update redirects")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventRedirectDeleted, "api",
			map[string]string{"app_id": appID, "rule_id": ruleID}))
	}

	w.WriteHeader(http.StatusNoContent)
}
