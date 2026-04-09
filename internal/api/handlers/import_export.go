package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

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

// validAppTypes is the list of allowed application types.
var validAppTypes = map[string]bool{
	"web":      true,
	"worker":   true,
	"static":   true,
	"cron":     true,
	"docker":   true,
	"compose":  true,
	"database": true,
	"service":  true,
}

// validSourceTypes is the list of allowed source types.
var validSourceTypes = map[string]bool{
	"git":        true,
	"github":     true,
	"gitlab":     true,
	"image":      true,
	"tarball":    true,
	"docker":     true,
	"dockerfile": true,
}

// domainNameRegex validates domain names (simplified pattern).
var domainNameRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)

// Validate checks if the manifest has valid data.
func (m *AppManifest) Validate() []string {
	var errors []string

	// Validate name (required, no special chars)
	if m.Name == "" {
		errors = append(errors, "name is required")
	} else if len(m.Name) > 64 {
		errors = append(errors, "name must be at most 64 characters")
	} else if strings.ContainsAny(m.Name, "<>:\"/\\|?*") {
		errors = append(errors, "name contains invalid characters")
	}

	// Validate type (required, must be one of allowed types)
	if m.Type == "" {
		errors = append(errors, "type is required")
	} else if !validAppTypes[m.Type] {
		errors = append(errors, "invalid app type: "+m.Type)
	}

	// Validate source_type (required, must be one of allowed types)
	if m.SourceType == "" {
		errors = append(errors, "source_type is required")
	} else if !validSourceTypes[m.SourceType] {
		errors = append(errors, "invalid source_type: "+m.SourceType)
	}

	// Validate source_url (required, must be valid URL or image reference)
	if m.SourceURL == "" {
		errors = append(errors, "source_url is required")
	} else if m.SourceType == "image" || m.SourceType == "docker" {
		// For Docker images, validate as image reference (e.g., "nginx:latest", "ghcr.io/repo/app:v1")
		if strings.ContainsAny(m.SourceURL, ";\n\r") {
			errors = append(errors, "source_url contains invalid characters")
		}
	} else {
		u, err := url.Parse(m.SourceURL)
		if err != nil {
			errors = append(errors, "invalid source_url format")
		} else if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" && u.Scheme != "" {
			errors = append(errors, "source_url must use http, https, or ssh scheme")
		}
	}

	// Validate branch (no path traversal)
	if m.Branch != "" {
		if strings.Contains(m.Branch, "..") || strings.ContainsAny(m.Branch, ";\n\r") {
			errors = append(errors, "branch contains invalid characters")
		}
	}

	// Validate replicas (must be reasonable)
	if m.Replicas < 0 {
		errors = append(errors, "replicas must be non-negative")
	} else if m.Replicas > 100 {
		errors = append(errors, "replicas must be at most 100")
	}

	// Validate domains
	for _, domain := range m.Domains {
		if domain == "" {
			errors = append(errors, "domain cannot be empty")
		} else if len(domain) > 253 {
			errors = append(errors, "domain too long: "+domain)
		} else if !domainNameRegex.MatchString(domain) {
			errors = append(errors, "invalid domain format: "+domain)
		}
	}

	// Validate env var keys
	for key := range m.EnvVars {
		if key == "" {
			errors = append(errors, "env var key cannot be empty")
		} else if strings.ContainsAny(key, "=\n\r") {
			errors = append(errors, "env var key contains invalid characters: "+key)
		}
	}

	// Validate label keys
	for key := range m.Labels {
		if key == "" {
			errors = append(errors, "label key cannot be empty")
		}
	}

	return errors
}

// Export handles GET /api/v1/apps/{id}/export
func (h *ImportExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}

	domains, err := h.store.ListDomainsByApp(r.Context(), app.ID)
	if err != nil {
		slog.Warn("export: failed to list domains", "app_id", app.ID, "error", err)
	}
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
		writeError(w, http.StatusBadRequest, "invalid manifest format")
		return
	}

	// Security: Validate manifest before processing
	if validationErrors := manifest.Validate(); len(validationErrors) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "manifest validation failed",
			"details": validationErrors,
		})
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
	projects, pErr := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if pErr != nil {
		slog.Warn("import: failed to list projects", "error", pErr)
	}
	if len(projects) > 0 {
		app.ProjectID = projects[0].ID
	}

	if err := h.store.CreateApp(r.Context(), app); err != nil {
		writeError(w, http.StatusInternalServerError, "import failed")
		return
	}

	// Create domains
	var domainsCreated, domainsFailed int
	for _, fqdn := range manifest.Domains {
		if err := h.store.CreateDomain(r.Context(), &core.Domain{
			AppID: app.ID,
			FQDN:  fqdn,
			Type:  "custom",
		}); err != nil {
			domainsFailed++
		} else {
			domainsCreated++
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"app_id":          app.ID,
		"name":            app.Name,
		"domains_created": domainsCreated,
		"domains_failed":  domainsFailed,
	})
}
