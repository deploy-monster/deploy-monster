package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/ws"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/topology"
)

// TopologyHandler handles topology editor endpoints.
type TopologyHandler struct {
	store  core.Store
	core   *core.Core
	logger *slog.Logger
}

// NewTopologyHandler creates a new topology handler.
func NewTopologyHandler(store core.Store, c *core.Core) *TopologyHandler {
	return &TopologyHandler{store: store, core: c, logger: slog.Default()}
}

// TopologyNode represents a node in the topology editor
type TopologyNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Position Position       `json:"position"`
	Data     map[string]any `json:"data"`
}

// Position represents a node position
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// TopologyEdge represents an edge in the topology editor
type TopologyEdge struct {
	ID       string         `json:"id"`
	Source   string         `json:"source"`
	Target   string         `json:"target"`
	Type     string         `json:"type,omitempty"`
	Animated bool           `json:"animated,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

// TopologyDeployRequest represents a request to deploy a topology
type TopologyDeployRequest struct {
	Nodes       []TopologyNode `json:"nodes"`
	Edges       []TopologyEdge `json:"edges"`
	ProjectID   string         `json:"projectId"`
	Environment string         `json:"environment"`

	// Deploy options
	DryRun     bool `json:"dryRun"`
	ForceBuild bool `json:"forceBuild"`
	NoCache    bool `json:"noCache"`
}

// TopologyDeployResponse represents the response from deploying a topology
type TopologyDeployResponse struct {
	Success          bool                      `json:"success"`
	Message          string                    `json:"message"`
	CreatedResources *TopologyCreatedResources `json:"createdResources,omitempty"`

	// Generated files
	ComposeYAML string `json:"composeYaml,omitempty"`
	Caddyfile   string `json:"caddyfile,omitempty"`
	EnvFile     string `json:"envFile,omitempty"`

	// Deployed resources
	Containers []string `json:"containers,omitempty"`
	Networks   []string `json:"networks,omitempty"`
	Volumes    []string `json:"volumes,omitempty"`

	// Timing
	Duration string `json:"duration,omitempty"`

	Errors []string `json:"errors,omitempty"`
}

// TopologyCreatedResources lists resources created from topology deployment
type TopologyCreatedResources struct {
	Apps      []string `json:"apps"`
	Databases []string `json:"databases"`
	Domains   []string `json:"domains"`
	Volumes   []string `json:"volumes"`
}

// CompileResponse is the response for compile requests
type CompileResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`

	// Generated files
	ComposeYAML string `json:"composeYaml,omitempty"`
	Caddyfile   string `json:"caddyfile,omitempty"`
	EnvFile     string `json:"envFile,omitempty"`

	// Validation errors
	Errors []string `json:"errors,omitempty"`
}

// Save handles POST /api/v1/topology/save - saves topology configuration
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

	// Persist topology to BBolt KV store
	// Key format: topology:{tenantID}:{projectID}:{environment}
	key := fmt.Sprintf("topology:%s:%s:%s", claims.TenantID, req.ProjectID, req.Environment)

	// Store the entire request as JSON
	if err := h.core.DB.Bolt.Set("topologies", key, req, 0); err != nil {
		h.logger.Error("Failed to save topology", "error", err, "key", key)
		writeError(w, http.StatusInternalServerError, "failed to save topology")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Topology saved successfully",
	})
}

// Load handles GET /api/v1/topology/{projectId}/{environment} - loads saved topology
func (h *TopologyHandler) Load(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	projectID, ok := requirePathParam(w, r, "projectId")
	if !ok {
		return
	}
	environment, ok2 := requirePathParam(w, r, "environment")
	if !ok2 {
		return
	}

	// Tenant isolation: verify the project belongs to the calling user's tenant
	project, err := h.store.GetProject(r.Context(), projectID)
	if err != nil || project.TenantID != claims.TenantID {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	// Load topology from BBolt KV store
	key := fmt.Sprintf("topology:%s:%s:%s", claims.TenantID, projectID, environment)

	var req TopologyDeployRequest
	if err := h.core.DB.Bolt.Get("topologies", key, &req); err != nil {
		// No saved topology found - return empty state
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "No saved topology found",
			"nodes":   []TopologyNode{},
			"edges":   []TopologyEdge{},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"message":     "Topology loaded successfully",
		"nodes":       req.Nodes,
		"edges":       req.Edges,
		"projectId":   req.ProjectID,
		"environment": req.Environment,
	})
}

// Compile handles POST /api/v1/topology/compile - compiles topology to docker-compose
func (h *TopologyHandler) Compile(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "no nodes to compile")
		return
	}

	// Convert UI nodes to topology components
	t, errors := h.convertNodesToTopology(req)
	if len(errors) > 0 {
		writeJSON(w, http.StatusOK, CompileResponse{
			Success: false,
			Message: "Validation failed",
			Errors:  errors,
		})
		return
	}

	// Create compiler
	compiler := topology.NewCompiler(t, req.ProjectID, req.Environment)

	// Compile
	compose, err := compiler.Compile()
	if err != nil {
		writeJSON(w, http.StatusOK, CompileResponse{
			Success: false,
			Message: err.Error(),
			Errors:  []string{err.Error()},
		})
		return
	}

	// Generate additional files
	caddyfile := compiler.GenerateCaddyfile()
	envFile := compiler.GenerateEnvFile()

	writeJSON(w, http.StatusOK, CompileResponse{
		Success:     true,
		Message:     "Topology compiled successfully",
		ComposeYAML: compose.ToYAML(),
		Caddyfile:   caddyfile,
		EnvFile:     envFile,
	})
}

// Validate handles POST /api/v1/topology/validate
func (h *TopologyHandler) Validate(w http.ResponseWriter, r *http.Request) {
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

	// Convert and validate
	_, errors := h.convertNodesToTopology(req)

	writeJSON(w, http.StatusOK, map[string]any{
		"valid":  len(errors) == 0,
		"errors": errors,
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

	// Guard against path traversal in user-supplied path components
	// SECURITY: Decode URL-encoded sequences before checking, as attackers may use
	// URL encoding to bypass naive path traversal checks (e.g., %2e%2e%2f for ../)
	projectIDDecoded, _ := url.QueryUnescape(req.ProjectID)
	envDecoded, _ := url.QueryUnescape(req.Environment)
	if strings.ContainsAny(projectIDDecoded, "../\\") || strings.ContainsAny(envDecoded, "../\\") {
		writeError(w, http.StatusBadRequest, "invalid project ID or environment")
		return
	}

	start := time.Now()
	projectID := req.ProjectID

	// Broadcast validating stage
	ws.BroadcastValidating(projectID)

	// Convert UI nodes to topology components
	t, errors := h.convertNodesToTopology(req)
	if len(errors) > 0 {
		ws.BroadcastError(projectID, "Validation failed", errors)
		writeJSON(w, http.StatusOK, TopologyDeployResponse{
			Success: false,
			Message: "Validation failed",
			Errors:  errors,
		})
		return
	}

	// Broadcast compiling stage
	ws.BroadcastCompiling(projectID)

	// Create work directory for this deployment
	workDir := filepath.Join("/var/lib/deploymonster", "deployments", claims.TenantID, req.ProjectID, req.Environment)

	// Create compiler
	compiler := topology.NewCompiler(t, req.ProjectID, req.Environment)

	// Compile
	compose, err := compiler.Compile()
	if err != nil {
		ws.BroadcastError(projectID, "Compilation failed: "+err.Error(), []string{err.Error()})
		writeJSON(w, http.StatusOK, TopologyDeployResponse{
			Success: false,
			Message: "Compilation failed: " + err.Error(),
			Errors:  []string{err.Error()},
		})
		return
	}

	// Generate additional files
	caddyfile := compiler.GenerateCaddyfile()
	envFile := compiler.GenerateEnvFile()

	// If dry run, return generated files
	if req.DryRun {
		writeJSON(w, http.StatusOK, TopologyDeployResponse{
			Success:     true,
			Message:     "Dry run completed - files generated but not deployed",
			ComposeYAML: compose.ToYAML(),
			Caddyfile:   caddyfile,
			EnvFile:     envFile,
			Duration:    time.Since(start).String(),
		})
		return
	}

	// Broadcast deploying stage
	ws.BroadcastDeploying(projectID)

	// Create deployer
	deployer := topology.NewDeployer(workDir)

	// Broadcast deploying stage
	ws.BroadcastDeploying(projectID)

	// Deploy
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	result, err := deployer.Deploy(ctx, compose, caddyfile, envFile, false)
	if err != nil {
		var deployErrors []string
		for _, e := range result.Errors {
			deployErrors = append(deployErrors, fmt.Sprintf("%s: %s", e.Stage, e.Message))
		}
		ws.BroadcastError(projectID, "Deployment failed: "+err.Error(), deployErrors)
		writeJSON(w, http.StatusOK, TopologyDeployResponse{
			Success:     false,
			Message:     "Deployment failed: " + err.Error(),
			ComposeYAML: result.ComposeYAML,
			Caddyfile:   result.Caddyfile,
			EnvFile:     result.EnvFile,
			Errors:      deployErrors,
			Duration:    time.Since(start).String(),
		})
		return
	}

	// Build created resources list
	resources := &TopologyCreatedResources{
		Apps:      []string{},
		Databases: []string{},
		Domains:   []string{},
		Volumes:   []string{},
	}
	for _, app := range t.Apps {
		resources.Apps = append(resources.Apps, app.Name)
	}
	for _, db := range t.Databases {
		resources.Databases = append(resources.Databases, db.Name)
	}
	for _, d := range t.Domains {
		resources.Domains = append(resources.Domains, d.FQDN)
	}
	for _, v := range t.Volumes {
		resources.Volumes = append(resources.Volumes, v.Name)
	}

	// Broadcast success
	ws.BroadcastSuccess(projectID, time.Since(start).String(), result.Containers, result.Networks, result.Volumes)

	writeJSON(w, http.StatusOK, TopologyDeployResponse{
		Success:          true,
		Message:          result.Message,
		CreatedResources: resources,
		ComposeYAML:      result.ComposeYAML,
		Caddyfile:        result.Caddyfile,
		EnvFile:          result.EnvFile,
		Containers:       result.Containers,
		Networks:         result.Networks,
		Volumes:          result.Volumes,
		Duration:         time.Since(start).String(),
	})
}

// Templates handles GET /api/v1/topology/templates
func (h *TopologyHandler) Templates(w http.ResponseWriter, r *http.Request) {
	templates := []map[string]any{
		{
			"id":          "web-app-with-db",
			"name":        "Web App with Database",
			"description": "A simple web application with a PostgreSQL database",
			"nodes": []TopologyNode{
				{ID: "app-1", Type: "app", Position: Position{X: 250, Y: 150}, Data: map[string]any{
					"name":      "web",
					"gitUrl":    "https://github.com/user/myapp",
					"branch":    "main",
					"port":      3000,
					"replicas":  2,
					"buildPack": "auto",
				}},
				{ID: "db-1", Type: "database", Position: Position{X: 500, Y: 150}, Data: map[string]any{
					"name":    "db",
					"engine":  "postgres",
					"version": "16",
					"sizeGB":  10,
				}},
				{ID: "vol-1", Type: "volume", Position: Position{X: 250, Y: 300}, Data: map[string]any{
					"name":      "uploads",
					"sizeGB":    5,
					"mountPath": "/app/uploads",
				}},
				{ID: "domain-1", Type: "domain", Position: Position{X: 100, Y: 150}, Data: map[string]any{
					"name":       "primary",
					"fqdn":       "myapp.example.com",
					"sslEnabled": true,
				}},
			},
			"edges": []TopologyEdge{
				{ID: "edge-1", Source: "app-1", Target: "db-1", Type: "dependency", Animated: true},
				{ID: "edge-2", Source: "app-1", Target: "vol-1", Type: "mount", Animated: false},
				{ID: "edge-3", Source: "domain-1", Target: "app-1", Type: "route", Animated: true},
			},
		},
		{
			"id":          "microservices",
			"name":        "Microservices Stack",
			"description": "API Gateway, multiple services, Redis cache, and PostgreSQL",
			"nodes": []TopologyNode{
				{ID: "gateway", Type: "app", Position: Position{X: 250, Y: 100}, Data: map[string]any{
					"name":     "gateway",
					"gitUrl":   "https://github.com/user/gateway",
					"port":     8080,
					"replicas": 2,
				}},
				{ID: "users-svc", Type: "app", Position: Position{X: 500, Y: 50}, Data: map[string]any{
					"name":         "users-service",
					"gitUrl":       "https://github.com/user/users-service",
					"port":         3001,
					"internalOnly": true,
				}},
				{ID: "orders-svc", Type: "app", Position: Position{X: 500, Y: 150}, Data: map[string]any{
					"name":         "orders-service",
					"gitUrl":       "https://github.com/user/orders-service",
					"port":         3002,
					"internalOnly": true,
				}},
				{ID: "postgres", Type: "database", Position: Position{X: 750, Y: 50}, Data: map[string]any{
					"name":    "postgres",
					"engine":  "postgres",
					"version": "16",
					"sizeGB":  20,
				}},
				{ID: "redis", Type: "database", Position: Position{X: 750, Y: 150}, Data: map[string]any{
					"name":    "redis",
					"engine":  "redis",
					"version": "7",
					"sizeGB":  2,
				}},
			},
			"edges": []TopologyEdge{
				{ID: "e1", Source: "gateway", Target: "users-svc", Animated: true},
				{ID: "e2", Source: "gateway", Target: "orders-svc", Animated: true},
				{ID: "e3", Source: "users-svc", Target: "postgres", Animated: true},
				{ID: "e4", Source: "orders-svc", Target: "postgres", Animated: true},
				{ID: "e5", Source: "gateway", Target: "redis", Animated: true},
			},
		},
	}

	writeJSON(w, http.StatusOK, templates)
}

// convertNodesToTopology converts UI nodes to topology components
func (h *TopologyHandler) convertNodesToTopology(req TopologyDeployRequest) (*topology.Topology, []string) {
	var errors []string

	t := &topology.Topology{
		ID:          fmt.Sprintf("topo-%d", time.Now().Unix()),
		ProjectID:   req.ProjectID,
		Environment: req.Environment,
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Set defaults
	if t.Environment == "" {
		t.Environment = "production"
	}
	if t.ProjectID == "" {
		t.ProjectID = "default"
	}

	// Convert nodes by type
	for _, node := range req.Nodes {
		switch node.Type {
		case "app":
			app := h.convertToApp(node)
			t.Apps = append(t.Apps, app)

		case "database":
			db := h.convertToDatabase(node)
			t.Databases = append(t.Databases, db)

		case "domain":
			domain := h.convertToDomain(node)
			t.Domains = append(t.Domains, domain)

		case "volume":
			vol := h.convertToVolume(node)
			t.Volumes = append(t.Volumes, vol)

		case "worker":
			worker := h.convertToWorker(node)
			t.Workers = append(t.Workers, worker)
		}
	}

	// Convert edges to connections
	for _, edge := range req.Edges {
		conn := topology.Connection{
			ID:       edge.ID,
			SourceID: edge.Source,
			TargetID: edge.Target,
		}

		switch edge.Type {
		case "dependency":
			conn.Type = topology.ConnDependency
		case "mount":
			conn.Type = topology.ConnMount
		case "route", "dns":
			conn.Type = topology.ConnRoute
			// For route/dns edges, set the domain's TargetAppID
			// Source is domain, Target is app
			for i := range t.Domains {
				if t.Domains[i].ID == edge.Source {
					t.Domains[i].TargetAppID = edge.Target
					break
				}
			}
		default:
			conn.Type = topology.ConnNetwork
		}

		// Extract config from edge data
		if edge.Data != nil {
			if mp, ok := edge.Data["mountPath"].(string); ok {
				conn.Config.MountPath = mp
			}
			if ev, ok := edge.Data["envVarName"].(string); ok {
				conn.Config.EnvVarName = ev
			}
		}

		t.Connections = append(t.Connections, conn)
	}

	return t, errors
}

// Node conversion helpers

func (h *TopologyHandler) convertToApp(node TopologyNode) topology.App {
	return topology.App{
		ID:              node.ID,
		Name:            getStringFromMap(node.Data, "name", node.ID),
		GitURL:          getStringFromMap(node.Data, "gitUrl", ""),
		Branch:          getStringFromMap(node.Data, "branch", "main"),
		BuildPack:       getStringFromMap(node.Data, "buildPack", "auto"),
		Port:            getIntFromMap(node.Data, "port", 3000),
		Replicas:        getIntFromMap(node.Data, "replicas", 1),
		MemoryMB:        getIntFromMap(node.Data, "memoryMB", 0),
		CPU:             getIntFromMap(node.Data, "cpu", 0),
		EnvVars:         getStringMapFromMap(node.Data, "envVars"),
		HealthCheckPath: getStringFromMap(node.Data, "healthCheckPath", ""),
		InternalOnly:    getBoolFromMap(node.Data, "internalOnly", false),
	}
}

func (h *TopologyHandler) convertToDatabase(node TopologyNode) topology.Database {
	engine := topology.DatabaseEngine(getStringFromMap(node.Data, "engine", "postgres"))
	return topology.Database{
		ID:       node.ID,
		Name:     getStringFromMap(node.Data, "name", node.ID),
		Engine:   engine,
		Version:  getStringFromMap(node.Data, "version", "16"),
		SizeGB:   getIntFromMap(node.Data, "sizeGB", 10),
		Username: getStringFromMap(node.Data, "username", ""),
		Password: getStringFromMap(node.Data, "password", ""),
		Database: getStringFromMap(node.Data, "database", ""),
		Managed:  getBoolFromMap(node.Data, "managed", false),
		External: getBoolFromMap(node.Data, "external", false),
		ConnURL:  getStringFromMap(node.Data, "connUrl", ""),
	}
}

func (h *TopologyHandler) convertToDomain(node TopologyNode) topology.Domain {
	return topology.Domain{
		ID:          node.ID,
		Name:        getStringFromMap(node.Data, "name", node.ID),
		FQDN:        getStringFromMap(node.Data, "fqdn", ""),
		SSLEnabled:  getBoolFromMap(node.Data, "sslEnabled", true),
		SSLMODE:     topology.SSLMode(getStringFromMap(node.Data, "sslMode", "auto")),
		TargetAppID: getStringFromMap(node.Data, "targetAppId", ""),
		PathPrefix:  getStringFromMap(node.Data, "pathPrefix", ""),
	}
}

func (h *TopologyHandler) convertToVolume(node TopologyNode) topology.Volume {
	return topology.Volume{
		ID:         node.ID,
		Name:       getStringFromMap(node.Data, "name", node.ID),
		SizeGB:     getIntFromMap(node.Data, "sizeGB", 10),
		MountPath:  getStringFromMap(node.Data, "mountPath", "/data"),
		VolumeType: topology.VolumeType(getStringFromMap(node.Data, "volumeType", "local")),
		Temporary:  getBoolFromMap(node.Data, "temporary", false),
	}
}

func (h *TopologyHandler) convertToWorker(node TopologyNode) topology.Worker {
	return topology.Worker{
		ID:        node.ID,
		Name:      getStringFromMap(node.Data, "name", node.ID),
		GitURL:    getStringFromMap(node.Data, "gitUrl", ""),
		Branch:    getStringFromMap(node.Data, "branch", "main"),
		BuildPack: getStringFromMap(node.Data, "buildPack", "auto"),
		Command:   getStringFromMap(node.Data, "command", ""),
		Replicas:  getIntFromMap(node.Data, "replicas", 1),
		Schedule:  getStringFromMap(node.Data, "schedule", ""),
		EnvVars:   getStringMapFromMap(node.Data, "envVars"),
	}
}

// Map helper functions

func getStringFromMap(m map[string]any, key, defaultValue string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return defaultValue
}

func getIntFromMap(m map[string]any, key string, defaultValue int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return defaultValue
}

func getBoolFromMap(m map[string]any, key string, defaultValue bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultValue
}

func getStringMapFromMap(m map[string]any, key string) map[string]string {
	result := make(map[string]string)
	if v, ok := m[key]; ok {
		if sm, ok := v.(map[string]any); ok {
			for k, val := range sm {
				if s, ok := val.(string); ok {
					result[k] = s
				}
			}
		}
	}
	return result
}
