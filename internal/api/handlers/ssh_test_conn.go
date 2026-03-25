package handlers

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SSHTestHandler tests SSH connectivity to a remote server.
type SSHTestHandler struct {
	services *core.Services
}

func NewSSHTestHandler(services *core.Services) *SSHTestHandler {
	return &SSHTestHandler{services: services}
}

type sshTestRequest struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	ServerID string `json:"server_id,omitempty"`
}

// Test handles POST /api/v1/servers/test-ssh
// Quick TCP connectivity check to verify SSH port is reachable,
// and optionally tests SSH execution if a server_id is provided.
func (h *SSHTestHandler) Test(w http.ResponseWriter, r *http.Request) {
	var req sshTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "host required")
		return
	}
	if req.Port <= 0 {
		req.Port = 22
	}

	addr := net.JoinHostPort(req.Host, fmt.Sprintf("%d", req.Port))
	start := time.Now()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	latency := time.Since(start)

	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"host":      req.Host,
			"port":      req.Port,
			"reachable": false,
			"error":     err.Error(),
			"latency":   latency.String(),
		})
		return
	}
	conn.Close()

	result := map[string]any{
		"host":      req.Host,
		"port":      req.Port,
		"reachable": true,
		"latency":   latency.String(),
	}

	// If server_id provided and SSH client available, test actual SSH execution
	if req.ServerID != "" && h.services.SSH != nil {
		output, err := h.services.SSH.Execute(r.Context(), req.ServerID, "echo ok")
		if err != nil {
			result["ssh_auth"] = false
			result["ssh_error"] = err.Error()
		} else {
			result["ssh_auth"] = true
			result["ssh_output"] = output
		}
	}

	writeJSON(w, http.StatusOK, result)
}
