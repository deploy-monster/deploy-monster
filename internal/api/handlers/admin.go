package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AdminHandler serves system administration endpoints.
type AdminHandler struct {
	core    *core.Core
	store   core.Store
	authMod *auth.Module
}

// NewAdminHandler creates an AdminHandler. authMod may be nil in tests
// that don't exercise key revocation — RevokeAllKeys checks before use.
func NewAdminHandler(c *core.Core, store core.Store, authMod ...*auth.Module) *AdminHandler {
	var mod *auth.Module
	if len(authMod) > 0 {
		mod = authMod[0]
	}
	return &AdminHandler{core: c, store: store, authMod: mod}
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
		"version":    h.core.Build.Version,
		"commit":     h.core.Build.Commit,
		"built":      h.core.Build.Date,
		"go":         runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
		"memory": map[string]any{
			"alloc_mb": mem.Alloc / 1024 / 1024,
			"sys_mb":   mem.Sys / 1024 / 1024,
			"gc_runs":  mem.NumGC,
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
	pg := parsePagination(r)

	tenants, total, err := h.store.ListAllTenants(r.Context(), pg.PerPage, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	if tenants == nil {
		tenants = []core.Tenant{}
	}

	writePaginatedJSON(w, tenants, total, pg)
}

// RevokeAllKeys handles POST /api/v1/admin/keys/revoke-all
// Super admin only — immediately revokes all previous JWT rotation keys.
// This is an emergency endpoint: all tokens signed with old keys are
// rejected instantly (not just after RotationGracePeriod).
// Use when a key compromise is suspected.
func (h *AdminHandler) RevokeAllKeys(w http.ResponseWriter, r *http.Request) {
	if h.authMod == nil || h.authMod.JWT() == nil {
		writeError(w, http.StatusServiceUnavailable, "auth service unavailable")
		return
	}
	n := h.authMod.JWT().RevokeAllPreviousKeys()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"revoked_keys": n,
	})
}
