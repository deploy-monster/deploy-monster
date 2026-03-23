package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/database/engines"
)

// DatabaseHandler manages managed database instances.
type DatabaseHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewDatabaseHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *DatabaseHandler {
	return &DatabaseHandler{store: store, runtime: runtime, events: events}
}

type createDBRequest struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
}

// ListEngines handles GET /api/v1/databases/engines
func (h *DatabaseHandler) ListEngines(w http.ResponseWriter, _ *http.Request) {
	result := make([]map[string]any, 0)
	for name, engine := range engines.Registry {
		result = append(result, map[string]any{
			"id":           name,
			"name":         engine.Name(),
			"versions":     engine.Versions(),
			"default_port": engine.DefaultPort(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// Create handles POST /api/v1/databases
func (h *DatabaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createDBRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Engine == "" {
		writeError(w, http.StatusBadRequest, "name and engine are required")
		return
	}

	engine, ok := engines.Get(req.Engine)
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported engine: "+req.Engine)
		return
	}

	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}

	containerID, creds, err := engines.Provision(r.Context(), h.runtime, engine, engines.ProvisionOpts{
		TenantID: claims.TenantID,
		Name:     req.Name,
		Engine:   req.Engine,
		Version:  req.Version,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "provisioning failed: "+err.Error())
		return
	}

	connStr := engine.ConnectionString("localhost", engine.DefaultPort(), creds)

	h.events.Publish(r.Context(), core.NewTenantEvent(
		core.EventDatabaseCreated, "api", claims.TenantID, claims.UserID,
		map[string]string{"engine": req.Engine, "name": req.Name},
	))

	writeJSON(w, http.StatusCreated, map[string]any{
		"container_id":     containerID,
		"engine":           req.Engine,
		"name":             req.Name,
		"port":             engine.DefaultPort(),
		"connection_string": connStr,
		"credentials": map[string]string{
			"database": creds.Database,
			"user":     creds.User,
			"password": creds.Password,
		},
	})
}
