package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AutoscaleHandler manages autoscaling rules per app.
type AutoscaleHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewAutoscaleHandler(store core.Store, bolt core.BoltStorer) *AutoscaleHandler {
	return &AutoscaleHandler{store: store, bolt: bolt}
}

// AutoscaleConfig defines autoscaling behavior.
type AutoscaleConfig struct {
	Enabled        bool `json:"enabled"`
	MinReplicas    int  `json:"min_replicas"`
	MaxReplicas    int  `json:"max_replicas"`
	CPUTarget      int  `json:"cpu_target_percent"` // Scale up when CPU exceeds this
	RAMTarget      int  `json:"ram_target_percent"` // Scale up when RAM exceeds this
	ScaleUpDelay   int  `json:"scale_up_delay_sec"` // Cooldown before scaling up
	ScaleDownDelay int  `json:"scale_down_delay_sec"`
}

// defaultAutoscaleConfig returns sensible defaults.
func defaultAutoscaleConfig() AutoscaleConfig {
	return AutoscaleConfig{
		Enabled:        false,
		MinReplicas:    1,
		MaxReplicas:    10,
		CPUTarget:      80,
		RAMTarget:      85,
		ScaleUpDelay:   60,
		ScaleDownDelay: 300,
	}
}

// Get handles GET /api/v1/apps/{id}/autoscale
func (h *AutoscaleHandler) Get(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg AutoscaleConfig
	if err := h.bolt.Get("autoscale", appID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, defaultAutoscaleConfig())
		return
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/autoscale
func (h *AutoscaleHandler) Update(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var cfg AutoscaleConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if cfg.MinReplicas < 1 {
		cfg.MinReplicas = 1
	}
	if cfg.MaxReplicas < cfg.MinReplicas {
		cfg.MaxReplicas = cfg.MinReplicas
	}

	if err := h.bolt.Set("autoscale", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save autoscale config")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
