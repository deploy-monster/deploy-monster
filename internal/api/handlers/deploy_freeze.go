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
	bolt   core.BoltStorer
}

func NewDeployFreezeHandler(store core.Store, events *core.EventBus, bolt core.BoltStorer) *DeployFreezeHandler {
	return &DeployFreezeHandler{store: store, events: events, bolt: bolt}
}

// FreezeWindow defines a time range where deployments are blocked.
type FreezeWindow struct {
	ID       string    `json:"id"`
	Reason   string    `json:"reason"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Active   bool      `json:"active"`
}

// freezeWindowList holds all freeze windows.
type freezeWindowList struct {
	Windows []FreezeWindow `json:"windows"`
}

// Get handles GET /api/v1/deploy/freeze
func (h *DeployFreezeHandler) Get(w http.ResponseWriter, _ *http.Request) {
	var list freezeWindowList
	if err := h.bolt.Get("deploy_freeze", "_all", &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "frozen": false})
		return
	}

	// Check if any freeze is currently active
	now := time.Now()
	frozen := false
	active := make([]FreezeWindow, 0)
	for _, fw := range list.Windows {
		if fw.Active && now.After(fw.StartsAt) && now.Before(fw.EndsAt) {
			frozen = true
		}
		if fw.Active {
			active = append(active, fw)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": active, "frozen": frozen})
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
		ID:       core.GenerateID(),
		Reason:   req.Reason,
		StartsAt: startsAt,
		EndsAt:   endsAt,
		Active:   true,
	}

	var list freezeWindowList
	_ = h.bolt.Get("deploy_freeze", "_all", &list)
	list.Windows = append(list.Windows, freeze)

	if err := h.bolt.Set("deploy_freeze", "_all", list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save freeze window")
		return
	}

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.freeze.created", "api",
		map[string]string{"freeze_id": freeze.ID, "reason": freeze.Reason}))

	writeJSON(w, http.StatusCreated, freeze)
}

// Delete handles DELETE /api/v1/deploy/freeze/{id}
func (h *DeployFreezeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	freezeID := r.PathValue("id")

	var list freezeWindowList
	if err := h.bolt.Get("deploy_freeze", "_all", &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	for i := range list.Windows {
		if list.Windows[i].ID == freezeID {
			list.Windows[i].Active = false
			break
		}
	}

	_ = h.bolt.Set("deploy_freeze", "_all", list, 0)

	h.events.PublishAsync(r.Context(), core.NewEvent("deploy.freeze.deleted", "api",
		map[string]string{"freeze_id": freezeID}))

	w.WriteHeader(http.StatusNoContent)
}
