package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AutoscaleHandler manages autoscaling rules per app.
type AutoscaleHandler struct {
	store  core.Store
	bolt   core.BoltStorer
	events *core.EventBus
}

func NewAutoscaleHandler(store core.Store, bolt core.BoltStorer) *AutoscaleHandler {
	return &AutoscaleHandler{store: store, bolt: bolt}
}

// SetEvents sets the event bus for audit event emission.
func (h *AutoscaleHandler) SetEvents(events *core.EventBus) { h.events = events }

// AutoscaleConfig defines autoscaling behavior. The persisted shape is
// the same as the response shape so the response from GET round-trips
// through PUT without any field shuffling. LastDecision is read-only on
// GET — it is populated from the autoscale_decisions bucket the
// evaluator writes, omitted when no evaluation has run yet.
type AutoscaleConfig struct {
	Enabled        bool           `json:"enabled"`
	MinReplicas    int            `json:"min_replicas"`
	MaxReplicas    int            `json:"max_replicas"`
	CPUTarget      int            `json:"cpu_target_percent"` // Scale up when CPU exceeds this
	RAMTarget      int            `json:"ram_target_percent"` // Scale up when RAM exceeds this
	ScaleUpDelay   int            `json:"scale_up_delay_sec"` // Cooldown before scaling up
	ScaleDownDelay int            `json:"scale_down_delay_sec"`
	LastDecision   map[string]any `json:"last_decision,omitempty"`
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

// Get handles GET /api/v1/apps/{id}/autoscale.
// Returns the persisted config and, if the autoscale evaluator has run
// at least once for this app, the most recent decision (cpu/mem, the
// action, the reason). Decisions are written to the
// autoscale_decisions bucket by internal/autoscale.
func (h *AutoscaleHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	cfg := defaultAutoscaleConfig()
	stored := AutoscaleConfig{}
	if err := h.bolt.Get("autoscale", app.ID, &stored); err == nil {
		cfg = stored
		if cfg.MinReplicas == 0 && cfg.MaxReplicas == 0 && cfg.CPUTarget == 0 {
			// Empty struct round-trip on a fresh bolt entry — keep defaults.
			cfg = defaultAutoscaleConfig()
		}
	}

	var lastDecision map[string]any
	if err := h.bolt.Get("autoscale_decisions", app.ID, &lastDecision); err == nil && lastDecision != nil {
		cfg.LastDecision = lastDecision
	}

	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/autoscale
func (h *AutoscaleHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var cfg AutoscaleConfig
	if !decodeJSONInto(w, r, &cfg) {
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

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventAutoscaleUpdated, "api",
			map[string]string{"app_id": appID}))
	}

	writeJSON(w, http.StatusOK, map[string]any{"app_id": appID, "config": cfg, "status": "updated"})
}
