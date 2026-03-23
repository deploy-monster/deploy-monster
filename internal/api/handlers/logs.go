package handlers

import (
	"net/http"
	"strconv"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LogHandler serves application logs.
type LogHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
}

func NewLogHandler(runtime core.ContainerRuntime, store core.Store) *LogHandler {
	return &LogHandler{runtime: runtime, store: store}
}

// GetLogs handles GET /api/v1/apps/{id}/logs
// Returns the last N lines of container logs.
func (h *LogHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	if _, err := strconv.Atoi(tail); err != nil {
		tail = "100"
	}

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no running container found")
		return
	}

	reader, err := h.runtime.Logs(r.Context(), containers[0].ID, tail, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read logs")
		return
	}
	defer reader.Close()

	buf := make([]byte, 256*1024) // 256KB max
	n, _ := reader.Read(buf)

	// Parse lines
	lines := splitLines(string(buf[:n]))

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"container_id": containers[0].ID[:12],
		"lines":        lines,
		"count":        len(lines),
	})
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
