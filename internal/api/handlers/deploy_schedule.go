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
}

func NewDeployScheduleHandler(store core.Store, events *core.EventBus) *DeployScheduleHandler {
	return &DeployScheduleHandler{store: store, events: events}
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

// Schedule handles POST /api/v1/apps/{id}/deploy/schedule
func (h *DeployScheduleHandler) Schedule(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

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

	writeJSON(w, http.StatusCreated, scheduled)
}

// ListScheduled handles GET /api/v1/apps/{id}/deploy/scheduled
func (h *DeployScheduleHandler) ListScheduled(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// CancelScheduled handles DELETE /api/v1/apps/{id}/deploy/scheduled/{scheduleId}
func (h *DeployScheduleHandler) CancelScheduled(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	_ = r.PathValue("scheduleId")
	w.WriteHeader(http.StatusNoContent)
}
