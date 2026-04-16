package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PortHandler manages app port mappings.
type PortHandler struct {
	store core.Store
}

func NewPortHandler(store core.Store) *PortHandler {
	return &PortHandler{store: store}
}

// PortMapping represents a container port mapping.
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port,omitempty"` // 0 = auto
	Protocol      string `json:"protocol"`            // tcp, udp
	Exposed       bool   `json:"exposed"`
}

// Get handles GET /api/v1/apps/{id}/ports
func (h *PortHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	// Default port based on app type — in production would read from container inspect
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"ports": []PortMapping{
			{ContainerPort: 80, Protocol: "tcp", Exposed: true},
		},
	})
}

// Update handles PUT /api/v1/apps/{id}/ports
func (h *PortHandler) Update(w http.ResponseWriter, r *http.Request) {
	// SECURITY: Verify the app belongs to this tenant
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var ports []PortMapping
	if err := json.NewDecoder(r.Body).Decode(&ports); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body — expected array of port mappings")
		return
	}

	// Cap the array — 100 mappings per app is more than any real workload
	// needs, and without this an attacker could post a multi-MB array that
	// slips under the 10MB BodyLimit middleware.
	if len(ports) > 100 {
		writeError(w, http.StatusBadRequest, "too many port mappings (max 100)")
		return
	}

	for i := range ports {
		p := &ports[i]
		if p.ContainerPort <= 0 || p.ContainerPort > 65535 {
			writeError(w, http.StatusBadRequest, "invalid container port")
			return
		}
		// 0 = auto-assign; otherwise must be in the valid TCP/UDP range.
		if p.HostPort < 0 || p.HostPort > 65535 {
			writeError(w, http.StatusBadRequest, "invalid host port")
			return
		}
		if p.Protocol == "" {
			p.Protocol = "tcp"
		}
		if p.Protocol != "tcp" && p.Protocol != "udp" {
			writeError(w, http.StatusBadRequest, "protocol must be tcp or udp")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"ports":  ports,
		"status": "updated",
	})
}
