package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/gitsources/providers"
)

const gitProviderConnectionsBucket = "git_provider_connections"

type gitTokenVault interface {
	Encrypt(string) (string, error)
	Decrypt(string) (string, error)
}

// GitSourceHandler manages Git provider connections.
type GitSourceHandler struct {
	services *core.Services
	bolt     core.BoltStorer
	vault    gitTokenVault
}

func NewGitSourceHandler(services *core.Services, bolt core.BoltStorer, vault gitTokenVault) *GitSourceHandler {
	return &GitSourceHandler{services: services, bolt: bolt, vault: vault}
}

type gitProviderConnectionRecord struct {
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	Name          string    `json:"name"`
	URL           string    `json:"url,omitempty"`
	TokenEnc      string    `json:"token_enc"`
	Connected     bool      `json:"connected"`
	RepoCount     int       `json:"repo_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastRepoError string    `json:"last_repo_error,omitempty"`
}

type gitProviderView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Connected bool   `json:"connected"`
	RepoCount int    `json:"repo_count"`
	URL       string `json:"url,omitempty"`
}

type connectGitProviderRequest struct {
	Type  string `json:"type"`
	Token string `json:"token"`
	URL   string `json:"url,omitempty"`
}

// ListProviders handles GET /api/v1/git/providers
func (h *GitSourceHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	connected := map[string]gitProviderConnectionRecord{}
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil && h.bolt != nil {
		records, err := h.listConnections(claims.TenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list git providers")
			return
		}
		for _, record := range records {
			connected[record.Type] = record
		}
	}

	names := h.services.GitProviders()
	for providerType := range connected {
		if !containsString(names, providerType) {
			names = append(names, providerType)
		}
	}
	sort.Strings(names)

	views := make([]gitProviderView, 0, len(names))
	for _, name := range names {
		view := gitProviderView{
			ID:        name,
			Name:      gitProviderDisplayName(name),
			Type:      name,
			Connected: false,
			RepoCount: 0,
		}
		if record, ok := connected[name]; ok {
			view.Name = record.Name
			view.Connected = record.Connected
			view.RepoCount = record.RepoCount
			view.URL = record.URL
		}
		views = append(views, view)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": views})
}

// Connect handles POST /api/v1/git/providers.
func (h *GitSourceHandler) Connect(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.bolt == nil || h.vault == nil {
		writeError(w, http.StatusServiceUnavailable, "git provider storage is unavailable")
		return
	}

	var req connectGitProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	providerType := strings.ToLower(strings.TrimSpace(req.Type))
	token := strings.TrimSpace(req.Token)
	if providerType == "" || token == "" {
		writeError(w, http.StatusBadRequest, "provider type and token are required")
		return
	}
	if _, ok := providers.Registry[providerType]; !ok {
		writeError(w, http.StatusBadRequest, "unsupported git provider")
		return
	}
	if len(token) > 4096 {
		writeError(w, http.StatusBadRequest, "token is too large")
		return
	}

	url := strings.TrimSpace(req.URL)
	if len(url) > 512 {
		writeError(w, http.StatusBadRequest, "url is too large")
		return
	}

	tokenEnc, err := h.vault.Encrypt(token)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encrypt git token")
		return
	}

	now := time.Now().UTC()
	record := gitProviderConnectionRecord{
		ID:        providerType,
		Type:      providerType,
		Name:      gitProviderDisplayName(providerType),
		URL:       url,
		TokenEnc:  tokenEnc,
		Connected: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if existing, err := h.getConnection(claims.TenantID, providerType); err == nil {
		record.CreatedAt = existing.CreatedAt
		record.RepoCount = existing.RepoCount
	}

	if err := h.bolt.Set(gitProviderConnectionsBucket, gitProviderKey(claims.TenantID, providerType), record, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to store git provider")
		return
	}

	writeJSON(w, http.StatusCreated, gitProviderView{
		ID:        record.ID,
		Name:      record.Name,
		Type:      record.Type,
		Connected: record.Connected,
		RepoCount: record.RepoCount,
		URL:       record.URL,
	})
}

// Disconnect handles DELETE /api/v1/git/providers/{id}.
func (h *GitSourceHandler) Disconnect(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.bolt == nil {
		writeError(w, http.StatusServiceUnavailable, "git provider storage is unavailable")
		return
	}

	id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
	if id == "" {
		writeError(w, http.StatusBadRequest, "provider id is required")
		return
	}

	if err := h.bolt.Delete(gitProviderConnectionsBucket, gitProviderKey(claims.TenantID, id)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disconnect git provider")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListRepos handles GET /api/v1/git/{provider}/repos
func (h *GitSourceHandler) ListRepos(w http.ResponseWriter, r *http.Request) {
	providerName, ok := requirePathParam(w, r, "provider")
	if !ok {
		return
	}
	p := h.providerForRequest(r, providerName)
	if p == nil {
		writeError(w, http.StatusNotFound, "provider not configured")
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
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
	providerName, ok := requirePathParam(w, r, "provider")
	if !ok {
		return
	}
	repo, ok2 := requirePathParam(w, r, "repo")
	if !ok2 {
		return
	}

	p := h.providerForRequest(r, providerName)
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

func (h *GitSourceHandler) providerForRequest(r *http.Request, providerType string) core.GitProvider {
	providerType = strings.ToLower(strings.TrimSpace(providerType))
	claims := auth.ClaimsFromContext(r.Context())
	if claims != nil && h.vault != nil {
		if record, err := h.getConnection(claims.TenantID, providerType); err == nil && record.Connected && record.TokenEnc != "" {
			token, err := h.vault.Decrypt(record.TokenEnc)
			if err == nil {
				if factory, ok := providers.Registry[providerType]; ok {
					return factory(token)
				}
			}
		}
	}
	return h.services.GitProvider(providerType)
}

func (h *GitSourceHandler) getConnection(tenantID, providerType string) (gitProviderConnectionRecord, error) {
	var record gitProviderConnectionRecord
	if h.bolt == nil {
		return record, core.ErrNotFound
	}
	err := h.bolt.Get(gitProviderConnectionsBucket, gitProviderKey(tenantID, providerType), &record)
	return record, err
}

func (h *GitSourceHandler) listConnections(tenantID string) ([]gitProviderConnectionRecord, error) {
	if h.bolt == nil {
		return nil, nil
	}
	keys, err := h.bolt.List(gitProviderConnectionsBucket)
	if err != nil {
		return nil, err
	}
	prefix := tenantID + "/"
	records := make([]gitProviderConnectionRecord, 0)
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		var record gitProviderConnectionRecord
		if err := h.bolt.Get(gitProviderConnectionsBucket, key, &record); err == nil {
			records = append(records, record)
		}
	}
	return records, nil
}

func gitProviderKey(tenantID, providerType string) string {
	return tenantID + "/" + strings.ToLower(strings.TrimSpace(providerType))
}

func gitProviderDisplayName(providerType string) string {
	switch providerType {
	case "github":
		return "GitHub"
	case "gitlab":
		return "GitLab"
	case "gitea":
		return "Gitea"
	case "bitbucket":
		return "Bitbucket"
	default:
		return providerType
	}
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
