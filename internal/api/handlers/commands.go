package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CommandHandler runs one-off tasks inside app containers.
// Useful for migrations, seed scripts, cache clearing, etc.
type CommandHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
}

func NewCommandHandler(runtime core.ContainerRuntime, store core.Store, events *core.EventBus) *CommandHandler {
	return &CommandHandler{runtime: runtime, store: store, events: events}
}

type runCommandRequest struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds, default 60
}

// Run handles POST /api/v1/apps/{id}/commands
func (h *CommandHandler) Run(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var req runCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = 60
	}

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no running container for app")
		return
	}

	// Log command execution for audit
	h.events.PublishAsync(r.Context(), core.NewEvent("app.command", "api",
		map[string]string{"app_id": appID, "command": req.Command}))

	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":       appID,
		"container_id": containers[0].ID[:12],
		"command":      req.Command,
		"timeout":      req.Timeout,
		"status":       "queued",
		"queued_at":    time.Now(),
	})
}

// History handles GET /api/v1/apps/{id}/commands
func (h *CommandHandler) History(w http.ResponseWriter, r *http.Request) {
	if app := requireTenantApp(w, r, h.store); app == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}
