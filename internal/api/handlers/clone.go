package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CloneHandler duplicates an existing application.
type CloneHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewCloneHandler(store core.Store, events *core.EventBus) *CloneHandler {
	return &CloneHandler{store: store, events: events}
}

type cloneRequest struct {
	NewName string `json:"new_name"`
}

// Clone handles POST /api/v1/apps/{id}/clone
func (h *CloneHandler) Clone(w http.ResponseWriter, r *http.Request) {
	sourceID := r.PathValue("id")

	var req cloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	source, err := h.store.GetApp(r.Context(), sourceID)
	if err != nil {
		writeError(w, http.StatusNotFound, "source app not found")
		return
	}

	newName := req.NewName
	if newName == "" {
		newName = source.Name + "-copy"
	}

	clone := &core.Application{
		ProjectID:  source.ProjectID,
		TenantID:   source.TenantID,
		Name:       newName,
		Type:       source.Type,
		SourceType: source.SourceType,
		SourceURL:  source.SourceURL,
		Branch:     source.Branch,
		Dockerfile: source.Dockerfile,
		BuildPack:  source.BuildPack,
		LabelsJSON: source.LabelsJSON,
		Replicas:   source.Replicas,
		Status:     "pending",
	}

	if err := h.store.CreateApp(r.Context(), clone); err != nil {
		writeError(w, http.StatusInternalServerError, "clone failed")
		return
	}

	h.events.Publish(r.Context(), core.NewEvent(core.EventAppCreated, "api",
		core.AppEventData{AppID: clone.ID, AppName: clone.Name}))

	writeJSON(w, http.StatusCreated, clone)
}
