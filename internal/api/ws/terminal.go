package ws

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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

// blockedPatterns commands that should never run in a tenant container.
var blockedPatterns = []string{
	"rm -rf /", "rm -rf /*", ":(){ :|:& };:", "mkfs",
	"dd if=/dev/zero", "> /dev/sd", "chmod -R 777 /", "chown -R",
	"curl | sh", "wget | sh", "curl | bash", "wget | bash",
}

// isCommandSafe checks if a command contains dangerous patterns.
func isCommandSafe(cmd string) bool {
	cmdLower := strings.ToLower(cmd)
	for _, pattern := range blockedPatterns {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return false
		}
	}
	return true
}

// splitCommand splits a command string into tokens, respecting quotes.
// Replaces "sh -c" usage to prevent shell injection.
func splitCommand(cmd string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := rune(0)
	for i := 0; i < len(cmd); i++ {
		ch := rune(cmd[i])
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			} else {
				current.WriteRune(ch)
			}
		} else {
			switch ch {
			case '\'', '"':
				inQuote = ch
			case ' ', '\t', '\n', '\r':
				if current.Len() > 0 {
					tokens = append(tokens, current.String())
					current.Reset()
				}
			default:
				current.WriteRune(ch)
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	if len(tokens) == 0 {
		return []string{"/bin/true"}
	}
	return tokens
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
		_, _ = w.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
		flusher.Flush()
		return
	}
	defer logReader.Close()

	// Send session ID so the client can correlate POST commands
	sessionID := core.GenerateID()
	_, _ = w.Write([]byte("event: session\ndata: " + sessionID + "\n\n"))
	flusher.Flush()

	// Send connection confirmation with container info
	connData, _ := json.Marshal(map[string]string{
		"container_id": containers[0].ID[:12],
		"image":        containers[0].Image,
		"status":       containers[0].State,
	})
	_, _ = w.Write([]byte("event: connected\ndata: " + string(connData) + "\n\n"))
	flusher.Flush()

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = w.Write([]byte("data: " + line + "\n\n"))
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Command == "" {
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
			"container", containerID[:12],
			"command", req.Command,
		)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command contains blocked pattern"})
		return
	}

	t.logger.Info("terminal exec",
		"app_id", appID,
		"container", containerID[:12],
		"command", req.Command,
	)

	// Execute directly without sh -c to prevent shell injection
	cmd := splitCommand(req.Command)
	output, err := t.runtime.Exec(r.Context(), containerID, cmd)
	if err != nil {
		t.logger.Error("terminal exec failed",
			"app_id", appID,
			"container", containerID[:12],
			"command", req.Command,
			"error", err,
		)

		writeJSON(w, http.StatusOK, map[string]any{
			"output":       output,
			"exit_code":    1,
			"container_id": containerID[:12],
			"error":        err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"output":       output,
		"exit_code":    0,
		"container_id": containerID[:12],
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
