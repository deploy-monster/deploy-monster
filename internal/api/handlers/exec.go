package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Command validation - dangerous patterns that should be blocked.
var blockedPatterns = []string{
	"rm -rf /",
	"rm -rf /*",
	":(){ :|:& };:",   // Fork bomb
	"mkfs",            // Format filesystem
	"dd if=/dev/zero", // Disk wipe
	"> /dev/sd",       // Direct disk write
	"chmod -R 777 /",  // Dangerous permission change
	"chown -R",        // Mass ownership change (often abused)
	"curl | sh",       // Remote code execution pattern
	"wget | sh",       // Remote code execution pattern
	"curl | bash",     // Remote code execution pattern
	"wget | bash",     // Remote code execution pattern
}

// isCommandSafe checks if a command is safe to execute.
func isCommandSafe(cmd string) bool {
	cmdLower := strings.ToLower(cmd)
	for _, pattern := range blockedPatterns {
		if strings.Contains(cmdLower, strings.ToLower(pattern)) {
			return false
		}
	}
	return true
}

// splitCommand splits a command string into tokens, respecting single/double
// quotes. This replaces "sh -c" injection by passing each token as a direct
// exec argument. Shell operators (&&, ||, |, $, subshells) are treated as
// data by the exec binary rather than interpreted.
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
				inQuote = rune(ch)
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

// ExecHandler handles container exec endpoints.
type ExecHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
	logger  *slog.Logger
	bolt    core.BoltStorer
}

// NewExecHandler creates a new exec handler.
func NewExecHandler(runtime core.ContainerRuntime, store core.Store, logger *slog.Logger, bolt core.BoltStorer) *ExecHandler {
	return &ExecHandler{
		runtime: runtime,
		store:   store,
		logger:  logger,
		bolt:    bolt,
	}
}

// auditExec logs the container exec command to the audit log.
func (h *ExecHandler) auditExec(ctx context.Context, appID, containerID, command string, exitCode int, execErr error) {
	claims := auth.ClaimsFromContext(ctx)
	userID := "unknown"
	tenantID := "unknown"
	if claims != nil {
		userID = claims.UserID
		tenantID = claims.TenantID
	}

	action := "container.exec.success"
	if execErr != nil {
		action = "container.exec.failed"
	}

	// Marshal details to JSON string
	details := map[string]any{
		"container_id": containerID,
		"command":      command,
		"exit_code":    exitCode,
	}
	detailsJSON, _ := json.Marshal(details)

	auditEntry := &core.AuditEntry{
		ID:           time.Now().UnixNano(),
		TenantID:     tenantID,
		UserID:       userID,
		Action:       action,
		ResourceType: "app",
		ResourceID:   appID,
		DetailsJSON:  string(detailsJSON),
		CreatedAt:    time.Now(),
	}

	if h.store != nil {
		if auditErr := h.store.CreateAuditLog(ctx, auditEntry); auditErr != nil {
			h.logger.Error("failed to write audit log", "error", auditErr)
		}
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
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}

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

	// Security: Validate command against blocked patterns
	if !isCommandSafe(req.Command) {
		h.logger.Warn("blocked dangerous exec command",
			"app_id", appID,
			"command", req.Command,
		)
		// Audit the blocked attempt
		h.auditExec(r.Context(), appID, "", req.Command, 0, fmt.Errorf("command blocked by security policy"))
		writeError(w, http.StatusBadRequest, "command contains blocked pattern for security reasons")
		return
	}

	// Verify the app exists and belongs to this tenant
	if h.store != nil {
		if app := requireTenantApp(w, r, h.store); app == nil {
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
	// Otherwise, split the command string into tokens and pass directly to
	// exec — this avoids shell interpretation (no &&, ||, $, subshells).
	var cmd []string
	if len(req.Args) > 0 {
		cmd = append([]string{req.Command}, req.Args...)
	} else {
		cmd = splitCommand(req.Command)
	}

	// Execute the command inside the container
	output, err := h.runtime.Exec(r.Context(), containerID, cmd)
	exitCode := 0
	if err != nil {
		h.logger.Error("container exec failed",
			"app_id", appID,
			"container_id", containerID,
			"command", req.Command,
			"error", err,
		)

		// If the error contains "exec create" or "exec attach", it's an infrastructure error
		errMsg := err.Error()
		if strings.Contains(errMsg, "exec create") || strings.Contains(errMsg, "exec attach") {
			h.auditExec(r.Context(), appID, containerID, req.Command, exitCode, err)
			writeError(w, http.StatusInternalServerError, "failed to exec in container")
			return
		}

		// Try to parse exit code from error message (e.g., "exit code 1")
		if strings.Contains(errMsg, "exit code") {
			parts := strings.Split(errMsg, "exit code")
			if len(parts) > 1 {
				codeStr := strings.TrimSpace(parts[len(parts)-1])
				if parsed, parseErr := strconv.Atoi(codeStr); parseErr == nil {
					exitCode = parsed
				}
			}
		}

		// Command ran but returned non-zero — still return the output we got
		// Don't expose internal error details, just return the output
		h.auditExec(r.Context(), appID, containerID, req.Command, exitCode, nil)
		writeJSON(w, http.StatusOK, execResponse{
			Output:      output,
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
	h.auditExec(r.Context(), appID, containerID, req.Command, 0, nil)
	writeJSON(w, http.StatusOK, execResponse{
		Output:      output,
		ExitCode:    0,
		ContainerID: containerID,
	})
}
