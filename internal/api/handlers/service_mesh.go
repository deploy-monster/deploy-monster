package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ServiceMeshHandler configures inter-service communication rules.
type ServiceMeshHandler struct {
	store core.Store
	bolt  core.BoltStorer
}

func NewServiceMeshHandler(store core.Store, bolt core.BoltStorer) *ServiceMeshHandler {
	return &ServiceMeshHandler{store: store, bolt: bolt}
}

// ServiceLink defines a connection between two apps.
type ServiceLink struct {
	ID          string `json:"id"`
	SourceAppID string `json:"source_app_id"`
	TargetAppID string `json:"target_app_id"`
	EnvVar      string `json:"env_var"` // Env var name injected into source (e.g., DATABASE_URL)
	TargetPort  int    `json:"target_port"`
	Protocol    string `json:"protocol"` // tcp, http, grpc
}

// serviceLinkList is the persisted list of service links for an app.
type serviceLinkList struct {
	Links []ServiceLink `json:"links"`
}

// List handles GET /api/v1/apps/{id}/links
func (h *ServiceMeshHandler) List(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var list serviceLinkList
	if err := h.bolt.Get("service_mesh", appID, &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": list.Links, "total": len(list.Links)})
}

// Create handles POST /api/v1/apps/{id}/links
func (h *ServiceMeshHandler) Create(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	var link ServiceLink
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	link.ID = core.GenerateID()
	link.SourceAppID = appID
	if link.TargetAppID == "" {
		writeError(w, http.StatusBadRequest, "target_app_id required")
		return
	}
	if link.Protocol == "" {
		link.Protocol = "tcp"
	}

	// Load existing links
	var list serviceLinkList
	_ = h.bolt.Get("service_mesh", appID, &list)

	list.Links = append(list.Links, link)

	if err := h.bolt.Set("service_mesh", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save service link")
		return
	}

	writeJSON(w, http.StatusCreated, link)
}

// Delete handles DELETE /api/v1/apps/{id}/links/{targetId}
func (h *ServiceMeshHandler) Delete(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	targetID := r.PathValue("targetId")

	var list serviceLinkList
	if err := h.bolt.Get("service_mesh", appID, &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	filtered := make([]ServiceLink, 0, len(list.Links))
	for _, l := range list.Links {
		if l.ID != targetID && l.TargetAppID != targetID {
			filtered = append(filtered, l)
		}
	}
	list.Links = filtered

	if err := h.bolt.Set("service_mesh", appID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update service links")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
