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

	for i, appID := range req.AppIDs {
		results[i] = bulkResult{AppID: appID}

		switch req.Action {
		case "start":
			if err := h.store.UpdateAppStatus(r.Context(), appID, "running"); err != nil {
				results[i].Status = "error"
				results[i].Error = err.Error()
			} else {
				results[i].Status = "started"
			}
		case "stop":
			if err := h.store.UpdateAppStatus(r.Context(), appID, "stopped"); err != nil {
				results[i].Status = "error"
				results[i].Error = err.Error()
			} else {
				results[i].Status = "stopped"
			}
		case "restart":
			h.store.UpdateAppStatus(r.Context(), appID, "running")
			results[i].Status = "restarted"
		case "delete":
			if err := h.store.DeleteApp(r.Context(), appID); err != nil {
				results[i].Status = "error"
				results[i].Error = err.Error()
			} else {
				results[i].Status = "deleted"
			}
		default:
			results[i].Status = "error"
			results[i].Error = "unknown action: " + req.Action
		}
	}

	succeeded := 0
	for _, r := range results {
		if r.Status != "error" {
			succeeded++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"results":   results,
		"total":     len(results),
		"succeeded": succeeded,
		"failed":    len(results) - succeeded,
	})
}
