package handlers

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// DependencyHandler shows relationships between apps, databases, and services.
type DependencyHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewDependencyHandler(store core.Store, runtime core.ContainerRuntime) *DependencyHandler {
	return &DependencyHandler{store: store, runtime: runtime}
}

// DependencyNode represents an element in the dependency graph.
type DependencyNode struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Type   string   `json:"type"` // app, database, cache, volume
	Status string   `json:"status"`
	Links  []string `json:"links"` // IDs this node depends on
}

// Graph handles GET /api/v1/apps/{id}/dependencies
func (h *DependencyHandler) Graph(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID

	nodes := []DependencyNode{
		{ID: app.ID, Name: app.Name, Type: "app", Status: app.Status, Links: []string{}},
	}

	// If compose stack, discover linked services
	if h.runtime != nil {
		containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{
			"monster.stack": app.Name,
		})
		for _, c := range containers {
			svcName := c.Labels["monster.stack.service"]
			if svcName != "" && c.Labels["monster.app.id"] != appID {
				nodes = append(nodes, DependencyNode{
					ID:     c.ID[:12],
					Name:   svcName,
					Type:   guessNodeType(c.Image),
					Status: c.State,
					Links:  []string{appID},
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"nodes":  nodes,
	})
}

func guessNodeType(image string) string {
	switch {
	case contains(image, "postgres"), contains(image, "mysql"), contains(image, "mariadb"):
		return "database"
	case contains(image, "redis"), contains(image, "memcache"):
		return "cache"
	case contains(image, "mongo"):
		return "database"
	default:
		return "service"
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
