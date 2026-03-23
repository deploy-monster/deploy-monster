package api

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/api/handlers"
	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/api/ws"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
	"github.com/deploy-monster/deploy-monster/internal/webhooks"
)

// Router sets up all HTTP routes for the API.
type Router struct {
	mux     *http.ServeMux
	core    *core.Core
	authMod *auth.Module
	store   core.Store
}

// NewRouter creates a new API router with all routes registered.
func NewRouter(c *core.Core, authMod *auth.Module, store core.Store) *Router {
	r := &Router{
		mux:     http.NewServeMux(),
		core:    c,
		authMod: authMod,
		store:   store,
	}
	r.registerRoutes()
	return r
}

// Handler returns the root HTTP handler with global middleware applied.
func (r *Router) Handler() http.Handler {
	return middleware.Chain(
		r.mux,
		middleware.Recovery(r.core.Logger),
		middleware.RequestLogger(r.core.Logger),
		middleware.CORS("*"),
	)
}

func (r *Router) registerRoutes() {
	protected := middleware.RequireAuth(r.authMod.JWT())

	// ── Health ──────────────────────────────────────────
	r.mux.HandleFunc("GET /health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)

	// ── Auth (public) ──────────────────────────────────
	authH := handlers.NewAuthHandler(r.authMod, r.store)
	r.mux.HandleFunc("POST /api/v1/auth/login", authH.Login)
	r.mux.HandleFunc("POST /api/v1/auth/register", authH.Register)
	r.mux.HandleFunc("POST /api/v1/auth/refresh", authH.Refresh)

	// ── Webhooks (signature-verified, not JWT) ─────────
	webhookRecv := webhooks.NewReceiver(r.store, r.core.Events, r.core.Logger)
	r.mux.HandleFunc("POST /hooks/v1/{webhookID}", webhookRecv.HandleWebhook)

	// ── Apps ────────────────────────────────────────────
	appH := handlers.NewAppHandler(r.store, r.core)
	r.mux.Handle("GET /api/v1/apps", protected(http.HandlerFunc(appH.List)))
	r.mux.Handle("POST /api/v1/apps", protected(http.HandlerFunc(appH.Create)))
	r.mux.Handle("GET /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Get)))
	r.mux.Handle("DELETE /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Delete)))
	r.mux.Handle("POST /api/v1/apps/{id}/restart", protected(http.HandlerFunc(appH.Restart)))
	r.mux.Handle("POST /api/v1/apps/{id}/stop", protected(http.HandlerFunc(appH.Stop)))
	r.mux.Handle("POST /api/v1/apps/{id}/start", protected(http.HandlerFunc(appH.Start)))

	// ── Deployments ────────────────────────────────────
	depH := handlers.NewDeploymentHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/deployments", protected(http.HandlerFunc(depH.ListByApp)))
	r.mux.Handle("GET /api/v1/apps/{id}/deployments/latest", protected(http.HandlerFunc(depH.GetLatest)))

	// ── Projects ───────────────────────────────────────
	projH := handlers.NewProjectHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/projects", protected(http.HandlerFunc(projH.List)))
	r.mux.Handle("POST /api/v1/projects", protected(http.HandlerFunc(projH.Create)))
	r.mux.Handle("GET /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Get)))
	r.mux.Handle("DELETE /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Delete)))

	// ── Domains ────────────────────────────────────────
	domH := handlers.NewDomainHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/domains", protected(http.HandlerFunc(domH.List)))
	r.mux.Handle("POST /api/v1/domains", protected(http.HandlerFunc(domH.Create)))
	r.mux.Handle("DELETE /api/v1/domains/{id}", protected(http.HandlerFunc(domH.Delete)))

	// ── Team ───────────────────────────────────────────
	teamH := handlers.NewTeamHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/team/roles", protected(http.HandlerFunc(teamH.ListRoles)))
	r.mux.Handle("GET /api/v1/team/audit-log", protected(http.HandlerFunc(teamH.GetAuditLog)))

	// ── Marketplace (public list, auth for deploy) ────
	mpMod := r.core.Registry.Get("marketplace")
	if mpMod != nil {
		mpH := handlers.NewMarketplaceHandler(mpMod.(*marketplace.Module).Registry())
		r.mux.HandleFunc("GET /api/v1/marketplace", mpH.List)
		r.mux.HandleFunc("GET /api/v1/marketplace/{slug}", mpH.Get)
	}

	// ── Streaming (SSE) ────────────────────────────────
	logStreamer := ws.NewLogStreamer(r.core.Services.Container, r.core.Logger)
	eventStreamer := ws.NewEventStreamer(r.core.Events, r.core.Logger)
	r.mux.Handle("GET /api/v1/apps/{id}/logs/stream", protected(http.HandlerFunc(logStreamer.StreamLogs)))
	r.mux.Handle("GET /api/v1/events/stream", protected(http.HandlerFunc(eventStreamer.StreamEvents)))

	// ── SPA fallback ───────────────────────────────────
	r.mux.HandleFunc("/", r.handleSPA)
}

func (r *Router) handleHealth(w http.ResponseWriter, _ *http.Request) {
	health := r.core.Registry.HealthAll()
	status := "ok"
	httpStatus := http.StatusOK

	modules := make(map[string]string, len(health))
	for id, h := range health {
		modules[id] = h.String()
		if h == core.HealthDown {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	writeJSON(w, httpStatus, map[string]any{
		"status":  status,
		"version": r.core.Build.Version,
		"modules": modules,
	})
}

func (r *Router) handleSPA(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html><html><head><title>DeployMonster</title></head><body><h1>DeployMonster</h1><p>UI will be embedded here.</p></body></html>`))
}
