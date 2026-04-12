package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/handlers"
)

// registerAdminRoutes wires every /api/v1/admin/* endpoint.
// adminOnly must already stack RequireSuperAdmin on top of protected(auth+tenantRL).
func registerAdminRoutes(r *Router, adminOnly func(http.Handler) http.Handler) {
	// ── Announcements ─────────────────────────────────
	announcH := handlers.NewAnnouncementHandler(r.core.DB.Bolt)
	r.mux.HandleFunc("GET /api/v1/announcements", announcH.List) // public
	r.mux.Handle("POST /api/v1/admin/announcements", adminOnly(http.HandlerFunc(announcH.Create)))
	r.mux.Handle("DELETE /api/v1/admin/announcements/{id}", adminOnly(http.HandlerFunc(announcH.Dismiss)))

	// ── Admin Disk Usage ──────────────────────────────
	diskH := handlers.NewDiskUsageHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/admin/disk", adminOnly(http.HandlerFunc(diskH.SystemDisk)))

	// ── Tenant Rate Limits (super admin) ──────────────
	trlH := handlers.NewTenantRateLimitHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/admin/tenants/{id}/ratelimit", adminOnly(http.HandlerFunc(trlH.Get)))
	r.mux.Handle("PUT /api/v1/admin/tenants/{id}/ratelimit", adminOnly(http.HandlerFunc(trlH.Update)))

	// ── Platform Stats (super admin) ──────────────────
	platH := handlers.NewPlatformStatsHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/stats", adminOnly(http.HandlerFunc(platH.Overview)))

	// ── License ──────────────────────────────────────
	licH := handlers.NewLicenseHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/admin/license", adminOnly(http.HandlerFunc(licH.Get)))
	r.mux.Handle("POST /api/v1/admin/license", adminOnly(http.HandlerFunc(licH.Activate)))

	// ── DB Backup (admin) ─────────────────────────────
	dbBackupH := handlers.NewDBBackupHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/db/backup", adminOnly(http.HandlerFunc(dbBackupH.Backup)))
	r.mux.Handle("GET /api/v1/admin/db/status", adminOnly(http.HandlerFunc(dbBackupH.Status)))

	// ── Admin API Keys ────────────────────────────────
	adminKeyH := handlers.NewAdminAPIKeyHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/admin/api-keys", adminOnly(http.HandlerFunc(adminKeyH.List)))
	r.mux.Handle("POST /api/v1/admin/api-keys", adminOnly(http.HandlerFunc(adminKeyH.Generate)))
	r.mux.Handle("DELETE /api/v1/admin/api-keys/{prefix}", adminOnly(http.HandlerFunc(adminKeyH.Revoke)))

	// Background cleanup of expired API keys (every hour)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic in API key cleanup", "error", rec)
			}
		}()
		ticker := time.NewTicker(apiKeyCleanupEvery)
		defer ticker.Stop()
		for {
			select {
			case <-r.serverCtx.Done():
				return
			case <-ticker.C:
				if n := adminKeyH.CleanupExpiredKeys(); n > 0 {
					slog.Info("cleaned up expired API keys", "count", n)
				}
			}
		}
	}()

	// ── DB Migrations ─────────────────────────────────
	migH := handlers.NewMigrationHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/db/migrations", adminOnly(http.HandlerFunc(migH.Status)))

	// ── Admin (super admin only) ──────────────────────
	adminH := handlers.NewAdminHandler(r.core, r.store)
	r.mux.Handle("GET /api/v1/admin/system", adminOnly(http.HandlerFunc(adminH.SystemInfo)))
	r.mux.Handle("PATCH /api/v1/admin/settings", adminOnly(http.HandlerFunc(adminH.UpdateSettings)))
	r.mux.Handle("GET /api/v1/admin/tenants", adminOnly(http.HandlerFunc(adminH.ListTenants)))

	// ── Self-Update ──────────────────────────────────
	updateH := handlers.NewSelfUpdateHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/updates", adminOnly(http.HandlerFunc(updateH.CheckUpdate)))

	// ── Branding (public GET, admin PATCH) ────────────
	brandingH := handlers.NewBrandingHandler()
	r.mux.HandleFunc("GET /api/v1/branding", brandingH.Get)
	r.mux.Handle("PATCH /api/v1/admin/branding", adminOnly(http.HandlerFunc(brandingH.Update)))
}
