package api

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/api/handlers"
	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
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
	authHandler := handlers.NewAuthHandler(r.authMod, r.store)
	appHandler := handlers.NewAppHandler(r.store, r.core)

	// Health check (no auth)
	r.mux.HandleFunc("GET /health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)

	// Auth routes (no auth)
	r.mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	r.mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	r.mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)

	// Protected routes - wrapped with auth middleware
	protected := middleware.RequireAuth(r.authMod.JWT())

	// Apps
	r.mux.Handle("GET /api/v1/apps", protected(http.HandlerFunc(appHandler.List)))
	r.mux.Handle("POST /api/v1/apps", protected(http.HandlerFunc(appHandler.Create)))
	r.mux.Handle("GET /api/v1/apps/{id}", protected(http.HandlerFunc(appHandler.Get)))
	r.mux.Handle("DELETE /api/v1/apps/{id}", protected(http.HandlerFunc(appHandler.Delete)))
	r.mux.Handle("POST /api/v1/apps/{id}/restart", protected(http.HandlerFunc(appHandler.Restart)))
	r.mux.Handle("POST /api/v1/apps/{id}/stop", protected(http.HandlerFunc(appHandler.Stop)))
	r.mux.Handle("POST /api/v1/apps/{id}/start", protected(http.HandlerFunc(appHandler.Start)))

	// Domains
	domainHandler := handlers.NewDomainHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/domains", protected(http.HandlerFunc(domainHandler.List)))
	r.mux.Handle("POST /api/v1/domains", protected(http.HandlerFunc(domainHandler.Create)))
	r.mux.Handle("DELETE /api/v1/domains/{id}", protected(http.HandlerFunc(domainHandler.Delete)))

	// SPA fallback - serve React UI
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

func (r *Router) handleSPA(w http.ResponseWriter, req *http.Request) {
	// For now, return a simple placeholder
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html><html><head><title>DeployMonster</title></head><body><h1>DeployMonster</h1><p>UI will be embedded here.</p></body></html>`))
}
