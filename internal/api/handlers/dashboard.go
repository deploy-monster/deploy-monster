package handlers

import (
	"log/slog"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DashboardHandler serves aggregated platform statistics.
type DashboardHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewDashboardHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *DashboardHandler {
	return &DashboardHandler{store: store, runtime: runtime, events: events}
}

// Stats handles GET /api/v1/dashboard/stats
func (h *DashboardHandler) Stats(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// App counts
	tenantApps, totalApps, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 10000, 0)
	if err != nil {
		slog.Warn("dashboard: failed to list apps", "error", err)
	}

	// Domain count, scoped through the current tenant's applications.
	domainCount := 0
	for _, app := range tenantApps {
		domains, err := h.store.ListDomainsByApp(r.Context(), app.ID)
		if err != nil {
			slog.Warn("dashboard: failed to list domains", "app_id", app.ID, "error", err)
			continue
		}
		domainCount += len(domains)
	}

	// Project count
	projects, err := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if err != nil {
		slog.Warn("dashboard: failed to list projects", "error", err)
	}

	// Container counts
	var running, stopped int
	if h.runtime != nil {
		containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
			"monster.enable": "true",
		})
		if err == nil {
			for _, c := range containers {
				if c.State == "running" {
					running++
				} else {
					stopped++
				}
			}
		}
	}

	// Event stats
	eventStats := h.events.Stats()

	writeJSON(w, http.StatusOK, map[string]any{
		"apps": map[string]int{
			"total": totalApps,
		},
		"containers": map[string]int{
			"running": running,
			"stopped": stopped,
			"total":   running + stopped,
		},
		"domains":  domainCount,
		"projects": len(projects),
		"events": map[string]any{
			"published": eventStats.PublishCount,
			"errors":    eventStats.ErrorCount,
		},
	})
}
