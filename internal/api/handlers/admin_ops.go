package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── migrations.go ────────────────────
// MigrationHandler shows database migration status.
type MigrationHandler struct {
	core *core.Core
}

func NewMigrationHandler(c *core.Core) *MigrationHandler {
	return &MigrationHandler{core: c}
}

// Status handles GET /api/v1/admin/db/migrations. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *MigrationHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.core.Store == nil {
		writeError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	migrations, err := h.core.Store.ListMigrations(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"migrations": migrations,
		"total":      len(migrations),
		"driver":     h.core.Config.Database.Driver,
	})
}

// ──────────────────── license.go ────────────────────
// LicenseHandler manages platform license validation.
type LicenseHandler struct {
	kv core.KVStorer
}

func NewLicenseHandler(kv core.KVStorer) *LicenseHandler {
	return &LicenseHandler{kv: kv}
}

// LicenseInfo represents the current license state.
type LicenseInfo struct {
	Type       string    `json:"type"` // community, pro, enterprise
	Key        string    `json:"key"`  // masked
	ValidUntil time.Time `json:"valid_until"`
	MaxNodes   int       `json:"max_nodes"`
	Features   []string  `json:"features"`
	Status     string    `json:"status"` // active, expired, invalid
}

// Get handles GET /api/v1/admin/license
func (h *LicenseHandler) Get(w http.ResponseWriter, _ *http.Request) {
	var info LicenseInfo
	if err := h.kv.Get("license", "current", &info); err != nil {
		// No license stored — return community defaults
		writeJSON(w, http.StatusOK, LicenseInfo{
			Type:     "community",
			Key:      "",
			MaxNodes: 1,
			Features: []string{"core", "marketplace", "monitoring"},
			Status:   "active",
		})
		return
	}
	// Check expiration
	if !info.ValidUntil.IsZero() && time.Now().After(info.ValidUntil) {
		info.Status = "expired"
	}
	writeJSON(w, http.StatusOK, info)
}

// Activate handles POST /api/v1/admin/license
func (h *LicenseHandler) Activate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "license key required")
		return
	}
	if len(req.Key) < 8 {
		writeError(w, http.StatusBadRequest, "license key too short")
		return
	}
	// Validate key format (simplified — production would verify signature)
	hash := sha256.Sum256([]byte(req.Key))
	fingerprint := hex.EncodeToString(hash[:8])
	masked := req.Key[:4] + "****" + req.Key[len(req.Key)-4:]
	info := LicenseInfo{
		Type:       "enterprise",
		Key:        masked,
		ValidUntil: time.Now().Add(365 * 24 * time.Hour),
		MaxNodes:   100,
		Features:   []string{"core", "marketplace", "monitoring", "whitelabel", "reseller", "whmcs", "ha", "priority_support"},
		Status:     "active",
	}
	if err := h.kv.Set("license", "current", info, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save license")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":         masked,
		"fingerprint": fingerprint,
		"status":      "activated",
		"type":        info.Type,
		"features":    info.Features,
		"valid_until": info.ValidUntil,
	})
}

// ──────────────────── maintenance.go ────────────────────
// MaintenanceHandler manages app maintenance mode.
// When enabled, the ingress returns a 503 maintenance page instead of proxying.
type MaintenanceHandler struct {
	store  core.Store
	events *core.EventBus
	kv     core.KVStorer
}

func NewMaintenanceHandler(store core.Store, events *core.EventBus, kv core.KVStorer) *MaintenanceHandler {
	return &MaintenanceHandler{store: store, events: events, kv: kv}
}

// MaintenanceConfig holds maintenance mode settings.
type MaintenanceConfig struct {
	Enabled    bool     `json:"enabled"`
	Message    string   `json:"message,omitempty"`     // Custom message on maintenance page
	AllowedIPs []string `json:"allowed_ips,omitempty"` // IPs that bypass maintenance
}

// Get handles GET /api/v1/apps/{id}/maintenance
func (h *MaintenanceHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg MaintenanceConfig
	if err := h.kv.Get("maintenance", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, MaintenanceConfig{Enabled: false})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/maintenance
func (h *MaintenanceHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg MaintenanceConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if err := h.kv.Set("maintenance", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save maintenance config")
		return
	}
	action := "disabled"
	if cfg.Enabled {
		action = "enabled"
	}
	publishEventAsync(r.Context(), h.events, core.NewEvent("app.maintenance", "api",
		map[string]string{"app_id": appID, "action": action}))
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": action,
	})
}

// ──────────────────── platform_stats.go ────────────────────
// PlatformStatsHandler provides admin-level platform-wide statistics.
type PlatformStatsHandler struct {
	core *core.Core
}

func NewPlatformStatsHandler(c *core.Core) *PlatformStatsHandler {
	return &PlatformStatsHandler{core: c}
}

// Overview handles GET /api/v1/admin/stats.
// Super-admin overview of the entire platform. Authorization is enforced
// by middleware.RequireSuperAdmin at the router.
func (h *PlatformStatsHandler) Overview(w http.ResponseWriter, r *http.Request) {
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
	eventStats := eventBusStats(h.core.Events)
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
			"version":   h.core.Build.Version,
			"uptime_go": runtime.NumGoroutine(),
			"memory_mb": mem.Alloc / 1024 / 1024,
			"cpu_cores": runtime.NumCPU(),
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

// ──────────────────── selfupdate.go ────────────────────
// SelfUpdateHandler checks for platform updates.
type SelfUpdateHandler struct {
	core *core.Core
}

func NewSelfUpdateHandler(c *core.Core) *SelfUpdateHandler {
	return &SelfUpdateHandler{core: c}
}

// CheckUpdate handles GET /api/v1/admin/updates
func (h *SelfUpdateHandler) CheckUpdate(w http.ResponseWriter, _ *http.Request) {
	currentVersion := h.core.Build.Version
	latest, releaseURL, err := checkLatestRelease()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"current_version": currentVersion,
			"commit":          h.core.Build.Commit,
			"build_date":      h.core.Build.Date,
			"update_check":    "failed",
			"error":           err.Error(),
		})
		return
	}
	hasUpdate := latest != currentVersion && latest != ""
	writeJSON(w, http.StatusOK, map[string]any{
		"current_version":  currentVersion,
		"latest_version":   latest,
		"update_available": hasUpdate,
		"release_url":      releaseURL,
		"commit":           h.core.Build.Commit,
		"build_date":       h.core.Build.Date,
	})
}

// updateClient is a dedicated HTTP client for update checks with transport-level timeout.
var updateClient = &http.Client{Timeout: 15 * time.Second}

func checkLatestRelease() (version, url string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/deploy-monster/deploy-monster/releases/latest", nil)
	if err != nil {
		return "", "", err
	}
	resp, err := updateClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return "", "", err
	}
	return release.TagName, release.HTMLURL, nil
}

// ──────────────────── tenant_ratelimit.go ────────────────────
// TenantRateLimitHandler manages per-tenant API rate limits.
type TenantRateLimitHandler struct {
	kv core.KVStorer
}

func NewTenantRateLimitHandler(kv core.KVStorer) *TenantRateLimitHandler {
	return &TenantRateLimitHandler{kv: kv}
}

// RateLimitConfig defines API rate limits for a tenant.
type RateLimitConfig struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	BurstSize         int `json:"burst_size"`
	BuildsPerHour     int `json:"builds_per_hour"`
	DeploysPerHour    int `json:"deploys_per_hour"`
}

// defaultRateLimits returns sensible defaults.
func defaultRateLimits() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerMinute: 100,
		BurstSize:         20,
		BuildsPerHour:     10,
		DeploysPerHour:    20,
	}
}

// Get handles GET /api/v1/admin/tenants/{id}/ratelimit. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *TenantRateLimitHandler) Get(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var cfg RateLimitConfig
	if err := h.kv.Get("tenant_ratelimit", tenantID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultRateLimits())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/admin/tenants/{id}/ratelimit. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *TenantRateLimitHandler) Update(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var cfg RateLimitConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 100
	}
	if cfg.BurstSize <= 0 {
		cfg.BurstSize = 20
	}
	if err := h.kv.Set("tenant_ratelimit", tenantID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save rate limit config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"config":    cfg,
		"status":    "updated",
	})
}

// ──────────────────── tenant_settings.go ────────────────────
// TenantSettingsHandler manages per-tenant configuration.
type TenantSettingsHandler struct {
	store core.Store
}

func NewTenantSettingsHandler(store core.Store) *TenantSettingsHandler {
	return &TenantSettingsHandler{store: store}
}

// Get handles GET /api/v1/tenant/settings
func (h *TenantSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenant.ID,
		"name":      tenant.Name,
		"slug":      tenant.Slug,
		"plan_id":   tenant.PlanID,
		"status":    tenant.Status,
		"limits":    tenant.LimitsJSON,
		"metadata":  tenant.MetadataJSON,
	})
}

// Update handles PATCH /api/v1/tenant/settings
func (h *TenantSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Name     string `json:"name,omitempty"`
		Metadata string `json:"metadata,omitempty"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name must be 100 characters or less")
		return
	}
	if len(req.Metadata) > 64*1024 {
		writeError(w, http.StatusBadRequest, "metadata must be 64KB or less")
		return
	}
	tenant, err := h.store.GetTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if req.Name != "" {
		tenant.Name = req.Name
	}
	if req.Metadata != "" {
		tenant.MetadataJSON = req.Metadata
	}
	if err := h.store.UpdateTenant(r.Context(), tenant); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ──────────────────── admin_apikeys.go ────────────────────
// AdminAPIKeyHandler manages platform-level API keys.
type AdminAPIKeyHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewAdminAPIKeyHandler(store core.Store, kv core.KVStorer) *AdminAPIKeyHandler {
	return &AdminAPIKeyHandler{store: store, kv: kv}
}
func requireSuperAdminClaims(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	if claims.RoleID != "role_super_admin" {
		writeError(w, http.StatusForbidden, "insufficient privileges")
		return nil, false
	}
	return claims, true
}

// apiKeyRecord is persisted in KV storage for each API key.
type apiKeyRecord struct {
	Prefix    string     `json:"prefix"`
	Hash      string     `json:"hash"`
	Type      string     `json:"type"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // nil = no expiry
}
type apiKeyListItem struct {
	Prefix    string     `json:"prefix"`
	Type      string     `json:"type"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// apiKeyIndex maintains the list of all active API key prefixes.
type apiKeyIndex struct {
	Prefixes []string `json:"prefixes"`
}

// List handles GET /api/v1/admin/api-keys.
func (h *AdminAPIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSuperAdminClaims(w, r); !ok {
		return
	}
	var idx apiKeyIndex
	if err := h.kv.Get("api_keys", "_index", &idx); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	keys := make([]apiKeyListItem, 0, len(idx.Prefixes))
	for _, prefix := range idx.Prefixes {
		var rec apiKeyRecord
		if err := h.kv.Get("api_keys", prefix, &rec); err == nil {
			keys = append(keys, apiKeyListItem{
				Prefix:    rec.Prefix,
				Type:      rec.Type,
				CreatedBy: rec.CreatedBy,
				CreatedAt: rec.CreatedAt,
				ExpiresAt: rec.ExpiresAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": keys, "total": len(keys)})
}

// Generate handles POST /api/v1/admin/api-keys.
func (h *AdminAPIKeyHandler) Generate(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireSuperAdminClaims(w, r)
	if !ok {
		return
	}
	pair, err := auth.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}
	rec := apiKeyRecord{
		Prefix:    pair.Prefix,
		Hash:      pair.Hash,
		Type:      "platform",
		CreatedBy: claims.UserID,
		CreatedAt: time.Now(),
	}
	// Store the key record
	if err := h.kv.Set("api_keys", pair.Prefix, rec, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store api key")
		return
	}
	// Update the index
	var idx apiKeyIndex
	_ = h.kv.Get("api_keys", "_index", &idx)
	idx.Prefixes = append(idx.Prefixes, pair.Prefix)
	if err := h.kv.Set("api_keys", "_index", idx, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update key index")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"key":     pair.Key, // Shown only once
		"prefix":  pair.Prefix,
		"type":    "platform",
		"message": "Save this key — it will not be shown again",
	})
}

// CleanupExpiredKeys removes API keys that have passed their expiry time.
// Safe to call periodically from a background scheduler.
func (h *AdminAPIKeyHandler) CleanupExpiredKeys() int {
	var idx apiKeyIndex
	if err := h.kv.Get("api_keys", "_index", &idx); err != nil {
		return 0
	}
	now := time.Now()
	var removed int
	active := make([]string, 0, len(idx.Prefixes))
	for _, prefix := range idx.Prefixes {
		var rec apiKeyRecord
		if err := h.kv.Get("api_keys", prefix, &rec); err != nil {
			continue // key gone, skip
		}
		if rec.ExpiresAt != nil && now.After(*rec.ExpiresAt) {
			if err := h.kv.Delete("api_keys", prefix); err != nil {
				slog.Error("failed to delete expired API key", "prefix", prefix, "error", err)
			}
			removed++
		} else {
			active = append(active, prefix)
		}
	}
	if removed > 0 {
		idx.Prefixes = active
		if err := h.kv.Set("api_keys", "_index", idx, 0); err != nil {
			slog.Error("failed to update API key index after cleanup", "error", err)
		}
	}
	return removed
}

// Revoke handles DELETE /api/v1/admin/api-keys/{prefix}.
func (h *AdminAPIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSuperAdminClaims(w, r); !ok {
		return
	}
	prefix, ok := requirePathParam(w, r, "prefix")
	if !ok {
		return
	}
	// Delete the key record
	_ = h.kv.Delete("api_keys", prefix)
	// Update the index
	var idx apiKeyIndex
	if err := h.kv.Get("api_keys", "_index", &idx); err == nil {
		filtered := make([]string, 0, len(idx.Prefixes))
		for _, p := range idx.Prefixes {
			if p != prefix {
				filtered = append(filtered, p)
			}
		}
		idx.Prefixes = filtered
		_ = h.kv.Set("api_keys", "_index", idx, 0)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ──────────────────── admin.go ────────────────────
// AdminHandler serves system administration endpoints.
type AdminHandler struct {
	core    *core.Core
	store   core.Store
	authMod AuthServices
}

// NewAdminHandler creates an AdminHandler. authMod may be nil in tests
// that don't exercise key revocation — RevokeAllKeys checks before use.
func NewAdminHandler(c *core.Core, store core.Store, authMod ...AuthServices) *AdminHandler {
	var mod AuthServices
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
	stats := eventBusStats(h.core.Events)
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
	if !decodeJSONInto(w, r, &settings) {
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
