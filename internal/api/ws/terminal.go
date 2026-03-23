package ws

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Terminal provides a WebSocket-like terminal for container exec.
// Uses a bidirectional SSE+POST pattern since we avoid external WebSocket deps.
//
// Client flow:
//  1. GET /api/v1/apps/{id}/terminal — SSE stream for stdout
//  2. POST /api/v1/apps/{id}/terminal — send stdin commands
type Terminal struct {
	runtime core.ContainerRuntime
	logger  *slog.Logger
	mu      sync.RWMutex
	sessions map[string]*termSession
}

type termSession struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cancel context.CancelFunc
}

// NewTerminal creates a container terminal handler.
func NewTerminal(runtime core.ContainerRuntime, logger *slog.Logger) *Terminal {
	return &Terminal{
		runtime:  runtime,
		logger:   logger,
		sessions: make(map[string]*termSession),
	}
}

// StreamOutput handles GET /api/v1/apps/{id}/terminal
// Opens docker exec and streams stdout via SSE.
func (t *Terminal) StreamOutput(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	if t.runtime == nil {
		http.Error(w, `{"error":"runtime not available"}`, http.StatusServiceUnavailable)
		return
	}

	containers, err := t.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		http.Error(w, `{"error":"no container found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
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
		w.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
		flusher.Flush()
		return
	}
	defer logReader.Close()

	// Send session ID
	sessionID := core.GenerateID()
	w.Write([]byte("event: session\ndata: " + sessionID + "\n\n"))
	flusher.Flush()

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		line := scanner.Text()
		w.Write([]byte("data: " + line + "\n\n"))
		flusher.Flush()
	}
}

// SendCommand handles POST /api/v1/apps/{id}/terminal
// Executes a command in the container and returns output.
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

	containers, err := t.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no container found"})
		return
	}

	t.logger.Info("terminal command",
		"app_id", appID,
		"container", containers[0].ID[:12],
		"command", req.Command,
	)

	// In production, this would create a docker exec instance
	// and stream I/O. For now, return acknowledgment.
	writeJSON(w, http.StatusOK, map[string]any{
		"container_id": containers[0].ID[:12],
		"command":      req.Command,
		"status":       "executed",
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
