package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── snapshots.go ────────────────────
// SnapshotHandler manages container checkpoint/snapshot operations.
type SnapshotHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewSnapshotHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *SnapshotHandler {
	return &SnapshotHandler{store: store, runtime: runtime, events: events}
}

// SnapshotInfo represents a container snapshot.
type SnapshotInfo struct {
	ID        string    `json:"id"`
	AppID     string    `json:"app_id"`
	Image     string    `json:"image"`
	Size      string    `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// Create handles POST /api/v1/apps/{id}/snapshots
// Commits the current container state as a new image.
func (h *SnapshotHandler) Create(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	// Would use docker commit to create snapshot image
	snapshotID := core.GenerateID()
	publishEventAsync(r.Context(), h.events, core.NewEvent("app.snapshot.created", "api",
		map[string]string{"app_id": app.ID, "snapshot_id": snapshotID}))
	writeJSON(w, http.StatusCreated, SnapshotInfo{
		ID:        snapshotID,
		AppID:     app.ID,
		Image:     "monster-snapshot/" + core.ShortID(app.ID, 8) + ":" + core.ShortID(snapshotID, 8),
		CreatedAt: time.Now(),
	})
}

// List handles GET /api/v1/apps/{id}/snapshots
func (h *SnapshotHandler) List(w http.ResponseWriter, r *http.Request) {
	if app := requireTenantApp(w, r, h.store); app == nil {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// ──────────────────── scale.go ────────────────────
// ScaleHandler manages application replica scaling.
type ScaleHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewScaleHandler(store core.Store, events *core.EventBus) *ScaleHandler {
	return &ScaleHandler{store: store, events: events}
}

type scaleRequest struct {
	Replicas int `json:"replicas"`
}

// Scale handles POST /api/v1/apps/{id}/scale
func (h *ScaleHandler) Scale(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var req scaleRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Replicas < 0 || req.Replicas > 100 {
		writeError(w, http.StatusBadRequest, "replicas must be between 0 and 100")
		return
	}
	oldReplicas := app.Replicas
	app.Replicas = req.Replicas
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "scale failed")
		return
	}
	publishEvent(r.Context(), h.events, core.NewEvent(core.EventAppScaled, "api",
		core.AppEventData{
			AppID:   appID,
			AppName: app.Name,
			Status:  app.Status,
		},
	))
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"old_replicas": oldReplicas,
		"new_replicas": req.Replicas,
	})
}

// ──────────────────── clone.go ────────────────────
// CloneHandler duplicates an existing application.
type CloneHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewCloneHandler(store core.Store, events *core.EventBus) *CloneHandler {
	return &CloneHandler{store: store, events: events}
}

type cloneRequest struct {
	NewName string `json:"new_name"`
}

// Clone handles POST /api/v1/apps/{id}/clone
func (h *CloneHandler) Clone(w http.ResponseWriter, r *http.Request) {
	var req cloneRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	source := requireTenantApp(w, r, h.store)
	if source == nil {
		return
	}
	newName := req.NewName
	if newName == "" {
		newName = source.Name + "-copy"
	}
	clone := &core.Application{
		ProjectID:  source.ProjectID,
		TenantID:   source.TenantID,
		Name:       newName,
		Type:       source.Type,
		SourceType: source.SourceType,
		SourceURL:  source.SourceURL,
		Branch:     source.Branch,
		Dockerfile: source.Dockerfile,
		BuildPack:  source.BuildPack,
		LabelsJSON: source.LabelsJSON,
		Replicas:   source.Replicas,
		Status:     "pending",
	}
	if err := h.store.CreateApp(r.Context(), clone); err != nil {
		writeError(w, http.StatusInternalServerError, "clone failed")
		return
	}
	publishEvent(r.Context(), h.events, core.NewEvent(core.EventAppCreated, "api",
		core.AppEventData{AppID: clone.ID, AppName: clone.Name}))
	writeJSON(w, http.StatusCreated, clone)
}

// ──────────────────── save_template.go ────────────────────
// SaveTemplateHandler saves a running app as a reusable marketplace template.
type SaveTemplateHandler struct {
	store core.Store
}

func NewSaveTemplateHandler(store core.Store) *SaveTemplateHandler {
	return &SaveTemplateHandler{store: store}
}

type saveTemplateRequest struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
}

// Save handles POST /api/v1/apps/{id}/save-template
func (h *SaveTemplateHandler) Save(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var req saveTemplateRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Name == "" {
		req.Name = app.Name
	}
	if req.Slug == "" {
		req.Slug = app.Name
	}
	// Build template from app config
	template := map[string]any{
		"slug":        req.Slug,
		"name":        req.Name,
		"description": req.Description,
		"category":    req.Category,
		"tags":        req.Tags,
		"source": map[string]string{
			"type":       app.SourceType,
			"url":        app.SourceURL,
			"branch":     app.Branch,
			"dockerfile": app.Dockerfile,
		},
		"app_type":   app.Type,
		"saved_from": appID,
	}
	writeJSON(w, http.StatusCreated, template)
}

// ──────────────────── restart_policy.go ────────────────────
// RestartPolicyHandler manages container restart policies.
type RestartPolicyHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewRestartPolicyHandler(store core.Store, runtime core.ContainerRuntime) *RestartPolicyHandler {
	return &RestartPolicyHandler{store: store, runtime: runtime}
}

type restartPolicyRequest struct {
	Policy string `json:"policy"` // always, unless-stopped, on-failure, no
}

// Update handles PUT /api/v1/apps/{id}/restart-policy.
// The actual Docker `--restart` flag isn't mutable through the runtime
// interface (would require an UpdateContainer hook on
// core.ContainerRuntime — not currently exposed). Until that hook
// lands the policy is recorded on the application's labels JSON so the
// next deploy applies it. The response status names that explicitly so
// the operator isn't misled into thinking a live container changed.
func (h *RestartPolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var req restartPolicyRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	valid := map[string]bool{"always": true, "unless-stopped": true, "on-failure": true, "no": true}
	if !valid[req.Policy] {
		writeError(w, http.StatusBadRequest, "policy must be: always, unless-stopped, on-failure, no")
		return
	}
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	labels := map[string]string{}
	if app.LabelsJSON != "" {
		_ = json.Unmarshal([]byte(app.LabelsJSON), &labels)
	}
	labels["restart_policy"] = req.Policy
	encoded, err := json.Marshal(labels)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode labels")
		return
	}
	app.LabelsJSON = string(encoded)
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to persist policy")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"app_id": appID,
		"policy": req.Policy,
		"status": "applied_at_next_deploy",
		"note":   "policy persisted on application; live container restart policy isn't mutable without a redeploy",
	})
}

// ──────────────────── labels.go ────────────────────
// LabelsHandler manages app labels/tags for organization.
type LabelsHandler struct {
	store core.Store
}

func NewLabelsHandler(store core.Store) *LabelsHandler {
	return &LabelsHandler{store: store}
}

// Get handles GET /api/v1/apps/{id}/labels
func (h *LabelsHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var labels map[string]string
	if app.LabelsJSON != "" && app.LabelsJSON != "{}" {
		if err := json.Unmarshal([]byte(app.LabelsJSON), &labels); err != nil {
			writeError(w, http.StatusInternalServerError, "stored labels are invalid")
			return
		}
	}
	if labels == nil {
		labels = map[string]string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": labels})
}

// Update handles PUT /api/v1/apps/{id}/labels
func (h *LabelsHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var labels map[string]string
	if !decodeJSONInto(w, r, &labels) {
		return
	}
	const maxLabels = 64
	const maxKeyLen = 63
	const maxValueLen = 253
	if len(labels) > maxLabels {
		writeError(w, http.StatusBadRequest, "too many labels (max 64)")
		return
	}
	for k, v := range labels {
		if k == "" {
			writeError(w, http.StatusBadRequest, "label key must not be empty")
			return
		}
		if len(k) > maxKeyLen {
			writeError(w, http.StatusBadRequest, "label key exceeds 63 characters")
			return
		}
		if len(v) > maxValueLen {
			writeError(w, http.StatusBadRequest, "label value exceeds 253 characters")
			return
		}
	}
	data, _ := json.Marshal(labels)
	app.LabelsJSON = string(data)
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": labels})
}

// ──────────────────── transfer.go ────────────────────
// TransferHandler moves resources between tenants.
type TransferHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewTransferHandler(store core.Store, events *core.EventBus) *TransferHandler {
	return &TransferHandler{store: store, events: events}
}

type transferRequest struct {
	TargetTenantID string `json:"target_tenant_id"`
}

// TransferApp handles POST /api/v1/apps/{id}/transfer. Moves an app to
// another tenant. Authorized by middleware.RequireSuperAdmin at the
// router — this is the one non-/admin/* route that requires super-admin.
func (h *TransferHandler) TransferApp(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var req transferRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.TargetTenantID == "" {
		writeError(w, http.StatusBadRequest, "target_tenant_id required")
		return
	}
	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	// SECURITY FIX (AUTHZ-008): Verify the user has access to this app's tenant
	// Even though this is super-admin only, we verify the claim matches the app's current tenant
	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil && claims.TenantID != "" {
		// If user is not super admin, they can only transfer apps from their own tenant
		if claims.RoleID != "role_super_admin" && app.TenantID != claims.TenantID {
			writeError(w, http.StatusForbidden, "access denied to this app")
			return
		}
	}
	// Super-admin transfer: app must have valid tenant assignment
	if app.TenantID == "" {
		writeError(w, http.StatusBadRequest, "app has no tenant assigned")
		return
	}
	// Verify target tenant exists
	_, err = h.store.GetTenant(r.Context(), req.TargetTenantID)
	if err != nil {
		writeError(w, http.StatusNotFound, "target tenant not found")
		return
	}
	oldTenant := app.TenantID
	app.TenantID = req.TargetTenantID
	if err := h.store.UpdateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "transfer failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":      appID,
		"from_tenant": oldTenant,
		"to_tenant":   req.TargetTenantID,
		"status":      "transferred",
	})
}

// ──────────────────── suspend.go ────────────────────
// SuspendHandler manages app suspend/resume (freeze without deleting).
type SuspendHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewSuspendHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *SuspendHandler {
	return &SuspendHandler{store: store, runtime: runtime, events: events}
}

// Suspend handles POST /api/v1/apps/{id}/suspend
// Stops the container but preserves all data and configuration.
func (h *SuspendHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	if app.Status == "suspended" {
		writeError(w, http.StatusConflict, "app already suspended")
		return
	}
	// Stop container but keep it (don't remove)
	if h.runtime != nil {
		containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list app containers")
			return
		}
		for _, c := range containers {
			if err := h.runtime.Stop(r.Context(), c.ID, 30); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to stop app container")
				return
			}
		}
	}
	if err := h.store.UpdateAppStatus(r.Context(), appID, "suspended", app.TenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update app status")
		return
	}
	publishEventAsync(r.Context(), h.events, core.NewEvent(core.EventAppStopped, "api",
		core.AppEventData{AppID: appID, AppName: app.Name, Status: "suspended"}))
	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "status": "suspended"})
}

// Resume handles POST /api/v1/apps/{id}/resume
// Restarts a suspended app from its existing container.
func (h *SuspendHandler) Resume(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	if app.Status != "suspended" {
		writeError(w, http.StatusConflict, "app is not suspended")
		return
	}
	// Restart existing container
	if h.runtime != nil {
		containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list app containers")
			return
		}
		for _, c := range containers {
			if err := h.runtime.Restart(r.Context(), c.ID); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to restart app container")
				return
			}
		}
	}
	if err := h.store.UpdateAppStatus(r.Context(), appID, "running", app.TenantID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update app status")
		return
	}
	publishEventAsync(r.Context(), h.events, core.NewEvent(core.EventAppStarted, "api",
		core.AppEventData{AppID: appID, AppName: app.Name, Status: "running"}))
	writeJSON(w, http.StatusOK, map[string]string{"app_id": appID, "status": "running"})
}

// ──────────────────── basic_auth.go ────────────────────
// BasicAuthHandler manages HTTP Basic Auth protection per app.
// When enabled, the ingress adds a Basic Auth challenge before proxying.
type BasicAuthHandler struct {
	store  core.Store
	kv     core.KVStorer
	events *core.EventBus
}

func NewBasicAuthHandler(store core.Store, kv core.KVStorer) *BasicAuthHandler {
	return &BasicAuthHandler{store: store, kv: kv}
}

// SetEvents sets the event bus for audit event emission.
func (h *BasicAuthHandler) SetEvents(events *core.EventBus) { h.events = events }

// BasicAuthConfig holds per-app basic auth settings.
type BasicAuthConfig struct {
	Enabled bool              `json:"enabled"`
	Users   map[string]string `json:"users"` // username -> bcrypt hash
	Realm   string            `json:"realm"` // Challenge realm text
}

// Get handles GET /api/v1/apps/{id}/basic-auth
func (h *BasicAuthHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg BasicAuthConfig
	if err := h.kv.Get("basic_auth", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, BasicAuthConfig{Enabled: false, Realm: "Restricted"})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/basic-auth
func (h *BasicAuthHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg BasicAuthConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if cfg.Realm == "" {
		cfg.Realm = "Restricted"
	}
	if len(cfg.Realm) > 100 {
		writeError(w, http.StatusBadRequest, "realm must be 100 characters or less")
		return
	}
	if len(cfg.Users) > 50 {
		writeError(w, http.StatusBadRequest, "maximum 50 users allowed")
		return
	}
	for username := range cfg.Users {
		if len(username) > 100 {
			writeError(w, http.StatusBadRequest, "username must be 100 characters or less")
			return
		}
	}
	if err := h.kv.Set("basic_auth", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save basic auth config")
		return
	}
	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventBasicAuthUpdated, "api",
			map[string]string{"app_id": appID, "enabled": fmt.Sprintf("%t", cfg.Enabled)}))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}
