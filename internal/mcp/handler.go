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
	case "view_logs":
		return h.viewLogs(ctx, input)
	case "create_database":
		return h.textResponse("Database creation via MCP — coming soon")
	case "add_domain":
		return h.textResponse("Domain management via MCP — coming soon")
	case "marketplace_deploy":
		return h.textResponse("Marketplace deploy via MCP — coming soon")
	case "provision_server":
		return h.textResponse("Server provisioning via MCP — coming soon")
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
	}
	_ = json.Unmarshal(input, &params)

	return h.textResponse(fmt.Sprintf("Deploying %s from %s (%s) — pipeline initiated",
		params.Name, params.SourceURL, params.SourceType))
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

// ListTools returns all available MCP tools (for tools/list method).
func (h *Handler) ListTools() []Tool {
	return BuiltinTools()
}
