package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/mcp"
)

// MCPHandler serves MCP protocol over HTTP.
type MCPHandler struct {
	core    *core.Core
	handler *mcp.Handler
}

func NewMCPHandler(c *core.Core, store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *MCPHandler {
	return &MCPHandler{
		core:    c,
		handler: mcp.NewHandler(store, runtime, events, c.Logger),
	}
}

// ListTools handles GET /mcp/v1/tools
func (h *MCPHandler) ListTools(w http.ResponseWriter, _ *http.Request) {
	tools := h.handler.ListTools()

	// Enrich with module registry info
	modules := h.core.Registry.All()

	writeJSON(w, http.StatusOK, map[string]any{
		"tools":        tools,
		"version":      h.core.Build.Version,
		"modules":      modules,
		"module_count": len(modules),
	})
}

// CallTool handles POST /mcp/v1/tools/{name}
func (h *MCPHandler) CallTool(w http.ResponseWriter, r *http.Request) {
	toolName := r.PathValue("name")

	var input json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		input = []byte("{}")
	}

	result, err := h.handler.HandleToolCall(r.Context(), toolName, input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
