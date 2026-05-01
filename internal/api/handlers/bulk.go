package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// BulkHandler handles operations on multiple apps at once.
type BulkHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewBulkHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *BulkHandler {
	return &BulkHandler{store: store, runtime: runtime, events: events}
}

type bulkRequest struct {
	Action string   `json:"action"` // start, stop, restart, delete
	AppIDs []string `json:"app_ids"`
}

type bulkResult struct {
	AppID  string `json:"app_id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// Execute handles POST /api/v1/apps/bulk
func (h *BulkHandler) Execute(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Action == "" || len(req.AppIDs) == 0 {
		writeError(w, http.StatusBadRequest, "action and app_ids are required")
		return
	}

	if len(req.AppIDs) > 50 {
		writeError(w, http.StatusBadRequest, "max 50 apps per bulk operation")
		return
	}

	results := make([]bulkResult, len(req.AppIDs))
	completed := make([]struct {
		appID    string
		original string // original status before this bulk op
	}, 0)

	// Collect original statuses before making any changes.
	// This enables rollback if a later operation fails.
	appOriginalStatus := make(map[string]string)
	appTenantMap := make(map[string]string) // track per-app tenant for error reporting
	for _, appID := range req.AppIDs {
		if app, err := h.store.GetApp(r.Context(), appID); err == nil {
			appTenantMap[appID] = app.TenantID
			if app.TenantID == claims.TenantID {
				appOriginalStatus[appID] = app.Status
			}
		}
	}

	succeeded := 0

	for i, appID := range req.AppIDs {
		results[i] = bulkResult{AppID: appID}

		// Check if app exists and belongs to tenant
		if _, exists := appTenantMap[appID]; !exists {
			results[i].Status = "error"
			results[i].Error = "app not found"
			continue
		}
		if appTenantMap[appID] != claims.TenantID {
			results[i].Status = "error"
			results[i].Error = "access denied"
			continue
		}

		switch req.Action {
		case "start":
			if err := h.store.UpdateAppStatus(r.Context(), appID, "running"); err != nil {
				results[i].Status = "error"
				results[i].Error = sanitizeError(err)
			} else {
				results[i].Status = "started"
				completed = append(completed, struct{ appID, original string }{appID, appOriginalStatus[appID]})
			}
		case "stop":
			if err := h.store.UpdateAppStatus(r.Context(), appID, "stopped"); err != nil {
				results[i].Status = "error"
				results[i].Error = sanitizeError(err)
			} else {
				results[i].Status = "stopped"
				completed = append(completed, struct{ appID, original string }{appID, appOriginalStatus[appID]})
			}
		case "restart":
			origStatus := appOriginalStatus[appID]
			// Restart = stop + start. If start fails, rollback to stopped.
			if err := h.store.UpdateAppStatus(r.Context(), appID, "stopped"); err != nil {
				results[i].Status = "error"
				results[i].Error = sanitizeError(err)
			} else {
				// Track stop as completed (for rollback)
				completed = append(completed, struct{ appID, original string }{appID, origStatus})
				if startErr := h.store.UpdateAppStatus(r.Context(), appID, "running"); startErr != nil {
					// Rollback to original status
					h.store.UpdateAppStatus(r.Context(), appID, origStatus)
					results[i].Status = "error"
					results[i].Error = "restart failed"
					// Remove from completed (rollback happened)
					completed = completed[:len(completed)-1]
				} else {
					results[i].Status = "restarted"
				}
			}
		case "delete":
			if err := h.store.DeleteApp(r.Context(), appID); err != nil {
				results[i].Status = "error"
				results[i].Error = sanitizeError(err)
			} else {
				results[i].Status = "deleted"
				// No rollback for delete — can't undo
			}
		default:
			results[i].Status = "error"
			results[i].Error = "unknown action: " + req.Action
		}

		// Rollback: if any operation fails (after at least one succeeded),
		// reverse all already-completed changes to maintain consistency.
		if results[i].Status == "error" && succeeded > 0 {
			for _, done := range completed {
				if orig, ok := appOriginalStatus[done.appID]; ok {
					h.store.UpdateAppStatus(r.Context(), done.appID, orig)
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"results":     results,
				"total":       len(results),
				"succeeded":   0,
				"failed":      len(results),
				"rolled_back": true,
				"message":     "operation failed — already-completed apps have been rolled back",
			})
			return
		}

		if results[i].Status != "" && results[i].Status != "error" {
			succeeded++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results":     results,
		"total":       len(results),
		"succeeded":   succeeded,
		"failed":      len(results) - succeeded,
		"rolled_back": false,
	})
}

// sanitizeError removes potentially sensitive information from error messages.
// SECURITY FIX (AUTHZ-005): Sanitizes error messages to prevent information leakage.
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	// Sanitize common database error patterns that might leak internal details
	if strContains(msg, "sql") || strContains(msg, "connection refused") ||
		strContains(msg, "timeout") || strContains(msg, "deadline exceeded") {
		return "operation failed"
	}

	// Sanitize file system errors
	if strContains(msg, "no such file") || strContains(msg, "permission denied") {
		return "operation failed"
	}

	// Return generic error for internal errors
	if strContains(msg, "internal") || strContains(msg, "panic") {
		return "internal error"
	}

	// For other errors, return a generic message
	return "operation failed"
}

func strContains(s, substr string) bool {
	return len(s) >= len(substr) && containsCaseInsensitive(s, substr)
}

func containsCaseInsensitive(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		strContain(s, substr) || strContain(s, toUpper(substr)) || strContain(s, toLower(substr)))
}

func strContain(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toUpper(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c = c - 'a' + 'A'
		}
		result[i] = c
	}
	return string(result)
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c - 'A' + 'a'
		}
		result[i] = c
	}
	return string(result)
}
