package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/compose"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// MarketplaceDeployHandler deploys marketplace templates.
type MarketplaceDeployHandler struct {
	registry *marketplace.TemplateRegistry
	runtime  core.ContainerRuntime
	store    core.Store
	events   *core.EventBus
}

func NewMarketplaceDeployHandler(registry *marketplace.TemplateRegistry, runtime core.ContainerRuntime, store core.Store, events *core.EventBus) *MarketplaceDeployHandler {
	return &MarketplaceDeployHandler{registry: registry, runtime: runtime, store: store, events: events}
}

type deployTemplateRequest struct {
	Slug   string            `json:"slug"`
	Name   string            `json:"name"`
	Config map[string]string `json:"config"` // User-provided config vars
}

// Deploy handles POST /api/v1/marketplace/deploy
func (h *MarketplaceDeployHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req deployTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}

	tmpl := h.registry.Get(req.Slug)
	if tmpl == nil {
		writeError(w, http.StatusNotFound, "template not found: "+req.Slug)
		return
	}

	appName := req.Name
	if appName == "" {
		appName = tmpl.Slug
	}

	// Interpolate config vars into compose YAML
	yamlData := compose.Interpolate([]byte(tmpl.ComposeYAML), req.Config)

	// Parse the compose file
	cf, err := compose.Parse(yamlData)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid template compose: "+err.Error())
		return
	}

	// Create app record
	app := &core.Application{
		TenantID:   claims.TenantID,
		Name:       appName,
		Type:       "compose-stack",
		SourceType: "marketplace",
		Status:     "deploying",
		Replicas:   1,
	}

	// Find or create default project
	projects, err := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if err != nil {
		slog.Warn("marketplace deploy: failed to list projects", "error", err)
	}
	if len(projects) > 0 {
		app.ProjectID = projects[0].ID
	}

	if err := h.store.CreateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create app")
		return
	}

	// Deploy via compose deployer (async) - use background context to avoid cancellation
	go func() {
		ctx := context.Background()
		deployer := compose.NewStackDeployer(h.runtime, h.store, h.events, nil)
		err := deployer.Deploy(ctx, compose.DeployOpts{
			AppID:     app.ID,
			TenantID:  claims.TenantID,
			StackName: appName,
			Compose:   cf,
			EnvVars:   req.Config,
		})
		if err != nil {
			h.store.UpdateAppStatus(ctx, app.ID, "failed")
		} else {
			h.store.UpdateAppStatus(ctx, app.ID, "running")
		}
	}()

	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventAppCreated, "marketplace", claims.TenantID, claims.UserID,
		core.AppEventData{AppID: app.ID, AppName: appName, Status: "deploying"},
	))

	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":   app.ID,
		"name":     appName,
		"template": tmpl.Slug,
		"status":   "deploying",
		"services": len(cf.Services),
	})
}
