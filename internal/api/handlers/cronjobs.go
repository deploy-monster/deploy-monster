package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CronJobHandler manages per-app scheduled tasks.
type CronJobHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewCronJobHandler(store core.Store, bolt core.BoltStorer) *CronJobHandler {
	return &CronJobHandler{store: store, bolt: bolt}
}

// CronJobConfig represents a scheduled task for an app.
type CronJobConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"` // cron expression or "@every 5m"
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

// cronJobList is the persisted list of cron jobs for an app.
type cronJobList struct {
	Jobs []CronJobConfig `json:"jobs"`
}

// List handles GET /api/v1/apps/{id}/cron
func (h *CronJobHandler) List(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var list cronJobList
	if err := h.bolt.Get("cronjobs", appID, &list); err != nil {
		// No cron jobs yet — return empty
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": list.Jobs, "total": len(list.Jobs)})
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

	// Load existing jobs
	var list cronJobList
	_ = h.bolt.Get("cronjobs", appID, &list)

	list.Jobs = append(list.Jobs, req)

	if err := h.bolt.Set("cronjobs", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save cron job")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id": appID,
		"job":    req,
	})
}

// Delete handles DELETE /api/v1/apps/{id}/cron/{jobId}
func (h *CronJobHandler) Delete(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	jobID := r.PathValue("jobId")

	var list cronJobList
	if err := h.bolt.Get("cronjobs", appID, &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	filtered := make([]CronJobConfig, 0, len(list.Jobs))
	for _, j := range list.Jobs {
		if j.ID != jobID {
			filtered = append(filtered, j)
		}
	}
	list.Jobs = filtered

	if err := h.bolt.Set("cronjobs", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update cron jobs")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
