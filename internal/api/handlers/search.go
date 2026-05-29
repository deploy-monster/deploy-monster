package handlers

import (
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// SearchHandler provides unified search across resources.
type SearchHandler struct {
	store core.Store
}

func NewSearchHandler(store core.Store) *SearchHandler {
	return &SearchHandler{store: store}
}

// SearchResult represents a single search match.
type SearchResult struct {
	Type string `json:"type"` // app, domain, project
	ID   string `json:"id"`
	Name string `json:"name"`
	Info string `json:"info,omitempty"`
}

// Search handles GET /api/v1/search?q=...
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if query == "" || len(query) < 2 {
		writeError(w, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}

	apps, _, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 10000, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var results []SearchResult

	// Search apps
	for _, app := range apps {
		if strings.Contains(strings.ToLower(app.Name), query) {
			results = append(results, SearchResult{
				Type: "app", ID: app.ID, Name: app.Name, Info: app.Status,
			})
		}
	}

	// Search domains for the current tenant's applications only.
	// Batch-fetch domains for all apps in a single query to avoid N+1.
	appIDs := make([]string, len(apps))
	for i, app := range apps {
		appIDs[i] = app.ID
	}
	domainsByApp, err := h.store.ListDomainsByAppIDs(r.Context(), appIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, app := range apps {
		domains := domainsByApp[app.ID]
		for _, d := range domains {
			if strings.Contains(strings.ToLower(d.FQDN), query) {
				results = append(results, SearchResult{
					Type: "domain", ID: d.ID, Name: d.FQDN, Info: d.Type,
				})
			}
		}
	}

	// Search projects
	projects, err := h.store.ListProjectsByTenant(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, p := range projects {
		if strings.Contains(strings.ToLower(p.Name), query) {
			results = append(results, SearchResult{
				Type: "project", ID: p.ID, Name: p.Name, Info: p.Environment,
			})
		}
	}

	// Limit results
	if len(results) > 20 {
		results = results[:20]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"results": results,
		"total":   len(results),
	})
}
