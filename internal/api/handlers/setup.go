package handlers

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SetupHandler answers a focused subset of "is this box ready" questions used
// by the onboarding wizard. It is intentionally distinct from /health/detailed
// (which is operator-facing) and returns short user-friendly strings instead
// of opaque status codes.
type SetupHandler struct {
	core *core.Core
}

// NewSetupHandler returns a SetupHandler bound to the platform core.
func NewSetupHandler(c *core.Core) *SetupHandler {
	return &SetupHandler{core: c}
}

type setupCheck struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Status string `json:"status"` // "ok" | "warn" | "fail"
}

// Checks handles GET /api/v1/setup/checks.
func (h *SetupHandler) Checks(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	dockerStatus := "fail"
	dockerValue := "Not available"
	if h.core != nil && h.core.Services != nil && h.core.Services.Container != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if rt, ok := h.core.Services.Container.(interface {
			PingContext(context.Context) error
		}); ok {
			if err := rt.PingContext(ctx); err == nil {
				dockerStatus = "ok"
				dockerValue = "Connected"
			} else {
				dockerValue = err.Error()
			}
		} else if h.core.Services.Container.Ping() == nil {
			dockerStatus = "ok"
			dockerValue = "Connected"
		}
	}

	sslStatus := "warn"
	sslValue := "Disabled (HTTP only)"
	if h.core != nil && h.core.Config != nil {
		if h.core.Config.Ingress.EnableHTTPS {
			if h.core.Config.ACME.Email != "" {
				sslStatus = "ok"
				sslValue = "Let's Encrypt (" + h.core.Config.ACME.Email + ")"
			} else {
				sslStatus = "warn"
				sslValue = "HTTPS on, ACME email missing"
			}
		}
	}

	apiPort := 8443
	httpPort := 80
	httpsPort := 443
	if h.core != nil && h.core.Config != nil {
		if h.core.Config.Server.Port > 0 {
			apiPort = h.core.Config.Server.Port
		}
		if h.core.Config.Ingress.HTTPPort > 0 {
			httpPort = h.core.Config.Ingress.HTTPPort
		}
		if h.core.Config.Ingress.HTTPSPort > 0 {
			httpsPort = h.core.Config.Ingress.HTTPSPort
		}
	}
	apiBound := portInUse(apiPort)
	httpBound := portInUse(httpPort)
	httpsBound := portInUse(httpsPort)

	portsValue := strconv.Itoa(httpPort) + ", " + strconv.Itoa(httpsPort) + ", " + strconv.Itoa(apiPort)
	portsStatus := "ok"
	if !apiBound {
		portsStatus = "fail"
		portsValue = "API port " + strconv.Itoa(apiPort) + " not listening"
	} else if !httpBound || !httpsBound {
		portsStatus = "warn"
		portsValue += " (ingress partially bound)"
	}

	checks := []setupCheck{
		{Label: "Hostname", Value: hostname, Status: "ok"},
		{Label: "Docker Engine", Value: dockerValue, Status: dockerStatus},
		{Label: "SSL", Value: sslValue, Status: sslStatus},
		{Label: "Ports", Value: portsValue, Status: portsStatus},
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": checks})
}

// portInUse returns true if any local process is bound to the given TCP port
// on any interface — meaning a Listen on the same port would currently fail.
func portInUse(port int) bool {
	addr := ":" + strconv.Itoa(port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return true
	}
	_ = l.Close()
	return false
}
