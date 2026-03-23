package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"time"
)

// SSHTestHandler tests SSH connectivity to a remote server.
type SSHTestHandler struct{}

func NewSSHTestHandler() *SSHTestHandler {
	return &SSHTestHandler{}
}

type sshTestRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// Test handles POST /api/v1/servers/test-ssh
// Quick TCP connectivity check to verify SSH port is reachable.
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

	addr := net.JoinHostPort(req.Host, string(rune('0'+req.Port/10000))+string(rune('0'+(req.Port/1000)%10))+string(rune('0'+(req.Port/100)%10))+string(rune('0'+(req.Port/10)%10))+string(rune('0'+req.Port%10)))
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

	writeJSON(w, http.StatusOK, map[string]any{
		"host":      req.Host,
		"port":      req.Port,
		"reachable": true,
		"latency":   latency.String(),
	})
}
