package handlers

import (
	"net/http"
	"runtime"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PlatformStatsHandler provides admin-level platform-wide statistics.
type PlatformStatsHandler struct {
	core *core.Core
}

func NewPlatformStatsHandler(c *core.Core) *PlatformStatsHandler {
	return &PlatformStatsHandler{core: c}
}

// Overview handles GET /api/v1/admin/stats
// Super admin overview of the entire platform.
func (h *PlatformStatsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil || claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	// Module health
	moduleHealth := h.core.Registry.HealthAll()
	healthy, degraded, down := 0, 0, 0
	for _, s := range moduleHealth {
		switch s {
		case core.HealthOK:
			healthy++
		case core.HealthDegraded:
			degraded++
		case core.HealthDown:
			down++
		}
	}

	eventStats := h.core.Events.Stats()

	// Container counts
	var containers int
	if h.core.Services.Container != nil {
		list, err := h.core.Services.Container.ListByLabels(r.Context(), map[string]string{"monster.enable": "true"})
		if err == nil {
			containers = len(list)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"platform": map[string]any{
			"version":    h.core.Build.Version,
			"uptime_go":  runtime.NumGoroutine(),
			"memory_mb":  mem.Alloc / 1024 / 1024,
			"cpu_cores":  runtime.NumCPU(),
		},
		"modules": map[string]int{
			"total":    healthy + degraded + down,
			"healthy":  healthy,
			"degraded": degraded,
			"down":     down,
		},
		"containers": containers,
		"events": map[string]any{
			"published":     eventStats.PublishCount,
			"errors":        eventStats.ErrorCount,
			"subscriptions": eventStats.SubscriptionCount,
		},
		"endpoints": 150,
	})
}
