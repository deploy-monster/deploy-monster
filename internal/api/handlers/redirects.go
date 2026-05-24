package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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

var (
	errRedirectLimitReached = errors.New("redirect rule limit reached")
	errRedirectListMissing  = errors.New("redirect list missing")
)

func validateRedirectRule(rule RedirectRule) error {
	if rule.Source == "" || rule.Destination == "" {
		return fmt.Errorf("source and destination required")
	}
	if len(rule.Source) > 2048 {
		return fmt.Errorf("source must be 2048 characters or less")
	}
	if len(rule.Destination) > 2048 {
		return fmt.Errorf("destination must be 2048 characters or less")
	}
	if strings.ContainsAny(rule.Source, "\r\n") || strings.ContainsAny(rule.Destination, "\r\n") {
		return fmt.Errorf("source and destination must not contain control characters")
	}
	if !strings.HasPrefix(rule.Source, "/") || strings.HasPrefix(rule.Source, "//") {
		return fmt.Errorf("source must be an absolute path")
	}

	ruleType := rule.Type
	if ruleType == "" {
		ruleType = "redirect"
	}
	switch ruleType {
	case "redirect", "rewrite":
	default:
		return fmt.Errorf("type must be redirect or rewrite")
	}

	destinationIsPath := strings.HasPrefix(rule.Destination, "/") && !strings.HasPrefix(rule.Destination, "//")
	if ruleType == "rewrite" {
		if !destinationIsPath {
			return fmt.Errorf("rewrite destination must be an absolute path")
		}
		return nil
	}
	if destinationIsPath {
		return nil
	}

	u, err := url.Parse(rule.Destination)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("destination must be an absolute path or http(s) URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("destination URL must use http or https")
	}
	return nil
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

	if err := validateRedirectRule(rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if rule.Type == "" {
		rule.Type = "redirect"
	}
	if rule.StatusCode == 0 {
		rule.StatusCode = 301
	}
	switch rule.StatusCode {
	case 301, 302, 307, 308:
	default:
		writeError(w, http.StatusBadRequest, "status_code must be 301, 302, 307, or 308")
		return
	}
	rule.ID = core.GenerateID()
	rule.Enabled = true

	var list redirectList
	err := mutateBoltValue(h.bolt, "redirects", appID, &list, 0, func(_ bool) error {
		if len(list.Rules) >= 200 {
			return errRedirectLimitReached
		}
		list.Rules = append(list.Rules, rule)
		return nil
	})
	if errors.Is(err, errRedirectLimitReached) {
		writeError(w, http.StatusConflict, "redirect rule limit reached (200 per app)")
		return
	}
	if err != nil {
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
	err := mutateBoltValue(h.bolt, "redirects", appID, &list, 0, func(exists bool) error {
		if !exists {
			return errRedirectListMissing
		}
		filtered := make([]RedirectRule, 0, len(list.Rules))
		for _, r := range list.Rules {
			if r.ID != ruleID {
				filtered = append(filtered, r)
			}
		}
		list.Rules = filtered
		return nil
	})
	if errors.Is(err, errRedirectListMissing) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update redirects")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventRedirectDeleted, "api",
			map[string]string{"app_id": appID, "rule_id": ruleID}))
	}

	w.WriteHeader(http.StatusNoContent)
}
