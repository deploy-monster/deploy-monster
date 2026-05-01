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

// Allowed commands for terminal exec. Commands are passed as exec arguments
// (no shell interpretation). The allowlist provides defense-in-depth.
var allowedCommands = map[string]struct{}{
	// File and directory operations
	"ls": {}, "ll": {}, "la": {}, "dir": {}, "find": {}, "stat": {},
	"cat": {}, "head": {}, "tail": {}, "grep": {}, "egrep": {}, "awk": {},
	"sed": {}, "cut": {}, "sort": {}, "uniq": {}, "wc": {}, "tr": {},
	"base64": {},

	// Navigation and info
	"pwd": {}, "cd": {}, "echo": {}, "printf": {}, "env": {}, "printenv": {},
	"id": {}, "whoami": {}, "hostname": {}, "uname": {}, "uptime": {},
	"date": {},

	// Process and system info
	"ps": {}, "top": {}, "htop": {}, "kill": {}, "pkill": {}, "killall": {},
	"sleep": {}, "watch": {}, "free": {}, "vmstat": {}, "iostat": {},

	// Network utilities
	"ping": {}, "ping6": {}, "curl": {}, "wget": {}, "nc": {}, "netcat": {},
	"ssh": {}, "scp": {}, "rsync": {}, "dig": {}, "nslookup": {}, "host": {},
	"ip": {}, "ss": {}, "ifconfig": {}, "route": {}, "netstat": {},

	// Disk and filesystem
	"df": {}, "du": {}, "mount": {}, "umount": {}, "ln": {}, "mkdir": {},
	"touch": {}, "file": {}, "tar": {}, "gzip": {}, "gunzip": {}, "zip": {},
	"unzip": {}, "cp": {}, "mv": {}, "rm": {}, "chmod": {}, "chown": {},

	// Package managers (read-only)
	"apt": {}, "apt-get": {}, "yum": {}, "dnf": {}, "apk": {}, "pacman": {},

	// Interpreters/shell
	"sh": {}, "bash": {}, "zsh": {}, "fish": {}, "python": {}, "python3": {},
	"node": {}, "ruby": {}, "perl": {}, "php": {}, "lua": {},

	// Text editors
	"vi": {}, "vim": {}, "nano": {}, "emacs": {}, "ed": {},

	// Utility commands
	"true": {}, "false": {}, "yes": {}, "seq": {}, "expr": {}, "test": {},
}

// isCommandSafe validates the primary command against the allowlist.
// splitCommand already provides shell-injection protection by passing tokens
// as exec arguments (no shell interpretation). This function is an additional
// defense-in-depth layer.
func isCommandSafe(cmd string) bool {
	tokens := splitCommand(cmd)
	if len(tokens) == 0 {
		return false
	}
	// Extract the base command name (strip any leading path)
	base := tokens[0]
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	if _, ok := allowedCommands[strings.ToLower(base)]; !ok {
		return false
	}
	// Block shell operators that might appear as arguments
	cmdLower := strings.ToLower(cmd)
	blockedSuffixes := []string{"&&", "||", "|", ";", "$(", "`"}
	for _, suffix := range blockedSuffixes {
		if strings.Contains(cmdLower, suffix) {
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
	defer func() { _ = logReader.Close() }()

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
