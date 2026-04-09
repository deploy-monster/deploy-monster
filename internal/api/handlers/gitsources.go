package handlers

import (
	"net/http"
	"strconv"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// GitSourceHandler manages Git provider connections.
type GitSourceHandler struct {
	services *core.Services
}

func NewGitSourceHandler(services *core.Services) *GitSourceHandler {
	return &GitSourceHandler{services: services}
}

// ListProviders handles GET /api/v1/git/providers
func (h *GitSourceHandler) ListProviders(w http.ResponseWriter, _ *http.Request) {
	names := h.services.GitProviders()
	providers := make([]map[string]string, len(names))
	for i, name := range names {
		providers[i] = map[string]string{"id": name, "name": name}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// ListRepos handles GET /api/v1/git/{provider}/repos
func (h *GitSourceHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	p := h.services.GitProvider(providerName)
	if p == nil {
		writeError(w, http.StatusNotFound, "provider not configured")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	repos, err := p.ListRepos(r.Context(), page, 20)
	if err != nil {
		internalError(w, "failed to list repos", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": repos, "page": page})
}

// ListBranches handles GET /api/v1/git/{provider}/repos/{repo}/branches
func (h *GitSourceHandler) ListBranches(w http.ResponseWriter, r *http.Request) {
	providerName := r.PathValue("provider")
	repo := r.PathValue("repo")

	p := h.services.GitProvider(providerName)
	if p == nil {
		writeError(w, http.StatusNotFound, "provider not configured")
		return
	}

	branches, err := p.ListBranches(r.Context(), repo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list branches")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": branches})
}
