package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CommandHandler runs one-off tasks inside app containers.
// Useful for migrations, seed scripts, cache clearing, etc.
type CommandHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
	bolt    core.BoltStorer
}

func NewCommandHandler(runtime core.ContainerRuntime, store core.Store, events *core.EventBus) *CommandHandler {
	return &CommandHandler{runtime: runtime, store: store, events: events}
}

// SetBolt wires the KV store used to persist command history.
// Called by the router after the handler is constructed so the bolt
// dependency is opt-in (some unit tests construct the handler without
// a backing store).
func (h *CommandHandler) SetBolt(b core.BoltStorer) { h.bolt = b }

type runCommandRequest struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"` // seconds, default 60
}

// commandHistoryEntry is what we persist per app per execution.
type commandHistoryEntry struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	UserID      string    `json:"user_id"`
	Command     string    `json:"command"`
	ContainerID string    `json:"container_id"`
	ExitOutput  string    `json:"output"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
}

const commandHistoryBucket = "app_commands"
const commandOutputCap = 64 * 1024 // truncate output beyond 64 KB so a runaway command can't fill bolt

// Run handles POST /api/v1/apps/{id}/commands.
// Runs `command` inside the app's container synchronously via the
// runtime's Exec. The previous implementation only logged an event and
// returned "queued"; nothing actually ran. We persist a small history
// entry so /apps/{id}/commands can surface real past executions.
func (h *CommandHandler) Run(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	claims := auth.ClaimsFromContext(r.Context())

	var req runCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cmdStr := strings.TrimSpace(req.Command)
	if cmdStr == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if len(cmdStr) > 8*1024 {
		writeError(w, http.StatusBadRequest, "command too long (max 8 KB)")
		return
	}
	cmd := splitCommand(cmdStr)
	if !areCommandTokensSafe(cmd) {
		writeError(w, http.StatusBadRequest, "command contains blocked pattern for security reasons")
		return
	}
	if req.Timeout <= 0 {
		req.Timeout = 60
	}
	if req.Timeout > 600 {
		req.Timeout = 600 // cap so a runaway can't pin the API forever
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

	containerID := containers[0].ID

	execCtx, cancel := context.WithTimeout(r.Context(), time.Duration(req.Timeout)*time.Second)
	defer cancel()

	startedAt := time.Now()
	output, execErr := h.runtime.Exec(execCtx, containerID, cmd)
	completedAt := time.Now()
	if len(output) > commandOutputCap {
		output = output[:commandOutputCap] + "\n... [output truncated]"
	}

	entry := commandHistoryEntry{
		ID:          core.GenerateID(),
		AppID:       appID,
		Command:     cmdStr,
		ContainerID: shortResourceID(containerID),
		ExitOutput:  output,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		Success:     execErr == nil,
	}
	if claims != nil {
		entry.UserID = claims.UserID
	}
	if execErr != nil {
		entry.Error = execErr.Error()
	}
	if h.bolt != nil {
		_ = h.bolt.Set(commandHistoryBucket, appID+":"+entry.ID, entry, 30*24*3600)
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("app.command", "api",
		map[string]string{
			"app_id":  appID,
			"command": cmdStr,
			"success": map[bool]string{true: "true", false: "false"}[entry.Success],
		}))

	status := http.StatusOK
	if !entry.Success {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]any{
		"app_id":       appID,
		"container_id": entry.ContainerID,
		"command":      cmdStr,
		"timeout":      req.Timeout,
		"output":       output,
		"success":      entry.Success,
		"error":        entry.Error,
		"started_at":   startedAt,
		"completed_at": completedAt,
		"duration_ms":  completedAt.Sub(startedAt).Milliseconds(),
	})
}

// History handles GET /api/v1/apps/{id}/commands.
func (h *CommandHandler) History(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	if h.bolt == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	keys, err := h.bolt.List(commandHistoryBucket)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	prefix := app.ID + ":"
	entries := make([]commandHistoryEntry, 0)
	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		var e commandHistoryEntry
		if h.bolt.Get(commandHistoryBucket, k, &e) == nil {
			entries = append(entries, e)
		}
	}
	// Newest first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "total": len(entries)})
}
