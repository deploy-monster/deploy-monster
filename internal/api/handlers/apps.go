package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/deploy"
)

// AppHandler handles application CRUD and control endpoints.
type AppHandler struct {
	store core.Store
	core  *core.Core
}

// NewAppHandler creates a new app handler.
func NewAppHandler(store core.Store, c *core.Core) *AppHandler {
	return &AppHandler{store: store, core: c}
}

type createAppRequest struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	SourceType string `json:"source_type"`
	SourceURL  string `json:"source_url"`
	Branch     string `json:"branch"`
	ProjectID  string `json:"project_id"`
}

// List handles GET /api/v1/apps
func (h *AppHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	pg := parsePagination(r)

	apps, total, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, pg.PerPage, pg.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writePaginatedJSON(w, apps, total, pg)
}

// Create handles POST /api/v1/apps
func (h *AppHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateAppName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var fieldErrs []FieldError
	if len(req.SourceURL) > 2048 {
		fieldErrs = append(fieldErrs, FieldError{Field: "source_url", Message: "must be 2048 characters or fewer"})
	}
	// Validate git URL format before storing to prevent SSRF at build time
	if req.SourceURL != "" {
		if err := build.ValidateGitURL(req.SourceURL); err != nil {
			fieldErrs = append(fieldErrs, FieldError{Field: "source_url", Message: "invalid git URL: " + err.Error()})
		}
	}
	if len(req.Branch) > 100 {
		fieldErrs = append(fieldErrs, FieldError{Field: "branch", Message: "must be 100 characters or fewer"})
	}
	if len(req.Type) > 50 {
		fieldErrs = append(fieldErrs, FieldError{Field: "type", Message: "must be 50 characters or fewer"})
	}
	if len(req.SourceType) > 50 {
		fieldErrs = append(fieldErrs, FieldError{Field: "source_type", Message: "must be 50 characters or fewer"})
	}
	if len(req.ProjectID) > 100 {
		fieldErrs = append(fieldErrs, FieldError{Field: "project_id", Message: "must be 100 characters or fewer"})
	}
	if len(fieldErrs) > 0 {
		writeValidationErrors(w, "field validation failed", fieldErrs)
		return
	}

	appType := req.Type
	if appType == "" {
		appType = "service"
	}
	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "image"
	}
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}

	// Check for duplicate app name within tenant
	if _, err := h.store.GetAppByName(r.Context(), claims.TenantID, req.Name); err == nil {
		writeError(w, http.StatusConflict, "application with this name already exists")
		return
	}

	// Enforce per-tenant app quota (Config.Limits.MaxAppsPerTenant).
	// Zero/negative is treated as unlimited so existing deployments that
	// leave the field unset keep working. This is the app-count half of
	// Phase 3.3.6 quota enforcement — the CPU/RAM half lives in the
	// deploy pipeline where live container stats are already aggregated.
	if limit := h.core.Config.Limits.MaxAppsPerTenant; limit > 0 {
		_, total, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 1, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check app quota")
			return
		}
		if total >= limit {
			writeError(w, http.StatusTooManyRequests,
				fmt.Sprintf("tenant app quota exceeded (%d/%d)", total, limit))
			return
		}
	}

	app := &core.Application{
		ProjectID:  req.ProjectID,
		TenantID:   claims.TenantID,
		Name:       req.Name,
		Type:       appType,
		SourceType: sourceType,
		SourceURL:  req.SourceURL,
		Branch:     branch,
		Status:     "pending",
		Replicas:   1,
	}

	if err := h.store.CreateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create application")
		return
	}

	// Auto-generate subdomain if configured
	if h.core.Config.DNS.AutoSubdomain != "" {
		go deploy.AutoDomain(r.Context(), h.store, h.core.Events, app, h.core.Config.DNS.AutoSubdomain)
	}

	h.core.Events.Publish(r.Context(), core.Event{
		Type:   core.EventAppCreated,
		Source: "api",
		Data:   app,
	})

	writeJSON(w, http.StatusCreated, app)
}

// Get handles GET /api/v1/apps/{id}
func (h *AppHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// Delete handles DELETE /api/v1/apps/{id}
func (h *AppHandler) Delete(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	// Stop and remove containers if runtime is available
	if rt := h.core.Services.Container; rt != nil {
		containerName := "dm-" + app.ID
		_ = rt.Stop(r.Context(), containerName, 10)
		_ = rt.Remove(r.Context(), containerName, true)
	}

	// Cascade: delete associated domains
	h.store.DeleteDomainsByApp(r.Context(), app.ID)

	if err := h.store.DeleteApp(r.Context(), app.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete application")
		return
	}

	h.core.Events.Publish(r.Context(), core.Event{
		Type:   core.EventAppDeleted,
		Source: "api",
		Data:   map[string]string{"id": app.ID},
	})

	w.WriteHeader(http.StatusNoContent)
}

// Restart handles POST /api/v1/apps/{id}/restart
func (h *AppHandler) Restart(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	if err := h.store.UpdateAppStatus(r.Context(), app.ID, "running"); err != nil {
		writeError(w, http.StatusInternalServerError, "restart failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting"})
}

// Stop handles POST /api/v1/apps/{id}/stop
func (h *AppHandler) Stop(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	// Stop container if runtime is available
	if rt := h.core.Services.Container; rt != nil {
		_ = rt.Stop(r.Context(), "dm-"+app.ID, 10)
	}

	if err := h.store.UpdateAppStatus(r.Context(), app.ID, "stopped"); err != nil {
		writeError(w, http.StatusInternalServerError, "stop failed")
		return
	}

	h.core.Events.Publish(r.Context(), core.Event{
		Type:   core.EventAppStopped,
		Source: "api",
		Data:   map[string]string{"id": app.ID},
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

// Start handles POST /api/v1/apps/{id}/start
func (h *AppHandler) Start(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	if err := h.store.UpdateAppStatus(r.Context(), app.ID, "running"); err != nil {
		writeError(w, http.StatusInternalServerError, "start failed")
		return
	}

	h.core.Events.Publish(r.Context(), core.Event{
		Type:   core.EventAppStarted,
		Source: "api",
		Data:   map[string]string{"id": app.ID},
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "running"})
}
