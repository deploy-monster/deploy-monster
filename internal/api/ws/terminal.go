package ws

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const maxTerminalCommandBodySize = 1 << 20

// Terminal provides a WebSocket-like terminal for container exec.
// Uses a bidirectional SSE+POST pattern since we avoid external WebSocket deps.
//
// Client flow:
//  1. GET /api/v1/apps/{id}/terminal — SSE stream for stdout/logs
//  2. POST /api/v1/apps/{id}/terminal — send stdin commands, get output back
type Terminal struct {
	runtime core.ContainerRuntime
	store   core.Store
	logger  *slog.Logger
}

// NewTerminal creates a container terminal handler.
func NewTerminal(runtime core.ContainerRuntime, store core.Store, logger *slog.Logger) *Terminal {
	return &Terminal{
		runtime: runtime,
		store:   store,
		logger:  logger,
	}
}

// isCommandSafe validates the command against the shared exec safety policy.
func isCommandSafe(cmd string) bool {
	return core.CommandSafe(cmd)
}

// splitCommand splits a command string into argv tokens without invoking a shell.
func splitCommand(cmd string) []string {
	return core.SplitCommand(cmd)
}

// StreamOutput handles GET /api/v1/apps/{id}/terminal
// Opens docker exec and streams stdout via SSE.
func (t *Terminal) StreamOutput(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	if t.runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not available"})
		return
	}

	containers, err := t.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no container found"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// Stream container logs as terminal output
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	logReader, err := t.runtime.Logs(ctx, containers[0].ID, "20", true)
	if err != nil {
		writeSSEEvent(w, "error", err.Error())
		flusher.Flush()
		return
	}
	defer func() { _ = logReader.Close() }()

	// Send session ID so the client can correlate POST commands
	sessionID := core.GenerateID()
	writeSSEEvent(w, "session", sessionID)
	flusher.Flush()

	// Send connection confirmation with container info
	connData, _ := json.Marshal(map[string]string{
		"container_id": shortResourceID(containers[0].ID),
		"image":        containers[0].Image,
		"status":       containers[0].State,
	})
	writeSSEEvent(w, "connected", string(connData))
	flusher.Flush()

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		writeSSEEvent(w, "", scanner.Text())
		flusher.Flush()
	}
}

// SendCommand handles POST /api/v1/apps/{id}/terminal
// Executes a command in the container via runtime.Exec and returns output.
//
// Request body:
//
//	{"command": "ls -la"}
//
// Response:
//
//	{"output": "...", "exit_code": 0, "container_id": "abc123def456"}
func (t *Terminal) SendCommand(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	if t.runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not available"})
		return
	}

	var req struct {
		Command string `json:"command"`
	}
	if !decodeTerminalCommand(r, &req) || req.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}

	// Verify the app exists
	if t.store != nil {
		if _, err := t.store.GetApp(r.Context(), appID); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "app not found"})
			return
		}
	}

	containers, err := t.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil {
		t.logger.Error("list containers for terminal", "app_id", appID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find container"})
		return
	}
	if len(containers) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no running container for this app"})
		return
	}

	containerID := containers[0].ID

	// Security: validate command against blocklist before execution
	if !isCommandSafe(req.Command) {
		t.logger.Warn("terminal blocked dangerous command",
			"app_id", appID,
			"container", shortResourceID(containerID),
			"command", req.Command,
		)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command contains blocked pattern"})
		return
	}

	t.logger.Info("terminal exec",
		"app_id", appID,
		"container", shortResourceID(containerID),
		"command", req.Command,
	)

	// Execute directly without sh -c to prevent shell injection
	cmd := splitCommand(req.Command)
	output, err := t.runtime.Exec(r.Context(), containerID, cmd)
	if err != nil {
		t.logger.Error("terminal exec failed",
			"app_id", appID,
			"container", shortResourceID(containerID),
			"command", req.Command,
			"error", err,
		)

		writeJSON(w, http.StatusOK, map[string]any{
			"output":       output,
			"exit_code":    1,
			"container_id": shortResourceID(containerID),
			"error":        err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"output":       output,
		"exit_code":    0,
		"container_id": shortResourceID(containerID),
	})
}

func decodeTerminalCommand(r *http.Request, target any) bool {
	body := io.LimitReader(r.Body, maxTerminalCommandBodySize+1)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return false
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		return false
	}
	return true
}

func shortResourceID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
