package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"strconv"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AdminHandler serves system administration endpoints.
type AdminHandler struct {
	core  *core.Core
	store core.Store
}

func NewAdminHandler(c *core.Core, store core.Store) *AdminHandler {
	return &AdminHandler{core: c, store: store}
}

// SystemInfo handles GET /api/v1/admin/system
func (h *AdminHandler) SystemInfo(w http.ResponseWriter, _ *http.Request) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	modules := make([]map[string]any, 0)
	for id, status := range h.core.Registry.HealthAll() {
		modules = append(modules, map[string]any{
			"id":     id,
			"status": status.String(),
		})
	}

	stats := h.core.Events.Stats()

	writeJSON(w, http.StatusOK, map[string]any{
		"version": h.core.Build.Version,
		"commit":  h.core.Build.Commit,
		"built":   h.core.Build.Date,
		"go":      runtime.Version(),
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"cpus":    runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
		"memory": map[string]any{
			"alloc_mb":   mem.Alloc / 1024 / 1024,
			"sys_mb":     mem.Sys / 1024 / 1024,
			"gc_runs":    mem.NumGC,
		},
		"modules": modules,
		"events": map[string]any{
			"published":     stats.PublishCount,
			"errors":        stats.ErrorCount,
			"subscriptions": stats.SubscriptionCount,
		},
	})
}

// UpdateSettings handles PATCH /api/v1/admin/settings
func (h *AdminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]any
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Settings would be persisted to config/DB
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "updated",
		"settings": settings,
	})
}

// ListTenants handles GET /api/v1/admin/tenants
// Super admin only — lists all tenants on the platform.
func (h *AdminHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	tenants, total, err := h.store.ListAllTenants(r.Context(), perPage, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	if tenants == nil {
		tenants = []core.Tenant{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":  tenants,
		"total": total,
	})
}
