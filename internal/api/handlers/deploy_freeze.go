package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DeployFreezeHandler manages deployment freeze windows.
// When a freeze is active, new deployments are blocked.
type DeployFreezeHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewDeployFreezeHandler(store core.Store, events *core.EventBus) *DeployFreezeHandler {
	return &DeployFreezeHandler{store: store, events: events}
}

// FreezeWindow defines a time range where deployments are blocked.
type FreezeWindow struct {
	ID       string    `json:"id"`
	Reason   string    `json:"reason"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Active   bool      `json:"active"`
}

// Get handles GET /api/v1/deploy/freeze
func (h *DeployFreezeHandler) Get(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "frozen": false})
}

// Create handles POST /api/v1/deploy/freeze
func (h *DeployFreezeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Reason   string `json:"reason"`
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	startsAt, _ := time.Parse(time.RFC3339, req.StartsAt)
	endsAt, _ := time.Parse(time.RFC3339, req.EndsAt)

	if startsAt.IsZero() {
		startsAt = time.Now()
	}
	if endsAt.IsZero() {
		endsAt = startsAt.Add(24 * time.Hour)
	}

	freeze := FreezeWindow{
		ID: core.GenerateID(), Reason: req.Reason,
		StartsAt: startsAt, EndsAt: endsAt, Active: true,
	}
	writeJSON(w, http.StatusCreated, freeze)
}

// Delete handles DELETE /api/v1/deploy/freeze/{id}
func (h *DeployFreezeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
