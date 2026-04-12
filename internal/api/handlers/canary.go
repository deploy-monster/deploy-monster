package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CanaryHandler manages canary (gradual traffic shift) deployments.
type CanaryHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewCanaryHandler(store core.Store, events *core.EventBus) *CanaryHandler {
	return &CanaryHandler{store: store, events: events}
}

// CanaryConfig defines a canary deployment configuration.
type CanaryConfig struct {
	Enabled     bool   `json:"enabled"`
	NewImage    string `json:"new_image"`
	WeightNew   int    `json:"weight_new"`   // 0-100, percentage to new version
	WeightOld   int    `json:"weight_old"`   // auto-calculated: 100 - weight_new
	AutoPromote bool   `json:"auto_promote"` // Auto promote to 100% after health check
	StepPercent int    `json:"step_percent"` // Increment per step (e.g., 10%)
	StepDelay   int    `json:"step_delay"`   // Seconds between steps
}

// Get handles GET /api/v1/apps/{id}/canary
func (h *CanaryHandler) Get(w http.ResponseWriter, r *http.Request) {
	if app := requireTenantApp(w, r, h.store); app == nil {
		return
	}
	writeJSON(w, http.StatusOK, CanaryConfig{Enabled: false, WeightNew: 0, WeightOld: 100})
}

// Start handles POST /api/v1/apps/{id}/canary
func (h *CanaryHandler) Start(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	var cfg CanaryConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.NewImage == "" {
		writeError(w, http.StatusBadRequest, "new_image required")
		return
	}
	if cfg.WeightNew <= 0 || cfg.WeightNew > 100 {
		cfg.WeightNew = 10
	}
	cfg.WeightOld = 100 - cfg.WeightNew
	cfg.Enabled = true

	if cfg.StepPercent <= 0 {
		cfg.StepPercent = 10
	}
	if cfg.StepDelay <= 0 {
		cfg.StepDelay = 60
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.canary.started", "api",
		map[string]any{"app_id": app.ID, "new_image": cfg.NewImage, "weight": cfg.WeightNew}))

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id": app.ID,
		"config": cfg,
		"status": "canary_active",
	})
}

// Promote handles POST /api/v1/apps/{id}/canary/promote
// Shifts 100% traffic to the new version.
func (h *CanaryHandler) Promote(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.canary.promoted", "api",
		map[string]string{"app_id": app.ID}))

	writeJSON(w, http.StatusOK, map[string]string{
		"app_id": app.ID,
		"status": "promoted — 100% traffic to new version",
	})
}

// Cancel handles DELETE /api/v1/apps/{id}/canary
// Rolls back to 100% old version.
func (h *CanaryHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.canary.cancelled", "api",
		map[string]string{"app_id": app.ID}))

	writeJSON(w, http.StatusOK, map[string]string{
		"app_id": app.ID,
		"status": "cancelled — 100% traffic back to old version",
	})
}
