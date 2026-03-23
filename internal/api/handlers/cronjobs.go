package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CronJobHandler manages per-app scheduled tasks.
type CronJobHandler struct {
	store core.Store
}

func NewCronJobHandler(store core.Store) *CronJobHandler {
	return &CronJobHandler{store: store}
}

// CronJobConfig represents a scheduled task for an app.
type CronJobConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // cron expression or "@every 5m"
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

// List handles GET /api/v1/apps/{id}/cron
func (h *CronJobHandler) List(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// Create handles POST /api/v1/apps/{id}/cron
func (h *CronJobHandler) Create(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var req CronJobConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Schedule == "" || req.Command == "" {
		writeError(w, http.StatusBadRequest, "schedule and command are required")
		return
	}

	req.ID = core.GenerateID()
	req.Enabled = true

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id": appID,
		"job":    req,
	})
}

// Delete handles DELETE /api/v1/apps/{id}/cron/{jobId}
func (h *CronJobHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	_ = r.PathValue("jobId")
	w.WriteHeader(http.StatusNoContent)
}
