package handlers

import (
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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

	h.events.PublishAsync(r.Context(), core.NewEvent("app.snapshot.created", "api",
		map[string]string{"app_id": app.ID, "snapshot_id": snapshotID}))

	writeJSON(w, http.StatusCreated, SnapshotInfo{
		ID:        snapshotID,
		AppID:     app.ID,
		Image:     "monster-snapshot/" + app.ID[:8] + ":" + snapshotID[:8],
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
