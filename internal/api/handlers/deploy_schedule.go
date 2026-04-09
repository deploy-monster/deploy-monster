package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployScheduleHandler manages scheduled deployments.
type DeployScheduleHandler struct {
	store  core.Store
	events *core.EventBus
	bolt   core.BoltStorer
}

func NewDeployScheduleHandler(store core.Store, events *core.EventBus, bolt core.BoltStorer) *DeployScheduleHandler {
	return &DeployScheduleHandler{store: store, events: events, bolt: bolt}
}

// ScheduledDeploy represents a deployment scheduled for a future time.
type ScheduledDeploy struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Image       string    `json:"image,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	Strategy    string    `json:"strategy"`
	Status      string    `json:"status"` // pending, executed, cancelled
	CreatedAt   time.Time `json:"created_at"`
}

// scheduledDeployList holds all scheduled deploys for an app.
type scheduledDeployList struct {
	Items []ScheduledDeploy `json:"items"`
}

// Schedule handles POST /api/v1/apps/{id}/deploy/schedule
func (h *DeployScheduleHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var req struct {
		ScheduledAt string `json:"scheduled_at"` // RFC3339
		Image       string `json:"image,omitempty"`
		Branch      string `json:"branch,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "scheduled_at must be RFC3339 format")
		return
	}

	if scheduledAt.Before(time.Now()) {
		writeError(w, http.StatusBadRequest, "scheduled_at must be in the future")
		return
	}

	scheduled := ScheduledDeploy{
		ID:          core.GenerateID(),
		AppID:       appID,
		ScheduledAt: scheduledAt,
		Image:       req.Image,
		Branch:      req.Branch,
		Strategy:    "recreate",
		Status:      "pending",
		CreatedAt:   time.Now(),
	}

	// Load existing scheduled deploys and append
	var list scheduledDeployList
	_ = h.bolt.Get("deploy_schedule", appID, &list)
	list.Items = append(list.Items, scheduled)

	if err := h.bolt.Set("deploy_schedule", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save scheduled deploy")
		return
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.scheduled", "api",
		map[string]string{"app_id": appID, "schedule_id": scheduled.ID}))

	writeJSON(w, http.StatusCreated, scheduled)
}

// ListScheduled handles GET /api/v1/apps/{id}/deploy/scheduled
func (h *DeployScheduleHandler) ListScheduled(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var list scheduledDeployList
	if err := h.bolt.Get("deploy_schedule", appID, &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	// Return only pending items
	pending := make([]ScheduledDeploy, 0)
	for _, item := range list.Items {
		if item.Status == "pending" {
			pending = append(pending, item)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": pending, "total": len(pending)})
}

// CancelScheduled handles DELETE /api/v1/apps/{id}/deploy/scheduled/{scheduleId}
func (h *DeployScheduleHandler) CancelScheduled(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	scheduleID := r.PathValue("scheduleId")

	var list scheduledDeployList
	if err := h.bolt.Get("deploy_schedule", appID, &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for i := range list.Items {
		if list.Items[i].ID == scheduleID {
			list.Items[i].Status = "cancelled"
			break
		}
	}

	_ = h.bolt.Set("deploy_schedule", appID, list, 0)
	w.WriteHeader(http.StatusNoContent)
}
