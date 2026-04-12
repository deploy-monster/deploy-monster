package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Handler processes MCP protocol messages.
// Implements the Model Context Protocol for AI-driven infrastructure management.
type Handler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
	logger  *slog.Logger
}

// NewHandler creates a new MCP protocol handler.
func NewHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus, logger *slog.Logger) *Handler {
	return &Handler{store: store, runtime: runtime, events: events, logger: logger}
}

// MCPRequest is the incoming MCP tool call.
type MCPRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// MCPResponse is the outgoing MCP result.
type MCPResponse struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a piece of MCP response content.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// HandleToolCall dispatches an MCP tool call to the appropriate handler.
func (h *Handler) HandleToolCall(ctx context.Context, toolName string, input json.RawMessage) (*MCPResponse, error) {
	h.logger.Info("MCP tool call", "tool", toolName)

	switch toolName {
	case "list_apps":
		return h.listApps(ctx)
	case "get_app_status":
		return h.getAppStatus(ctx, input)
	case "deploy_app":
		return h.deployApp(ctx, input)
	case "scale_app":
		return h.scaleApp(ctx, input)
	case "view_logs":
		return h.viewLogs(ctx, input)
	case "create_database":
		return h.createDatabase(ctx, input)
	case "add_domain":
		return h.addDomain(ctx, input)
	case "marketplace_deploy":
		return h.marketplaceDeploy(ctx, input)
	case "provision_server":
		return h.provisionServer(ctx, input)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (h *Handler) listApps(ctx context.Context) (*MCPResponse, error) {
	// Use a system tenant for MCP (super admin context)
	apps, total, err := h.store.ListAppsByTenant(ctx, "", 50, 0)
	if err != nil {
		return h.errorResponse(err.Error())
	}

	data, _ := json.MarshalIndent(map[string]any{
		"apps":  apps,
		"total": total,
	}, "", "  ")

	return h.textResponse(string(data))
}

func (h *Handler) getAppStatus(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		AppID string `json:"app_id"`
	}
	_ = json.Unmarshal(input, &params)

	app, err := h.store.GetApp(ctx, params.AppID)
	if err != nil {
		return h.errorResponse("App not found: " + params.AppID)
	}

	data, _ := json.MarshalIndent(app, "", "  ")
	return h.textResponse(string(data))
}

func (h *Handler) deployApp(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		Name       string `json:"name"`
		SourceType string `json:"source_type"`
		SourceURL  string `json:"source_url"`
		TenantID   string `json:"tenant_id"`
		ProjectID  string `json:"project_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return h.errorResponse("invalid parameters: " + err.Error())
	}

	if params.Name == "" || params.SourceType == "" || params.SourceURL == "" {
		return h.errorResponse("name, source_type, and source_url are required")
	}

	// Determine tenant
	tenantID := params.TenantID
	if tenantID == "" {
		// Use first available tenant (super-admin fallback)
		if tenants, _, err := h.store.ListAllTenants(ctx, 1, 0); err == nil && len(tenants) > 0 {
			tenantID = tenants[0].ID
		}
	}
	if tenantID == "" {
		return h.errorResponse("no tenant available; provide tenant_id")
	}

	app := &core.Application{
		ID:         core.GenerateID(),
		TenantID:   tenantID,
		ProjectID:  params.ProjectID,
		Name:       params.Name,
		SourceType: params.SourceType,
		SourceURL:  params.SourceURL,
		Type:       "app",
		Status:     "pending",
	}
	if err := h.store.CreateApp(ctx, app); err != nil {
		return h.errorResponse("failed to create app: " + err.Error())
	}

	_ = h.events.Publish(ctx, core.NewEvent(core.EventAppCreated, "mcp", map[string]string{
		"app_id":    app.ID,
		"tenant_id": tenantID,
	}))

	result := map[string]any{
		"app_id":      app.ID,
		"name":        app.Name,
		"source_type": app.SourceType,
		"source_url":  app.SourceURL,
		"tenant_id":   tenantID,
		"status":      "created",
		"message":     "Deployment pipeline initiated",
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return h.textResponse("Deploying app:\n" + string(data))
}

// scaleApp updates the replica count for an application and emits
// app.scaled so the deploy module can reconcile the running set.
func (h *Handler) scaleApp(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		AppID    string `json:"app_id"`
		Replicas int    `json:"replicas"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return h.errorResponse("invalid parameters: " + err.Error())
	}
	if params.AppID == "" {
		return h.errorResponse("app_id is required")
	}
	if params.Replicas < 0 {
		return h.errorResponse("replicas must be >= 0")
	}

	app, err := h.store.GetApp(ctx, params.AppID)
	if err != nil {
		return h.errorResponse("App not found: " + params.AppID)
	}

	previous := app.Replicas
	app.Replicas = params.Replicas
	if err := h.store.UpdateApp(ctx, app); err != nil {
		return h.errorResponse("failed to update app: " + err.Error())
	}

	_ = h.events.Publish(ctx, core.NewEvent(core.EventAppScaled, "mcp", map[string]any{
		"app_id":       app.ID,
		"tenant_id":    app.TenantID,
		"old_replicas": previous,
		"new_replicas": params.Replicas,
	}))

	result := map[string]any{
		"app_id":       app.ID,
		"name":         app.Name,
		"old_replicas": previous,
		"new_replicas": params.Replicas,
		"status":       "scaled",
		"message":      "Scale event emitted — deploy module will reconcile replicas",
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return h.textResponse(string(data))
}

func (h *Handler) viewLogs(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		AppID string `json:"app_id"`
		Lines int    `json:"lines"`
	}
	_ = json.Unmarshal(input, &params)

	if h.runtime == nil {
		return h.errorResponse("Container runtime not available")
	}

	containers, err := h.runtime.ListByLabels(ctx, map[string]string{
		"monster.app.id": params.AppID,
	})
	if err != nil || len(containers) == 0 {
		return h.errorResponse("No running container for app " + params.AppID)
	}

	lines := params.Lines
	if lines <= 0 {
		lines = 50
	}

	reader, err := h.runtime.Logs(ctx, containers[0].ID, fmt.Sprintf("%d", lines), false)
	if err != nil {
		return h.errorResponse("Failed to get logs: " + err.Error())
	}
	defer reader.Close()

	buf := make([]byte, 64*1024)
	n, _ := reader.Read(buf)
	return h.textResponse(string(buf[:n]))
}

func (h *Handler) textResponse(text string) (*MCPResponse, error) {
	return &MCPResponse{
		Content: []ContentBlock{{Type: "text", Text: text}},
	}, nil
}

func (h *Handler) errorResponse(msg string) (*MCPResponse, error) {
	return &MCPResponse{
		Content: []ContentBlock{{Type: "text", Text: "Error: " + msg}},
		IsError: true,
	}, nil
}

// createDatabase creates a new database configuration for an application.
func (h *Handler) createDatabase(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		AppID    string `json:"app_id"`
		Engine   string `json:"engine"` // mysql, postgres, redis, mongodb
		Name     string `json:"name"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return h.errorResponse("invalid parameters: " + err.Error())
	}

	if params.Engine == "" || params.Name == "" {
		return h.errorResponse("engine and name are required")
	}

	if params.AppID != "" {
		if _, err := h.store.GetApp(ctx, params.AppID); err != nil {
			return h.errorResponse("App not found: " + params.AppID)
		}
	}

	if params.User == "" {
		params.User = "dbuser"
	}
	if params.Password == "" {
		params.Password = core.GenerateID()[:16]
	}

	var connStr string
	switch params.Engine {
	case "mysql":
		connStr = fmt.Sprintf("mysql://%s:%s@localhost:3306/%s", params.User, params.Password, params.Name)
	case "postgres":
		connStr = fmt.Sprintf("postgres://%s:%s@localhost:5432/%s", params.User, params.Password, params.Name)
	case "redis":
		connStr = fmt.Sprintf("redis://localhost:6379/%s", params.Name)
	case "mongodb":
		connStr = fmt.Sprintf("mongodb://%s:%s@localhost:27017/%s", params.User, params.Password, params.Name)
	default:
		return h.errorResponse("Unsupported engine: " + params.Engine)
	}

	dbID := core.GenerateID()
	_ = h.events.Publish(ctx, core.NewEvent(core.EventDatabaseCreated, "mcp", map[string]string{
		"db_id":  dbID,
		"app_id": params.AppID,
		"engine": params.Engine,
	}))

	result := map[string]any{
		"id":         dbID,
		"name":       params.Name,
		"engine":     params.Engine,
		"user":       params.User,
		"password":   params.Password,
		"connection": connStr,
		"app_id":     params.AppID,
		"status":     "created",
		"message":    "Database configuration created and event published",
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return h.textResponse("Database created successfully:\n" + string(data))
}

// addDomain adds a domain to an application.
func (h *Handler) addDomain(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		AppID       string `json:"app_id"`
		FQDN        string `json:"fqdn"`
		Type        string `json:"type"`         // primary, custom, redirect
		DNSProvider string `json:"dns_provider"` // cloudflare, manual
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return h.errorResponse("Invalid parameters: " + err.Error())
	}

	// Validate required fields
	if params.AppID == "" || params.FQDN == "" {
		return h.errorResponse("app_id and fqdn are required")
	}

	// Verify app exists
	app, err := h.store.GetApp(ctx, params.AppID)
	if err != nil {
		return h.errorResponse("App not found: " + params.AppID)
	}

	// Set defaults
	if params.Type == "" {
		params.Type = "custom"
	}
	if params.DNSProvider == "" {
		params.DNSProvider = "manual"
	}

	// Create domain record
	domainID := core.GenerateID()
	domain := &core.Domain{
		ID:          domainID,
		AppID:       params.AppID,
		FQDN:        params.FQDN,
		Type:        params.Type,
		DNSProvider: params.DNSProvider,
		DNSSynced:   false,
		Verified:    false,
	}

	if err := h.store.CreateDomain(ctx, domain); err != nil {
		return h.errorResponse("Failed to create domain: " + err.Error())
	}

	result := map[string]any{
		"id":           domainID,
		"fqdn":         params.FQDN,
		"type":         params.Type,
		"dns_provider": params.DNSProvider,
		"app_id":       params.AppID,
		"app_name":     app.Name,
		"status":       "created",
		"message":      "Domain added. Add DNS A record pointing to your server IP, then verify the domain.",
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return h.textResponse("Domain added successfully:\n" + string(data))
}

// marketplaceDeploy deploys an application from the marketplace.
func (h *Handler) marketplaceDeploy(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		TemplateSlug string `json:"template_slug"`
		Name         string `json:"name"`
		ProjectID    string `json:"project_id"`
		Domain       string `json:"domain"`
		TenantID     string `json:"tenant_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return h.errorResponse("invalid parameters: " + err.Error())
	}

	if params.TemplateSlug == "" || params.Name == "" {
		return h.errorResponse("template_slug and name are required")
	}

	tenantID := params.TenantID
	if tenantID == "" {
		if tenants, _, err := h.store.ListAllTenants(ctx, 1, 0); err == nil && len(tenants) > 0 {
			tenantID = tenants[0].ID
		}
	}
	if tenantID == "" {
		return h.errorResponse("no tenant available; provide tenant_id")
	}

	app := &core.Application{
		ID:         core.GenerateID(),
		TenantID:   tenantID,
		ProjectID:  params.ProjectID,
		Name:       params.Name,
		SourceType: "image",
		SourceURL:  fmt.Sprintf("deploymonster/%s:latest", params.TemplateSlug),
		Type:       "app",
		Status:     "pending",
	}
	if err := h.store.CreateApp(ctx, app); err != nil {
		return h.errorResponse("failed to create app: " + err.Error())
	}

	if params.Domain != "" {
		domain := &core.Domain{
			ID:    core.GenerateID(),
			AppID: app.ID,
			FQDN:  params.Domain,
			Type:  "custom",
		}
		if err := h.store.CreateDomain(ctx, domain); err != nil {
			h.logger.Warn("failed to create domain for marketplace deploy", "error", err)
		}
	}

	_ = h.events.Publish(ctx, core.NewEvent(core.EventAppCreated, "mcp", map[string]string{
		"app_id":    app.ID,
		"tenant_id": tenantID,
	}))

	result := map[string]any{
		"app_id":        app.ID,
		"template_slug": params.TemplateSlug,
		"name":          params.Name,
		"project_id":    params.ProjectID,
		"domain":        params.Domain,
		"status":        "created",
		"message":       "Marketplace app created and deployment initiated",
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return h.textResponse("Marketplace deployment started:\n" + string(data))
}

// provisionServer provisions a new VPS server.
func (h *Handler) provisionServer(ctx context.Context, input json.RawMessage) (*MCPResponse, error) {
	var params struct {
		Provider string `json:"provider"` // hetzner, digitalocean, vultr, linode
		Region   string `json:"region"`
		Size     string `json:"size"`
		Name     string `json:"name"`
		SSHKey   string `json:"ssh_key_id"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return h.errorResponse("Invalid parameters: " + err.Error())
	}

	// Validate required fields
	if params.Provider == "" || params.Name == "" {
		return h.errorResponse("provider and name are required")
	}

	// Set defaults
	if params.Region == "" {
		params.Region = "auto"
	}
	if params.Size == "" {
		params.Size = "small"
	}

	// Validate provider
	validProviders := map[string]bool{
		"hetzner":      true,
		"digitalocean": true,
		"vultr":        true,
		"linode":       true,
		"custom":       true,
	}
	if !validProviders[params.Provider] {
		return h.errorResponse("Unsupported provider: " + params.Provider + ". Use: hetzner, digitalocean, vultr, linode, or custom")
	}

	serverID := core.GenerateID()

	_ = h.events.Publish(ctx, core.NewEvent(core.EventServerAdded, "mcp", map[string]string{
		"server_id": serverID,
		"provider":  params.Provider,
		"name":      params.Name,
	}))

	result := map[string]any{
		"id":         serverID,
		"name":       params.Name,
		"provider":   params.Provider,
		"region":     params.Region,
		"size":       params.Size,
		"ssh_key_id": params.SSHKey,
		"status":     "pending",
		"message":    "Server provisioning request accepted. Configure VPS provider API keys to provision automatically.",
		"api_docs":   "POST /api/v1/servers to create a server record, then use the provider's dashboard for actual provisioning.",
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return h.textResponse("Server provisioning initiated:\n" + string(data))
}

// ListTools returns all available MCP tools (for tools/list method).
func (h *Handler) ListTools() []Tool {
	return BuiltinTools()
}
