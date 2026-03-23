package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ImportExportHandler manages app configuration import/export.
type ImportExportHandler struct {
	store core.Store
}

func NewImportExportHandler(store core.Store) *ImportExportHandler {
	return &ImportExportHandler{store: store}
}

// AppManifest is the portable format for app configuration.
type AppManifest struct {
	Version    string            `json:"version"`
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	SourceType string            `json:"source_type"`
	SourceURL  string            `json:"source_url"`
	Branch     string            `json:"branch"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`
	Domains    []string          `json:"domains,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Replicas   int               `json:"replicas"`
}

// Export handles GET /api/v1/apps/{id}/export
func (h *ImportExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")

	app, err := h.store.GetApp(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}

	domains, _ := h.store.ListDomainsByApp(r.Context(), appID)
	domainNames := make([]string, len(domains))
	for i, d := range domains {
		domainNames[i] = d.FQDN
	}

	manifest := AppManifest{
		Version:    "1",
		Name:       app.Name,
		Type:       app.Type,
		SourceType: app.SourceType,
		SourceURL:  app.SourceURL,
		Branch:     app.Branch,
		Dockerfile: app.Dockerfile,
		Domains:    domainNames,
		Replicas:   app.Replicas,
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+app.Name+".json")
	writeJSON(w, http.StatusOK, manifest)
}

// Import handles POST /api/v1/apps/import
func (h *ImportExportHandler) Import(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var manifest AppManifest
	if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid manifest")
		return
	}

	app := &core.Application{
		TenantID:   claims.TenantID,
		Name:       manifest.Name,
		Type:       manifest.Type,
		SourceType: manifest.SourceType,
		SourceURL:  manifest.SourceURL,
		Branch:     manifest.Branch,
		Dockerfile: manifest.Dockerfile,
		Replicas:   manifest.Replicas,
		Status:     "pending",
	}

	// Find default project
	projects, _ := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if len(projects) > 0 {
		app.ProjectID = projects[0].ID
	}

	if err := h.store.CreateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "import failed")
		return
	}

	// Create domains
	for _, fqdn := range manifest.Domains {
		h.store.CreateDomain(r.Context(), &core.Domain{
			AppID: app.ID,
			FQDN:  fqdn,
			Type:  "custom",
		})
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id":  app.ID,
		"name":    app.Name,
		"domains": len(manifest.Domains),
	})
}
