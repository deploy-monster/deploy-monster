package api

import (
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/api/handlers"
	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/api/ws"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/enterprise/integrations"
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
		middleware.AuditLog(r.store, r.core.Logger),
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

	// ── Session / Profile ─────────────────────────────
	sessionH := handlers.NewSessionHandler(r.store)
	r.mux.Handle("GET /api/v1/auth/me", protected(http.HandlerFunc(sessionH.GetCurrentUser)))
	r.mux.Handle("PATCH /api/v1/auth/me", protected(http.HandlerFunc(sessionH.UpdateProfile)))
	r.mux.Handle("POST /api/v1/auth/change-password", protected(http.HandlerFunc(sessionH.ChangePassword)))

	// ── Webhooks (signature-verified, not JWT) ─────────
	webhookRecv := webhooks.NewReceiver(r.store, r.core.Events, r.core.Logger)
	r.mux.HandleFunc("POST /hooks/v1/{webhookID}", webhookRecv.HandleWebhook)

	// ── Apps ────────────────────────────────────────────
	appH := handlers.NewAppHandler(r.store, r.core)
	r.mux.Handle("GET /api/v1/apps", protected(http.HandlerFunc(appH.List)))
	r.mux.Handle("POST /api/v1/apps", protected(http.HandlerFunc(appH.Create)))
	r.mux.Handle("GET /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Get)))
	r.mux.Handle("PATCH /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Update)))
	r.mux.Handle("DELETE /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Delete)))
	r.mux.Handle("POST /api/v1/apps/{id}/restart", protected(http.HandlerFunc(appH.Restart)))
	r.mux.Handle("POST /api/v1/apps/{id}/stop", protected(http.HandlerFunc(appH.Stop)))
	r.mux.Handle("POST /api/v1/apps/{id}/start", protected(http.HandlerFunc(appH.Start)))
	deployTriggerH := handlers.NewDeployTriggerHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/deploy", protected(http.HandlerFunc(deployTriggerH.TriggerDeploy)))

	// ── Rollback & Versions ───────────────────────────
	rollbackH := handlers.NewRollbackHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rollback", protected(http.HandlerFunc(rollbackH.Rollback)))
	r.mux.Handle("GET /api/v1/apps/{id}/versions", protected(http.HandlerFunc(rollbackH.ListVersions)))

	// ── Deployments ────────────────────────────────────
	depH := handlers.NewDeploymentHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/deployments", protected(http.HandlerFunc(depH.ListByApp)))
	r.mux.Handle("GET /api/v1/apps/{id}/deployments/latest", protected(http.HandlerFunc(depH.GetLatest)))

	// ── Stats & Scaling ───────────────────────────────
	statsH := handlers.NewStatsHandler(r.core.Services.Container, r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/stats", protected(http.HandlerFunc(statsH.AppStats)))
	r.mux.Handle("GET /api/v1/servers/stats", protected(http.HandlerFunc(statsH.ServerStats)))
	scaleH := handlers.NewScaleHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/scale", protected(http.HandlerFunc(scaleH.Scale)))

	// ── Logs ──────────────────────────────────────────
	logH := handlers.NewLogHandler(r.core.Services.Container, r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/logs", protected(http.HandlerFunc(logH.GetLogs)))

	// ── Volumes ───────────────────────────────────────
	volH := handlers.NewVolumeHandler(r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /api/v1/volumes", protected(http.HandlerFunc(volH.List)))
	r.mux.Handle("POST /api/v1/volumes", protected(http.HandlerFunc(volH.Create)))

	// ── Projects ───────────────────────────────────────
	projH := handlers.NewProjectHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/projects", protected(http.HandlerFunc(projH.List)))
	r.mux.Handle("POST /api/v1/projects", protected(http.HandlerFunc(projH.Create)))
	r.mux.Handle("GET /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Get)))
	r.mux.Handle("DELETE /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Delete)))

	// ── Env Vars ──────────────────────────────────────
	envH := handlers.NewEnvVarHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/env", protected(http.HandlerFunc(envH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/env", protected(http.HandlerFunc(envH.Update)))

	// ── Domains ────────────────────────────────────────
	domH := handlers.NewDomainHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/domains", protected(http.HandlerFunc(domH.List)))
	r.mux.Handle("POST /api/v1/domains", protected(http.HandlerFunc(domH.Create)))
	r.mux.Handle("DELETE /api/v1/domains/{id}", protected(http.HandlerFunc(domH.Delete)))

	// ── Container Exec ────────────────────────────────
	execH := handlers.NewExecHandler(r.core.Services.Container)
	r.mux.Handle("POST /api/v1/apps/{id}/exec", protected(http.HandlerFunc(execH.Exec)))

	// ── Team ───────────────────────────────────────────
	teamH := handlers.NewTeamHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/team/roles", protected(http.HandlerFunc(teamH.ListRoles)))
	inviteH := handlers.NewInviteHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/team/invites", protected(http.HandlerFunc(inviteH.Create)))
	r.mux.Handle("GET /api/v1/team/audit-log", protected(http.HandlerFunc(teamH.GetAuditLog)))

	// ── Databases ─────────────────────────────────────
	dbH := handlers.NewDatabaseHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.HandleFunc("GET /api/v1/databases/engines", dbH.ListEngines)
	r.mux.Handle("POST /api/v1/databases", protected(http.HandlerFunc(dbH.Create)))

	// ── Backups ───────────────────────────────────────
	backupStorage := r.core.Services.BackupStorage("local")
	backupH := handlers.NewBackupHandler(r.store, backupStorage, r.core.Events)
	r.mux.Handle("GET /api/v1/backups", protected(http.HandlerFunc(backupH.List)))
	r.mux.Handle("POST /api/v1/backups", protected(http.HandlerFunc(backupH.Create)))
	r.mux.Handle("GET /api/v1/backups/{key}/download", protected(http.HandlerFunc(backupH.Download)))

	// ── Servers / VPS ─────────────────────────────────
	serverH := handlers.NewServerHandler(r.store, r.core.Services, r.core.Events)
	r.mux.Handle("GET /api/v1/servers/providers", protected(http.HandlerFunc(serverH.ListProviders)))
	r.mux.Handle("GET /api/v1/servers/providers/{provider}/regions", protected(http.HandlerFunc(serverH.ListRegions)))
	r.mux.Handle("GET /api/v1/servers/providers/{provider}/sizes", protected(http.HandlerFunc(serverH.ListSizes)))
	r.mux.Handle("POST /api/v1/servers/provision", protected(http.HandlerFunc(serverH.Provision)))

	// ── Git Sources ───────────────────────────────────
	gitH := handlers.NewGitSourceHandler(r.core.Services)
	r.mux.Handle("GET /api/v1/git/providers", protected(http.HandlerFunc(gitH.ListProviders)))
	r.mux.Handle("GET /api/v1/git/{provider}/repos", protected(http.HandlerFunc(gitH.ListRepos)))
	r.mux.Handle("GET /api/v1/git/{provider}/repos/{repo}/branches", protected(http.HandlerFunc(gitH.ListBranches)))

	// ── Compose Stacks ────────────────────────────────
	composeH := handlers.NewComposeHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/stacks", protected(http.HandlerFunc(composeH.Deploy)))
	r.mux.Handle("POST /api/v1/stacks/validate", protected(http.HandlerFunc(composeH.Validate)))

	// ── Secrets ───────────────────────────────────────
	var vault interface{ Encrypt(string) (string, error); Decrypt(string) (string, error) }
	secretsMod := r.core.Registry.Get("secrets")
	if secretsMod != nil {
		type vaultProvider interface{ Vault() interface{ Encrypt(string) (string, error); Decrypt(string) (string, error) } }
		if vp, ok := secretsMod.(vaultProvider); ok {
			vault = vp.Vault()
		}
	}
	secretH := handlers.NewSecretHandler(r.store, vault, r.core.Events)
	r.mux.Handle("GET /api/v1/secrets", protected(http.HandlerFunc(secretH.List)))
	r.mux.Handle("POST /api/v1/secrets", protected(http.HandlerFunc(secretH.Create)))

	// ── Billing ───────────────────────────────────────
	billingH := handlers.NewBillingHandler(r.store)
	r.mux.HandleFunc("GET /api/v1/billing/plans", billingH.ListPlans)
	r.mux.Handle("GET /api/v1/billing/usage", protected(http.HandlerFunc(billingH.GetUsage)))

	// ── Marketplace (public list, auth for deploy) ────
	mpMod := r.core.Registry.Get("marketplace")
	if mpMod != nil {
		reg := mpMod.(*marketplace.Module).Registry()
		mpH := handlers.NewMarketplaceHandler(reg)
		r.mux.HandleFunc("GET /api/v1/marketplace", mpH.List)
		r.mux.HandleFunc("GET /api/v1/marketplace/{slug}", mpH.Get)
		mpDeployH := handlers.NewMarketplaceDeployHandler(reg, r.core.Services.Container, r.store, r.core.Events)
		r.mux.Handle("POST /api/v1/marketplace/deploy", protected(http.HandlerFunc(mpDeployH.Deploy)))
	}

	// ── Notifications ─────────────────────────────────
	notifH := handlers.NewNotificationHandler(r.core.Services.Notifications)
	r.mux.Handle("POST /api/v1/notifications/test", protected(http.HandlerFunc(notifH.Test)))

	// ── Terminal ──────────────────────────────────────
	termH := ws.NewTerminal(r.core.Services.Container, r.core.Logger)
	r.mux.Handle("GET /api/v1/apps/{id}/terminal", protected(http.HandlerFunc(termH.StreamOutput)))
	r.mux.Handle("POST /api/v1/apps/{id}/terminal", protected(http.HandlerFunc(termH.SendCommand)))

	// ── MCP Protocol ──────────────────────────────────
	mcpH := handlers.NewMCPHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.HandleFunc("GET /mcp/v1/tools", mcpH.ListTools)
	r.mux.HandleFunc("POST /mcp/v1/tools/{name}", mcpH.CallTool)

	// ── Streaming (SSE) ────────────────────────────────
	logStreamer := ws.NewLogStreamer(r.core.Services.Container, r.core.Logger)
	eventStreamer := ws.NewEventStreamer(r.core.Events, r.core.Logger)
	r.mux.Handle("GET /api/v1/apps/{id}/logs/stream", protected(http.HandlerFunc(logStreamer.StreamLogs)))
	r.mux.Handle("GET /api/v1/events/stream", protected(http.HandlerFunc(eventStreamer.StreamEvents)))

	// ── Admin (super admin only) ──────────────────────
	adminH := handlers.NewAdminHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/system", protected(http.HandlerFunc(adminH.SystemInfo)))
	r.mux.Handle("PATCH /api/v1/admin/settings", protected(http.HandlerFunc(adminH.UpdateSettings)))
	r.mux.Handle("GET /api/v1/admin/tenants", protected(http.HandlerFunc(adminH.ListTenants)))

	// ── Branding (public GET, admin PATCH) ────────────
	brandingH := handlers.NewBrandingHandler()
	r.mux.HandleFunc("GET /api/v1/branding", brandingH.Get)
	r.mux.Handle("PATCH /api/v1/admin/branding", protected(http.HandlerFunc(brandingH.Update)))

	// ── Prometheus metrics (no auth — internal) ───────
	promExporter := integrations.NewPrometheusExporter(r.core.Registry, r.core.Events, r.core.Services)
	r.mux.HandleFunc("GET /metrics", promExporter.Handler())

	// ── SPA fallback — embedded React UI ──────────────
	r.mux.Handle("/", newSPAHandler())
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

