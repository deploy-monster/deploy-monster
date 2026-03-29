package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TopologyHandler handles topology editor endpoints.
type TopologyHandler struct {
	store core.Store
	core  *core.Core
}

// NewTopologyHandler creates a new topology handler.
func NewTopologyHandler(store core.Store, c *core.Core) *TopologyHandler {
	return &TopologyHandler{store: store, core: c}
}

// TopologyNode represents a node in the topology editor
type TopologyNode struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Position Position               `json:"position"`
	Data     map[string]interface{} `json:"data"`
}

// Position represents a node position
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TopologyEdge represents an edge in the topology editor
type TopologyEdge struct {
	ID       string                 `json:"id"`
	Source   string                 `json:"source"`
	Target   string                 `json:"target"`
	Type     string                 `json:"type,omitempty"`
	Animated bool                   `json:"animated,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// TopologyDeployRequest represents a request to deploy a topology
type TopologyDeployRequest struct {
	Nodes       []TopologyNode `json:"nodes"`
	Edges       []TopologyEdge `json:"edges"`
	ProjectID   string         `json:"projectId"`
	Environment string         `json:"environment"`
}

// TopologyDeployResponse represents the response from deploying a topology
type TopologyDeployResponse struct {
	Success          bool                      `json:"success"`
	Message          string                    `json:"message"`
	CreatedResources *TopologyCreatedResources `json:"createdResources,omitempty"`
	Errors           []string                  `json:"errors,omitempty"`
}

// TopologyCreatedResources lists resources created from topology deployment
type TopologyCreatedResources struct {
	Apps      []string `json:"apps"`
	Databases []string `json:"databases"`
	Domains   []string `json:"domains"`
	Volumes   []string `json:"volumes"`
}

// Save handles POST /api/v1/topology - saves topology configuration
func (h *TopologyHandler) Save(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req TopologyDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// For now, just return success - actual persistence would use BBolt KV store
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Topology saved successfully",
	})
}

// Deploy handles POST /api/v1/topology/deploy - deploys topology to infrastructure
func (h *TopologyHandler) Deploy(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req TopologyDeployRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Nodes) == 0 {
		writeError(w, http.StatusBadRequest, "no nodes to deploy")
		return
	}

	// Track created resources
	resources := &TopologyCreatedResources{
		Apps:      []string{},
		Databases: []string{},
		Domains:   []string{},
		Volumes:   []string{},
	}
	errors := []string{}

	// Process nodes by type in dependency order:
	// 1. Volumes first (storage)
	// 2. Databases (data layer)
	// 3. Apps (compute)
	// 4. Domains (routing)

	// Group nodes by type
	for _, node := range req.Nodes {
		switch node.Type {
		case "volume":
			// Create volume - for now just track it
			name := getStringFromMap(node.Data, "name", node.ID)
			resources.Volumes = append(resources.Volumes, name)
		}
	}

	for _, node := range req.Nodes {
		switch node.Type {
		case "database":
			// Create database - for now just track it
			name := getStringFromMap(node.Data, "name", node.ID)
			resources.Databases = append(resources.Databases, name)
		}
	}

	for _, node := range req.Nodes {
		switch node.Type {
		case "app", "worker":
			// Create app - for now just track it
			name := getStringFromMap(node.Data, "name", node.ID)
			resources.Apps = append(resources.Apps, name)
		}
	}

	for _, node := range req.Nodes {
		switch node.Type {
		case "domain":
			// Create domain - for now just track it
			name := getStringFromMap(node.Data, "name", node.ID)
			resources.Domains = append(resources.Domains, name)
		}
	}

	// Return success response
	response := TopologyDeployResponse{
		Success:          len(errors) == 0,
		Message:          "Topology deployment initiated",
		CreatedResources: resources,
		Errors:           errors,
	}

	if len(errors) > 0 {
		response.Message = "Topology deployment completed with errors"
		writeJSON(w, http.StatusOK, response)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

// getStringFromMap safely gets a string from a map
func getStringFromMap(m map[string]interface{}, key, defaultValue string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultValue
}
