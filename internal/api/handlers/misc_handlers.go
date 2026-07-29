package handlers

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── activity.go ────────────────────
// ActivityHandler serves tenant activity feed.
type ActivityHandler struct {
	store core.Store
}

func NewActivityHandler(store core.Store) *ActivityHandler {
	return &ActivityHandler{store: store}
}

// Feed handles GET /api/v1/activity
// Returns recent audit log entries as an activity feed.
func (h *ActivityHandler) Feed(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	pg := parsePagination(r)
	entries, total, err := h.store.ListAuditLogs(r.Context(), claims.TenantID, pg.PerPage, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writePaginatedJSON(w, entries, total, pg)
}

// ──────────────────── dashboard.go ────────────────────
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
		domains, err := h.store.ListDomainsByApp(r.Context(), app.ID, claims.TenantID)
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

// ──────────────────── search.go ────────────────────
// SearchHandler provides unified search across resources.
type SearchHandler struct {
	store core.Store
}

func NewSearchHandler(store core.Store) *SearchHandler {
	return &SearchHandler{store: store}
}

// SearchResult represents a single search match.
type SearchResult struct {
	Type string `json:"type"` // app, domain, project
	ID   string `json:"id"`
	Name string `json:"name"`
	Info string `json:"info,omitempty"`
}

// Search handles GET /api/v1/search?q=...
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query == "" || len(query) < 2 {
		writeError(w, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}
	apps, _, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 10000, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var results []SearchResult
	// Search apps
	for _, app := range apps {
		if strings.Contains(strings.ToLower(app.Name), query) {
			results = append(results, SearchResult{
				Type: "app", ID: app.ID, Name: app.Name, Info: app.Status,
			})
		}
	}
	// Search domains for the current tenant's applications only.
	// Batch-fetch domains for all apps in a single query to avoid N+1.
	appIDs := make([]string, len(apps))
	for i, app := range apps {
		appIDs[i] = app.ID
	}
	domainsByApp, err := h.store.ListDomainsByAppIDs(r.Context(), appIDs, claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, app := range apps {
		domains := domainsByApp[app.ID]
		for _, d := range domains {
			if strings.Contains(strings.ToLower(d.FQDN), query) {
				results = append(results, SearchResult{
					Type: "domain", ID: d.ID, Name: d.FQDN, Info: d.Type,
				})
			}
		}
	}
	// Search projects
	projects, err := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			results = append(results, SearchResult{
				Type: "project", ID: p.ID, Name: p.Name, Info: p.Environment,
			})
		}
	}
	// Limit results
	if len(results) > 20 {
		results = results[:20]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"results": results,
		"total":   len(results),
	})
}

// ──────────────────── error_pages.go ────────────────────
// ErrorPageHandler manages custom error pages per app.
type ErrorPageHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewErrorPageHandler(store core.Store, kv core.KVStorer) *ErrorPageHandler {
	return &ErrorPageHandler{store: store, kv: kv}
}

// ErrorPageConfig holds custom error page HTML per status code.
type ErrorPageConfig struct {
	Page502         string `json:"page_502,omitempty"`         // Bad Gateway
	Page503         string `json:"page_503,omitempty"`         // Service Unavailable
	Page504         string `json:"page_504,omitempty"`         // Gateway Timeout
	PageMaintenance string `json:"page_maintenance,omitempty"` // Maintenance
}

// Get handles GET /api/v1/apps/{id}/error-pages
func (h *ErrorPageHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg ErrorPageConfig
	if err := h.kv.Get("error_pages", app.ID, &cfg); err != nil {
		// No custom pages — return empty config
		writeJSON(w, http.StatusOK, ErrorPageConfig{})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/error-pages
func (h *ErrorPageHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg ErrorPageConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	const maxPageBytes = 1 << 20 // 1 MB per page
	pages := map[string]string{
		"page_502":         cfg.Page502,
		"page_503":         cfg.Page503,
		"page_504":         cfg.Page504,
		"page_maintenance": cfg.PageMaintenance,
	}
	for name, body := range pages {
		if len(body) > maxPageBytes {
			writeError(w, http.StatusBadRequest, name+" exceeds 1 MB limit")
			return
		}
	}
	if err := h.kv.Set("error_pages", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save error pages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}

// ──────────────────── response_headers.go ────────────────────
// httpTokenPattern is the RFC 7230 "token" grammar used for HTTP field-names.
// Rejects CRLF, whitespace, and the separator characters that would allow a
// caller to inject a new header line by embedding "\r\nSet-Cookie: ..." in
// the name (same class of bug as the sticky-sessions cookie-name fix).
var httpTokenPattern = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

// ResponseHeadersHandler manages per-app security and custom response headers.
type ResponseHeadersHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewResponseHeadersHandler(store core.Store, kv core.KVStorer) *ResponseHeadersHandler {
	return &ResponseHeadersHandler{store: store, kv: kv}
}

// ResponseHeadersConfig defines custom response headers for the ingress.
type ResponseHeadersConfig struct {
	HSTS              string            `json:"hsts,omitempty"`               // Strict-Transport-Security
	CSP               string            `json:"csp,omitempty"`                // Content-Security-Policy
	XFrameOptions     string            `json:"x_frame_options,omitempty"`    // DENY, SAMEORIGIN
	XContentType      string            `json:"x_content_type,omitempty"`     // nosniff
	ReferrerPolicy    string            `json:"referrer_policy,omitempty"`    // strict-origin-when-cross-origin
	PermissionsPolicy string            `json:"permissions_policy,omitempty"` // camera=(), microphone=()
	Custom            map[string]string `json:"custom,omitempty"`
}

// defaultResponseHeaders returns secure defaults.
func defaultResponseHeaders() ResponseHeadersConfig {
	return ResponseHeadersConfig{
		XFrameOptions:  "DENY",
		XContentType:   "nosniff",
		ReferrerPolicy: "strict-origin-when-cross-origin",
	}
}

// Get handles GET /api/v1/apps/{id}/response-headers
func (h *ResponseHeadersHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg ResponseHeadersConfig
	if err := h.kv.Get("response_headers", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultResponseHeaders())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/response-headers
func (h *ResponseHeadersHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg ResponseHeadersConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	const maxCustomHeaders = 50
	const maxHeaderValueLen = 4096
	if len(cfg.Custom) > maxCustomHeaders {
		writeError(w, http.StatusBadRequest, "too many custom headers (max 50)")
		return
	}
	for name, value := range cfg.Custom {
		if !httpTokenPattern.MatchString(name) {
			writeError(w, http.StatusBadRequest, "invalid header name: must match RFC 7230 token grammar")
			return
		}
		if len(value) > maxHeaderValueLen {
			writeError(w, http.StatusBadRequest, "header value exceeds 4096 characters")
			return
		}
		for i := 0; i < len(value); i++ {
			if value[i] == '\r' || value[i] == '\n' {
				writeError(w, http.StatusBadRequest, "header value must not contain CR or LF")
				return
			}
		}
	}
	if err := h.kv.Set("response_headers", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save response headers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}

// ──────────────────── sticky_sessions.go ────────────────────
// validCookieName matches an RFC 6265 cookie-name (a token). Rejecting
// non-token characters is what prevents Set-Cookie header splitting via
// a user-supplied cookie name.
var validCookieName = regexp.MustCompile(`^[!#$%&'*+\-.^_` + "`" + `|~0-9A-Za-z]+$`)

// StickySessionHandler configures session affinity for load-balanced apps.
type StickySessionHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewStickySessionHandler(store core.Store, kv core.KVStorer) *StickySessionHandler {
	return &StickySessionHandler{store: store, kv: kv}
}

// StickySessionConfig holds cookie-based session affinity settings.
type StickySessionConfig struct {
	Enabled  bool   `json:"enabled"`
	Cookie   string `json:"cookie"`  // Cookie name (default: MONSTER_AFFINITY)
	MaxAge   int    `json:"max_age"` // Seconds
	Secure   bool   `json:"secure"`
	HTTPOnly bool   `json:"http_only"`
	SameSite string `json:"same_site"` // lax, strict, none
}

// defaultStickyConfig returns secure defaults.
func defaultStickyConfig() StickySessionConfig {
	return StickySessionConfig{
		Enabled: false, Cookie: "MONSTER_AFFINITY", MaxAge: 3600,
		Secure: true, HTTPOnly: true, SameSite: "lax",
	}
}

// Get handles GET /api/v1/apps/{id}/sticky-sessions
func (h *StickySessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg StickySessionConfig
	if err := h.kv.Get("sticky_sessions", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultStickyConfig())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/sticky-sessions
func (h *StickySessionHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg StickySessionConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if cfg.Cookie == "" {
		cfg.Cookie = "MONSTER_AFFINITY"
	}
	// RFC 6265 token chars only — otherwise an attacker-controlled cookie
	// name ending in `; Path=/; Set-Cookie:` could split the Set-Cookie
	// header when the reverse proxy writes it.
	if !validCookieName.MatchString(cfg.Cookie) {
		writeError(w, http.StatusBadRequest, "cookie name must be RFC 6265 token characters only (alphanumerics, !#$%&'*+-.^_`|~)")
		return
	}
	if len(cfg.Cookie) > 128 {
		writeError(w, http.StatusBadRequest, "cookie name must be 128 characters or fewer")
		return
	}
	// 0 = session cookie; otherwise positive with a hard cap (1 year).
	if cfg.MaxAge < 0 {
		writeError(w, http.StatusBadRequest, "max_age must be zero or positive")
		return
	}
	if cfg.MaxAge > 31536000 {
		writeError(w, http.StatusBadRequest, "max_age must be 31536000 seconds (1 year) or fewer")
		return
	}
	if cfg.SameSite == "" {
		cfg.SameSite = "lax"
	}
	if cfg.SameSite != "lax" && cfg.SameSite != "strict" && cfg.SameSite != "none" {
		writeError(w, http.StatusBadRequest, "same_site must be one of: lax, strict, none")
		return
	}
	if err := h.kv.Set("sticky_sessions", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save sticky session config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}

// ──────────────────── announcements.go ────────────────────
// AnnouncementHandler manages platform-wide announcements.
type AnnouncementHandler struct {
	kv core.KVStorer
}

func NewAnnouncementHandler(kv core.KVStorer) *AnnouncementHandler {
	return &AnnouncementHandler{kv: kv}
}

// Announcement is a platform-wide broadcast message.
type Announcement struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Type      string     `json:"type"` // info, warning, critical, maintenance
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// announcementList wraps the persisted list of announcements.
type announcementList struct {
	Items []Announcement `json:"items"`
}

// List handles GET /api/v1/announcements
// Returns active announcements for the dashboard banner.
func (h *AnnouncementHandler) List(w http.ResponseWriter, _ *http.Request) {
	var list announcementList
	_ = h.kv.Get("announcements", "all", &list)
	active := make([]Announcement, 0)
	now := time.Now()
	for _, a := range list.Items {
		if a.Active && (a.ExpiresAt == nil || a.ExpiresAt.After(now)) {
			active = append(active, a)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": active, "total": len(active)})
}

// Create handles POST /api/v1/admin/announcements. Authorized by
// middleware.RequireSuperAdmin at the router.
func (h *AnnouncementHandler) Create(w http.ResponseWriter, r *http.Request) {
	var a Announcement
	if !decodeJSONInto(w, r, &a) {
		return
	}
	if len(a.Title) > 200 {
		writeError(w, http.StatusBadRequest, "title must be 200 characters or less")
		return
	}
	if len(a.Body) > 10000 {
		writeError(w, http.StatusBadRequest, "body must be 10000 characters or less")
		return
	}
	// Validate announcement type
	switch a.Type {
	case "info", "warning", "critical", "maintenance":
		// valid
	default:
		writeError(w, http.StatusBadRequest, "type must be one of: info, warning, critical, maintenance")
		return
	}
	a.ID = core.GenerateID()
	a.Active = true
	a.CreatedAt = time.Now()
	var list announcementList
	_ = h.kv.Get("announcements", "all", &list)
	if len(list.Items) >= 100 {
		writeError(w, http.StatusConflict, "announcement limit reached (100)")
		return
	}
	list.Items = append(list.Items, a)
	if err := h.kv.Set("announcements", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save announcement")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// Dismiss handles DELETE /api/v1/admin/announcements/{id}
func (h *AnnouncementHandler) Dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var list announcementList
	if err := h.kv.Get("announcements", "all", &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for i := range list.Items {
		if list.Items[i].ID == id {
			list.Items[i].Active = false
			break
		}
	}
	if err := h.kv.Set("announcements", "all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update announcement")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
