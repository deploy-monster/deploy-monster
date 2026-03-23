package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ServiceMeshHandler configures inter-service communication rules.
type ServiceMeshHandler struct {
	store core.Store
}

func NewServiceMeshHandler(store core.Store) *ServiceMeshHandler {
	return &ServiceMeshHandler{store: store}
}

// ServiceLink defines a connection between two apps.
type ServiceLink struct {
	SourceAppID string `json:"source_app_id"`
	TargetAppID string `json:"target_app_id"`
	EnvVar      string `json:"env_var"`    // Env var name injected into source (e.g., DATABASE_URL)
	TargetPort  int    `json:"target_port"`
	Protocol    string `json:"protocol"`   // tcp, http, grpc
}

// List handles GET /api/v1/apps/{id}/links
func (h *ServiceMeshHandler) List(w http.ResponseWriter, r *http.Request) {
	_ = r.PathValue("id")
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
}

// Create handles POST /api/v1/apps/{id}/links
func (h *ServiceMeshHandler) Create(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	var link ServiceLink
	if err := json.NewDecoder(r.Body).Decode(&link); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	link.SourceAppID = appID
	if link.TargetAppID == "" {
		writeError(w, http.StatusBadRequest, "target_app_id required")
		return
	}
	if link.Protocol == "" {
		link.Protocol = "tcp"
	}

	writeJSON(w, http.StatusCreated, link)
}

// Delete handles DELETE /api/v1/apps/{id}/links/{targetId}
func (h *ServiceMeshHandler) Delete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
