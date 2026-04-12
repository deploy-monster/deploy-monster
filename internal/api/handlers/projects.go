package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ProjectHandler handles project CRUD.
type ProjectHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewProjectHandler(store core.Store, events *core.EventBus) *ProjectHandler {
	return &ProjectHandler{store: store, events: events}
}

// List handles GET /api/v1/projects
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projects, err := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	pg := parsePagination(r)
	page, total := paginateSlice(projects, pg)
	writePaginatedJSON(w, page, total, pg)
}

// Create handles POST /api/v1/projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Environment string `json:"environment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name must be 100 characters or less")
		return
	}
	if len(req.Description) > 500 {
		writeError(w, http.StatusBadRequest, "description must be 500 characters or less")
		return
	}

	env := req.Environment
	if env == "" {
		env = "production"
	}

	project := &core.Project{
		TenantID:    claims.TenantID,
		Name:        req.Name,
		Description: req.Description,
		Environment: env,
	}

	if err := h.store.CreateProject(r.Context(), project); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventProjectCreated, "api",
			map[string]string{"id": project.ID, "name": project.Name}))
	}

	writeJSON(w, http.StatusCreated, project)
}

// requireTenantProject looks up a project by ID and verifies it belongs to
// the requesting user's tenant. Returns the project on success or writes an
// error and returns nil on failure.
func requireTenantProject(w http.ResponseWriter, r *http.Request, store core.Store) *core.Project {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return nil
	}

	id, ok := requirePathParam(w, r, "id")
	if !ok {
		return nil
	}

	project, err := store.GetProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return nil
	}

	if project.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "project not found")
		return nil
	}

	return project
}

// Get handles GET /api/v1/projects/{id}
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	project := requireTenantProject(w, r, h.store)
	if project == nil {
		return
	}
	writeJSON(w, http.StatusOK, project)
}

// Delete handles DELETE /api/v1/projects/{id}
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	project := requireTenantProject(w, r, h.store)
	if project == nil {
		return
	}

	if err := h.store.DeleteProject(r.Context(), project.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete project")
		return
	}

	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventProjectDeleted, "api",
			map[string]string{"id": project.ID, "name": project.Name}))
	}

	w.WriteHeader(http.StatusNoContent)
}
