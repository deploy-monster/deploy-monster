package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ExecHandler handles container exec endpoints.
type ExecHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
	logger  *slog.Logger
}

// NewExecHandler creates a new exec handler.
func NewExecHandler(runtime core.ContainerRuntime, store core.Store, logger *slog.Logger) *ExecHandler {
	return &ExecHandler{
		runtime: runtime,
		store:   store,
		logger:  logger,
	}
}

type execRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

type execResponse struct {
	Output      string `json:"output"`
	ExitCode    int    `json:"exit_code"`
	ContainerID string `json:"container_id"`
}

// Exec handles POST /api/v1/apps/{id}/exec
// Runs a command inside the application's container and returns output.
//
// Request body:
//
//	{"command": "ls -la"}                    — shell-style command string
//	{"command": "ls", "args": ["-la"]}       — explicit command + args
//
// Response:
//
//	{"output": "...", "exit_code": 0, "container_id": "abc123"}
func (h *ExecHandler) Exec(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	// Verify the app exists
	if h.store != nil {
		if _, err := h.store.GetApp(r.Context(), appID); err != nil {
			writeError(w, http.StatusNotFound, "app not found")
			return
		}
	}

	// Find running container for this app
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil {
		h.logger.Error("list containers by label", "app_id", appID, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to find container")
		return
	}
	if len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no running container for this app")
		return
	}

	containerID := containers[0].ID

	// Build the command slice. If args are provided explicitly, use them.
	// Otherwise, wrap the command string with sh -c for shell interpretation.
	var cmd []string
	if len(req.Args) > 0 {
		cmd = append([]string{req.Command}, req.Args...)
	} else {
		cmd = []string{"sh", "-c", req.Command}
	}

	// Execute the command inside the container
	output, err := h.runtime.Exec(r.Context(), containerID, cmd)
	if err != nil {
		h.logger.Error("container exec failed",
			"app_id", appID,
			"container_id", containerID,
			"command", req.Command,
			"error", err,
		)

		// If the error contains "exit code", parse it; otherwise report as failure
		exitCode := 1
		errMsg := err.Error()
		if strings.Contains(errMsg, "exec create") || strings.Contains(errMsg, "exec attach") {
			writeError(w, http.StatusInternalServerError, "failed to exec in container: "+errMsg)
			return
		}

		// Command ran but returned non-zero — still return the output we got
		writeJSON(w, http.StatusOK, execResponse{
			Output:      output + "\n" + errMsg,
			ExitCode:    exitCode,
			ContainerID: containerID,
		})
		return
	}

	h.logger.Info("container exec",
		"app_id", appID,
		"container_id", containerID,
		"command", req.Command,
	)

	writeJSON(w, http.StatusOK, execResponse{
		Output:      output,
		ExitCode:    0,
		ContainerID: containerID,
	})
}
