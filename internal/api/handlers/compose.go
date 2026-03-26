package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	ic "github.com/deploy-monster/deploy-monster/internal/compose"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ComposeHandler manages Docker Compose stacks.
type ComposeHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewComposeHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *ComposeHandler {
	return &ComposeHandler{store: store, runtime: runtime, events: events}
}

// Deploy handles POST /api/v1/stacks
// Accepts compose YAML in the request body and deploys all services.
func (h *ComposeHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Read compose YAML from body
	var req struct {
		Name    string            `json:"name"`
		YAML    string            `json:"yaml"`
		EnvVars map[string]string `json:"env_vars"`
	}

	if r.Header.Get("Content-Type") == "application/x-yaml" || r.Header.Get("Content-Type") == "text/yaml" {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		req.YAML = string(body)
		req.Name = r.URL.Query().Get("name")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if req.YAML == "" {
		writeError(w, http.StatusBadRequest, "yaml is required")
		return
	}
	if req.Name == "" {
		req.Name = "stack-" + core.GenerateID()[:8]
	}

	// Interpolate env vars
	yamlData := ic.Interpolate([]byte(req.YAML), req.EnvVars)

	cf, err := ic.Parse(yamlData)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid compose yaml: "+err.Error())
		return
	}

	// Create app record
	app := &core.Application{
		TenantID:   claims.TenantID,
		Name:       req.Name,
		Type:       "compose-stack",
		SourceType: "compose",
		Status:     "deploying",
		Replicas:   1,
	}
	projects, _ := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if len(projects) > 0 {
		app.ProjectID = projects[0].ID
	}
	h.store.CreateApp(r.Context(), app)

	// Deploy async - use background context to avoid cancellation when request completes
	go func() {
		ctx := context.Background()
		deployer := ic.NewStackDeployer(h.runtime, h.store, h.events, nil)
		err := deployer.Deploy(ctx, ic.DeployOpts{
			AppID:     app.ID,
			TenantID:  claims.TenantID,
			StackName: req.Name,
			Compose:   cf,
			EnvVars:   req.EnvVars,
		})
		if err != nil {
			h.store.UpdateAppStatus(ctx, app.ID, "failed")
		} else {
			h.store.UpdateAppStatus(ctx, app.ID, "running")
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"app_id":   app.ID,
		"name":     req.Name,
		"services": len(cf.Services),
		"order":    cf.DependencyOrder(),
		"status":   "deploying",
	})
}

// Validate handles POST /api/v1/stacks/validate
func (h *ComposeHandler) Validate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cf, err := ic.Parse([]byte(req.YAML))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":    true,
		"services": len(cf.Services),
		"order":    cf.DependencyOrder(),
	})
}
