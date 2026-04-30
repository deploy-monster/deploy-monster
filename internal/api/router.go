package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/handlers"
	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/api/ws"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/billing"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/enterprise/integrations"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
	"github.com/deploy-monster/deploy-monster/internal/webhooks"
)

const (
	maxBodySize        = 10 << 20 // 10MB global request body limit
	maxWebhookBody     = 1 << 20  // 1MB for inbound webhook payloads
	requestTimeout     = 30 * time.Second
	apiKeyCleanupEvery = 1 * time.Hour
)

// Router sets up all HTTP routes for the API.
type Router struct {
	mux              *http.ServeMux
	core             *core.Core
	authMod          *auth.Module
	store            core.Store
	apiMetrics       *middleware.APIMetrics
	gracefulShutdown *middleware.GracefulShutdown
	globalRL         *middleware.GlobalRateLimiter
	serverCtx        context.Context    // canceled on graceful shutdown
	serverCancel     context.CancelFunc // called by Stop to signal goroutines
	startedAt        time.Time          // server start time for uptime reporting
}

// NewRouter creates a new API router with all routes registered.
func NewRouter(c *core.Core, authMod *auth.Module, store core.Store) *Router {
	ctx, cancel := context.WithCancel(context.Background())
	// Global rate limit: default 120 req/min per IP if not configured
	rlRate := c.Config.Server.RateLimitPerMinute
	if rlRate == 0 {
		rlRate = 120
	}

	r := &Router{
		mux:              http.NewServeMux(),
		core:             c,
		authMod:          authMod,
		store:            store,
		apiMetrics:       middleware.NewAPIMetrics(),
		gracefulShutdown: middleware.NewGracefulShutdown(),
		globalRL:         middleware.NewGlobalRateLimiter(rlRate, time.Minute),
		serverCtx:        ctx,
		serverCancel:     cancel,
		startedAt:        time.Now(),
	}
	// Only rate-limit API and webhook traffic. The embedded React SPA's
	// static asset serving (/, /assets/*, /login, /register, /apps, …)
	// must not share the per-IP budget with real API calls, otherwise a
	// single browser session trivially exhausts the limit loading the
	// bundle and the user gets served a JSON "rate_limited" page in
	// place of the app. See Tier 102 for the E2E red that exposed it.
	r.globalRL.SetRateLimitedPrefixes([]string{"/api/", "/hooks/"})
	r.apiMetrics.SubscribeEvents(c.Events)
	r.registerRoutes()
	return r
}

// Handler returns the root HTTP handler with global middleware applied.
func (r *Router) Handler() http.Handler {
	return middleware.Chain(
		r.mux,
		middleware.RequestID,
		r.gracefulShutdown.Middleware,
		r.globalRL.Middleware,
		middleware.SecurityHeaders,
		r.apiMetrics.Middleware,
		middleware.APIVersion(r.core.Build.Version),
		middleware.BodyLimit(maxBodySize),
		middleware.Timeout(requestTimeout),
		middleware.Recovery(r.core.Logger),
		middleware.RequestLogger(r.core.Logger),
		middleware.CORS(r.core.Config.Server.CORSOrigins, r.core.Config.Ingress.EnableHTTPS),
		middleware.CSRFProtect,
		middleware.IdempotencyMiddleware(r.core.DB.Bolt),
		middleware.AuditLog(r.store, r.core.Logger),
	)
}

func (r *Router) registerRoutes() {
	authMiddleware := middleware.RequireAuth(r.authMod.JWT(), r.core.DB.Bolt, r.store)
	tenantRL := middleware.NewTenantRateLimiter(r.core.DB.Bolt, 100, time.Minute)
	// protected applies auth then per-tenant rate limiting
	protected := func(next http.Handler) http.Handler {
		return authMiddleware(tenantRL.Middleware(next))
	}
	// adminOnly stacks RequireSuperAdmin on top of protected. Every
	// /api/v1/admin/* route and any cross-tenant action (e.g. app transfer)
	// must be wrapped with this — never with `protected` alone. A router
	// test in router_test.go walks the whole tree with a developer token
	// and asserts 403 on every admin route, so a forgotten wrap fails CI.
	adminOnly := func(next http.Handler) http.Handler {
		return protected(middleware.RequireSuperAdmin(next))
	}
	// protectedPerm wraps a handler with auth, rate limiting, and an RBAC
	// permission check. Use on all destructive or state-mutating endpoints.
	protectedPerm := func(perm string, next http.HandlerFunc) http.Handler {
		return protected(middleware.RequirePermission(r.store, perm)(next))
	}

	// ── Health ──────────────────────────────────────────
	r.mux.HandleFunc("GET /health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)
	r.mux.HandleFunc("GET /readyz", r.handleReadiness)
	detailedH := handlers.NewDetailedHealthHandler(r.core)
	detailedH.SetRateLimiter(r.globalRL)
	r.mux.HandleFunc("GET /health/detailed", detailedH.DetailedHealth)

	// ── OpenAPI Spec (cacheable) ──────────────────────
	openAPIH := handlers.NewOpenAPIHandler(r.core.Build.Version)
	r.mux.HandleFunc("GET /api/v1/openapi.json", middleware.ETag(openAPIH.Spec))

	// ── Auth (public, rate-limited) ────────────────────
	// Raised from 5/3 to 120/120 req/min — the previous limits were far too low
	// for E2E test suites which make many concurrent auth calls. Matches the
	// global per-IP default (120 req/min). Production is unchanged.
	loginRL := middleware.NewAuthRateLimiter(r.core.DB.Bolt, 120, time.Minute, "login")
	registerRL := middleware.NewAuthRateLimiter(r.core.DB.Bolt, 120, time.Minute, "register")
	refreshRL := middleware.NewAuthRateLimiter(r.core.DB.Bolt, 5, time.Minute, "refresh")
	authH := handlers.NewAuthHandler(r.authMod, r.store, r.core.DB.Bolt)
	r.mux.HandleFunc("POST /api/v1/auth/login", loginRL.Wrap(authH.Login))
	r.mux.HandleFunc("POST /api/v1/auth/register", registerRL.Wrap(authH.Register))
	r.mux.HandleFunc("POST /api/v1/auth/refresh", refreshRL.Wrap(authH.Refresh))
	r.mux.HandleFunc("POST /api/v1/auth/logout", authH.Logout)

	// ── Session / Profile ─────────────────────────────
	sessionH := handlers.NewSessionHandler(r.store, r.core.DB.Bolt, r.authMod)
	r.mux.Handle("GET /api/v1/auth/me", protected(http.HandlerFunc(sessionH.GetCurrentUser)))
	r.mux.Handle("PATCH /api/v1/auth/me", protected(http.HandlerFunc(sessionH.UpdateProfile)))
	r.mux.Handle("POST /api/v1/auth/change-password", protected(http.HandlerFunc(sessionH.ChangePassword)))
	r.mux.Handle("GET /api/v1/auth/sessions", protected(http.HandlerFunc(sessionH.ListSessions)))
	r.mux.Handle("POST /api/v1/auth/logout-all", protected(http.HandlerFunc(sessionH.LogoutAll)))

	// ── Webhooks (signature-verified, not JWT) ─────────
	// Tighter 1MB body limit for external webhook payloads (vs global 10MB)
	webhookRecv := webhooks.NewReceiver(r.store, r.core.DB.Bolt, r.core.Events, r.core.Logger)
	r.mux.Handle("POST /hooks/v1/{webhookID}",
		middleware.BodyLimit(maxWebhookBody)(http.HandlerFunc(webhookRecv.HandleWebhook)))

	// Track outbound webhook delivery success/failure in BBolt
	deliveryTracker := webhooks.NewDeliveryTracker(r.core.DB.Bolt, r.core.Events)
	deliveryTracker.Start()
	_ = deliveryTracker // tracked via event subscriptions, no direct reference needed

	// ── Dashboard ─────────────────────────────────────
	dashH := handlers.NewDashboardHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /api/v1/dashboard/stats", protected(http.HandlerFunc(dashH.Stats)))

	// ── Apps ────────────────────────────────────────────
	appH := handlers.NewAppHandler(r.store, r.core)
	r.mux.Handle("GET /api/v1/apps", protected(http.HandlerFunc(appH.List)))
	r.mux.Handle("POST /api/v1/apps", protectedPerm(auth.PermAppCreate, appH.Create))
	ixH := handlers.NewImportExportHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/import", protectedPerm(auth.PermAppCreate, ixH.Import))
	r.mux.Handle("GET /api/v1/apps/{id}/export", protected(http.HandlerFunc(ixH.Export)))
	r.mux.Handle("GET /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Get)))
	r.mux.Handle("PATCH /api/v1/apps/{id}", protectedPerm(auth.PermAppCreate, appH.Update))
	r.mux.Handle("DELETE /api/v1/apps/{id}", protectedPerm(auth.PermAppDelete, appH.Delete))
	r.mux.Handle("POST /api/v1/apps/{id}/restart", protectedPerm(auth.PermAppRestart, appH.Restart))
	r.mux.Handle("POST /api/v1/apps/{id}/stop", protectedPerm(auth.PermAppStop, appH.Stop))
	r.mux.Handle("POST /api/v1/apps/{id}/start", protectedPerm(auth.PermAppRestart, appH.Start))
	deployTriggerH := handlers.NewDeployTriggerHandler(r.store, r.core.Services.Container, r.core.Events)
	deployTriggerH.SetServerContext(r.serverCtx)
	r.mux.Handle("POST /api/v1/apps/{id}/deploy", protectedPerm(auth.PermAppDeploy, deployTriggerH.TriggerDeploy))

	// ── App Suspend/Resume & Transfer ────────────────
	suspH := handlers.NewSuspendHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/suspend", protectedPerm(auth.PermAppStop, suspH.Suspend))
	r.mux.Handle("POST /api/v1/apps/{id}/resume", protectedPerm(auth.PermAppRestart, suspH.Resume))
	txfrH := handlers.NewTransferHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/transfer", adminOnly(http.HandlerFunc(txfrH.TransferApp)))

	// ── Metrics Export ────────────────────────────────
	mxExportH := handlers.NewMetricsExportHandler(r.store, r.core.DB.Bolt, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/metrics/export", protected(http.HandlerFunc(mxExportH.Export)))

	// ── App Rename ────────────────────────────────────
	renameH := handlers.NewRenameHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rename", protectedPerm(auth.PermAppCreate, renameH.Rename))

	// ── GPU Config ────────────────────────────────────
	gpuH := handlers.NewGPUHandler(r.store, r.core.Services.Container, r.core.DB.Bolt)
	gpuH.SetEvents(r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/gpu", protected(http.HandlerFunc(gpuH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/gpu", protectedPerm(auth.PermAppCreate, gpuH.Update))

	// ── App Pin ───────────────────────────────────────
	pinH := handlers.NewPinHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/pin", protectedPerm(auth.PermAppCreate, pinH.Pin))
	r.mux.Handle("DELETE /api/v1/apps/{id}/pin", protectedPerm(auth.PermAppCreate, pinH.Unpin))

	// ── Save as Template ──────────────────────────────
	saveTmplH := handlers.NewSaveTemplateHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/{id}/save-template", protectedPerm(auth.PermAppCreate, saveTmplH.Save))

	// ── Commit Rollback ───────────────────────────────
	crH := handlers.NewCommitRollbackHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rollback-to-commit", protectedPerm(auth.PermAppDeploy, crH.RollbackToCommit))

	// ── Snapshots ─────────────────────────────────────
	snapH := handlers.NewSnapshotHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/snapshots", protected(http.HandlerFunc(snapH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/snapshots", protectedPerm(auth.PermAppCreate, snapH.Create))

	// ── Webhook Replay ────────────────────────────────
	whReplayH := handlers.NewWebhookReplayHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/webhooks/{logId}/replay", protectedPerm(auth.PermAppCreate, whReplayH.Replay))

	// ── App Clone & Bulk Ops ──────────────────────────
	cloneH := handlers.NewCloneHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/clone", protectedPerm(auth.PermAppCreate, cloneH.Clone))
	bulkH := handlers.NewBulkHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/bulk", protectedPerm(auth.PermAppCreate, bulkH.Execute))

	// ── App Restart Policy ────────────────────────────
	rpH := handlers.NewRestartPolicyHandler(r.store, r.core.Services.Container)
	r.mux.Handle("PUT /api/v1/apps/{id}/restart-policy", protectedPerm(auth.PermAppCreate, rpH.Update))

	// ── App Labels ────────────────────────────────────
	labelsH := handlers.NewLabelsHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/labels", protected(http.HandlerFunc(labelsH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/labels", protectedPerm(auth.PermAppCreate, labelsH.Update))

	// ── Disk Usage ────────────────────────────────────
	diskH := handlers.NewDiskUsageHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/disk", protected(http.HandlerFunc(diskH.AppDisk)))

	// ── Webhook Test ──────────────────────────────────
	whTestH := handlers.NewWebhookTestDeliveryHandler(r.store, r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/webhooks/test", protectedPerm(auth.PermAppCreate, whTestH.TestDeliver))

	// ── App Ports ─────────────────────────────────────
	portH := handlers.NewPortHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/ports", protected(http.HandlerFunc(portH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/ports", protectedPerm(auth.PermAppCreate, portH.Update))

	// ── Health Check Config ───────────────────────────
	hcH := handlers.NewHealthCheckHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/healthcheck", protected(http.HandlerFunc(hcH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/healthcheck", protectedPerm(auth.PermAppCreate, hcH.Update))

	// ── Custom Commands ──────────────────────────────
	cmdH := handlers.NewCommandHandler(r.core.Services.Container, r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/commands", protectedPerm(auth.PermAppRestart, cmdH.Run))
	r.mux.Handle("GET /api/v1/apps/{id}/commands", protected(http.HandlerFunc(cmdH.History)))

	// ── Log Retention ─────────────────────────────────
	lrH := handlers.NewLogRetentionHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/log-retention", protected(http.HandlerFunc(lrH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/log-retention", protectedPerm(auth.PermAppCreate, lrH.Update))

	// ── App Middleware Config ─────────────────────────
	amwH := handlers.NewAppMiddlewareHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/middleware", protected(http.HandlerFunc(amwH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/middleware", protectedPerm(auth.PermAppCreate, amwH.Update))

	// ── Restart History ───────────────────────────────
	rstHistH := handlers.NewRestartHistoryHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/restarts", protected(http.HandlerFunc(rstHistH.List)))

	// ── Webhook Secret Rotation ───────────────────────
	whRotH := handlers.NewWebhookRotateHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/webhooks/rotate", protectedPerm(auth.PermAppCreate, whRotH.Rotate))

	// ── Deploy Preview, Diff & Schedule ───────────────
	dpH := handlers.NewDeployPreviewHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/{id}/deploy/preview", protectedPerm(auth.PermAppDeploy, dpH.Preview))
	diffH := handlers.NewDeployDiffHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/deployments/diff", protected(http.HandlerFunc(diffH.Diff)))

	// ── Build Logs ────────────────────────────────────
	bldLogH := handlers.NewBuildLogHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/builds/latest/log", protected(http.HandlerFunc(bldLogH.Get)))
	r.mux.Handle("GET /api/v1/apps/{id}/builds/latest/log/download", protected(http.HandlerFunc(bldLogH.Download)))

	// ── Maintenance Mode ──────────────────────────────
	maintH := handlers.NewMaintenanceHandler(r.store, r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/maintenance", protected(http.HandlerFunc(maintH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/maintenance", protectedPerm(auth.PermAppCreate, maintH.Update))

	// ── Redirects ─────────────────────────────────────
	redirH := handlers.NewRedirectHandler(r.store, r.core.DB.Bolt)
	redirH.SetEvents(r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/redirects", protected(http.HandlerFunc(redirH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/redirects", protectedPerm(auth.PermAppCreate, redirH.Create))
	r.mux.Handle("DELETE /api/v1/apps/{id}/redirects/{ruleId}", protectedPerm(auth.PermAppCreate, redirH.Delete))

	// ── Error Pages ───────────────────────────────────
	epH := handlers.NewErrorPageHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/error-pages", protected(http.HandlerFunc(epH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/error-pages", protectedPerm(auth.PermAppCreate, epH.Update))

	// ── Sticky Sessions ───────────────────────────────
	stickyH := handlers.NewStickySessionHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/sticky-sessions", protected(http.HandlerFunc(stickyH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/sticky-sessions", protectedPerm(auth.PermAppCreate, stickyH.Update))

	// ── Autoscale ─────────────────────────────────────
	asH := handlers.NewAutoscaleHandler(r.store, r.core.DB.Bolt)
	asH.SetEvents(r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/autoscale", protected(http.HandlerFunc(asH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/autoscale", protectedPerm(auth.PermAppCreate, asH.Update))

	// ── Response Headers ──────────────────────────────
	rhH := handlers.NewResponseHeadersHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/response-headers", protected(http.HandlerFunc(rhH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/response-headers", protectedPerm(auth.PermAppCreate, rhH.Update))

	// ── Container History ─────────────────────────────
	chH := handlers.NewContainerHistoryHandler(r.store, r.core.Services.Container, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/containers/history", protected(http.HandlerFunc(chH.History)))

	// ── Deploy Notifications ──────────────────────────
	dnH := handlers.NewDeployNotifyHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/deploy-notifications", protected(http.HandlerFunc(dnH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/deploy-notifications", protectedPerm(auth.PermAppCreate, dnH.Update))

	// ── Basic Auth ────────────────────────────────────
	baH := handlers.NewBasicAuthHandler(r.store, r.core.DB.Bolt)
	baH.SetEvents(r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/basic-auth", protected(http.HandlerFunc(baH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/basic-auth", protectedPerm(auth.PermAppCreate, baH.Update))

	// ── Container Processes ───────────────────────────
	topH := handlers.NewContainerTopHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/processes", protected(http.HandlerFunc(topH.Top)))

	// ── Webhook Logs ──────────────────────────────────
	whLogH := handlers.NewWebhookLogHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/webhooks/logs", protected(http.HandlerFunc(whLogH.List)))

	// ── Cron Jobs ─────────────────────────────────────
	cronH := handlers.NewCronJobHandler(r.store, r.core.DB.Bolt)
	cronH.SetEvents(r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/cron", protected(http.HandlerFunc(cronH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/cron", protectedPerm(auth.PermAppCreate, cronH.Create))
	r.mux.Handle("DELETE /api/v1/apps/{id}/cron/{jobId}", protectedPerm(auth.PermAppCreate, cronH.Delete))

	// ── Log Download ──────────────────────────────────
	logDlH := handlers.NewLogDownloadHandler(r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/logs/download", protected(http.HandlerFunc(logDlH.Download)))

	// ── Rollback & Versions ───────────────────────────
	rollbackH := handlers.NewRollbackHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rollback", protectedPerm(auth.PermAppDeploy, rollbackH.Rollback))
	r.mux.Handle("GET /api/v1/apps/{id}/versions", protected(http.HandlerFunc(rollbackH.ListVersions)))

	// ── Deployments ────────────────────────────────────
	depH := handlers.NewDeploymentHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/deployments", protected(http.HandlerFunc(depH.ListByApp)))
	r.mux.Handle("GET /api/v1/apps/{id}/deployments/latest", protected(http.HandlerFunc(depH.GetLatest)))

	// ── File Browser ──────────────────────────────────
	fbH := handlers.NewFileBrowserHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/files", protected(http.HandlerFunc(fbH.List)))

	// ── Stats & Scaling ───────────────────────────────
	statsH := handlers.NewStatsHandler(r.core.Services.Container, r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/stats", protected(http.HandlerFunc(statsH.AppStats)))
	r.mux.Handle("GET /api/v1/servers/stats", protected(http.HandlerFunc(statsH.ServerStats)))
	scaleH := handlers.NewScaleHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/scale", protected(http.HandlerFunc(scaleH.Scale)))

	// ── Resource Limits ───────────────────────────────
	resH := handlers.NewResourceHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/resources", protected(http.HandlerFunc(resH.GetLimits)))
	r.mux.Handle("PUT /api/v1/apps/{id}/resources", protected(http.HandlerFunc(resH.SetLimits)))

	// ── Dependencies ─────────────────────────────────
	depGraphH := handlers.NewDependencyHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/dependencies", protected(http.HandlerFunc(depGraphH.Graph)))

	// ── Metrics History ───────────────────────────────
	metricsH := handlers.NewMetricsHistoryHandler(r.store, r.core.Services.Container, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/metrics", protected(http.HandlerFunc(metricsH.AppMetrics)))
	r.mux.Handle("GET /api/v1/servers/{id}/metrics", protected(http.HandlerFunc(metricsH.ServerMetrics)))

	// ── Environments ──────────────────────────────────
	envPresetsH := handlers.NewEnvironmentHandler(r.store)
	r.mux.HandleFunc("GET /api/v1/environments/presets", envPresetsH.ListPresets)
	r.mux.Handle("POST /api/v1/projects/{id}/environment", protected(http.HandlerFunc(envPresetsH.ApplyPreset)))

	// ── Networks ──────────────────────────────────────
	netH := handlers.NewNetworkHandler(r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /api/v1/networks", protected(http.HandlerFunc(netH.List)))
	r.mux.Handle("POST /api/v1/networks/connect", protected(http.HandlerFunc(netH.Connect)))

	// ── Env Import/Export ─────────────────────────────
	envImH := handlers.NewEnvImportHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/{id}/env/import", protectedPerm(auth.PermAppEnvEdit, envImH.Import))
	r.mux.Handle("GET /api/v1/apps/{id}/env/export", protected(http.HandlerFunc(envImH.Export)))

	// ── DNS Records ───────────────────────────────────
	dnsRecH := handlers.NewDNSRecordHandler(r.core.Services)
	dnsRecH.SetEvents(r.core.Events)
	r.mux.Handle("GET /api/v1/dns/records", protected(http.HandlerFunc(dnsRecH.List)))
	r.mux.Handle("POST /api/v1/dns/records", protectedPerm(auth.PermDomainManage, dnsRecH.Create))
	r.mux.Handle("DELETE /api/v1/dns/records/{id}", protectedPerm(auth.PermDomainManage, dnsRecH.Delete))

	// ── SSL Status ────────────────────────────────────
	sslStatH := handlers.NewSSLStatusHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/domains/ssl-check", protected(http.HandlerFunc(sslStatH.Check)))

	// ── Agents ────────────────────────────────────────
	agentH := handlers.NewAgentStatusHandler(r.core)
	r.mux.Handle("GET /api/v1/agents", protected(http.HandlerFunc(agentH.List)))
	r.mux.Handle("GET /api/v1/agents/{id}", protected(http.HandlerFunc(agentH.GetAgent)))

	// ── Logs ──────────────────────────────────────────
	logH := handlers.NewLogHandler(r.core.Services.Container, r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/logs", protected(http.HandlerFunc(logH.GetLogs)))

	// ── Domain Verification ──────────────────────────
	dvH := handlers.NewDomainVerifyHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/domains/{id}/verify", protected(http.HandlerFunc(dvH.Verify)))
	r.mux.Handle("POST /api/v1/domains/verify-batch", protected(http.HandlerFunc(dvH.BatchVerify)))

	// ── Certificates ─────────────────────────────────
	certH := handlers.NewCertificateHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/certificates", protected(http.HandlerFunc(certH.List)))
	r.mux.Handle("POST /api/v1/certificates", protected(http.HandlerFunc(certH.Upload)))

	// ── Wildcard SSL ──────────────────────────────────
	wildcardH := handlers.NewWildcardSSLHandler(r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/certificates/wildcard", protected(http.HandlerFunc(wildcardH.Request)))

	// ── Image Tags & Cleanup ──────────────────────────
	imgTagH := handlers.NewImageTagHandler(r.store, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/images/tags", protected(http.HandlerFunc(imgTagH.List)))
	imgCleanH := handlers.NewImageCleanupHandler(r.core.Services.Container)
	r.mux.Handle("GET /api/v1/images/dangling", protected(http.HandlerFunc(imgCleanH.DanglingImages)))
	r.mux.Handle("DELETE /api/v1/images/prune", protected(http.HandlerFunc(imgCleanH.Prune)))

	// ── Volumes ───────────────────────────────────────
	volH := handlers.NewVolumeHandler(r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /api/v1/volumes", protected(http.HandlerFunc(volH.List)))
	r.mux.Handle("POST /api/v1/volumes", protected(http.HandlerFunc(volH.Create)))

	// ── Projects ───────────────────────────────────────
	projH := handlers.NewProjectHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/projects", protected(http.HandlerFunc(projH.List)))
	r.mux.Handle("POST /api/v1/projects", protectedPerm(auth.PermProjectCreate, projH.Create))
	r.mux.Handle("GET /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Get)))
	r.mux.Handle("DELETE /api/v1/projects/{id}", protectedPerm(auth.PermProjectDelete, projH.Delete))

	// ── Env Vars ──────────────────────────────────────
	envH := handlers.NewEnvVarHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/env", protected(http.HandlerFunc(envH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/env", protectedPerm(auth.PermAppEnvEdit, envH.Update))

	// ── Docker Registries ─────────────────────────────
	regH := handlers.NewRegistryHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/registries", protected(http.HandlerFunc(regH.List)))
	r.mux.Handle("POST /api/v1/registries", protected(http.HandlerFunc(regH.Add)))

	// ── Domains ────────────────────────────────────────
	domH := handlers.NewDomainHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/domains", protected(http.HandlerFunc(domH.List)))
	r.mux.Handle("POST /api/v1/domains", protectedPerm(auth.PermDomainManage, domH.Create))
	r.mux.Handle("DELETE /api/v1/domains/{id}", protectedPerm(auth.PermDomainManage, domH.Delete))

	// ── Container Exec ────────────────────────────────
	execH := handlers.NewExecHandler(r.core.Services.Container, r.store, r.core.Logger, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/exec", protectedPerm(auth.PermAppRestart, execH.Exec))

	// ── Team ───────────────────────────────────────────
	teamH := handlers.NewTeamHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/team/roles", protected(http.HandlerFunc(teamH.ListRoles)))
	inviteH := handlers.NewInviteHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/team/invites", protected(http.HandlerFunc(inviteH.List)))
	r.mux.Handle("POST /api/v1/team/invites", protected(http.HandlerFunc(inviteH.Create)))
	r.mux.Handle("GET /api/v1/team/audit-log", protected(http.HandlerFunc(teamH.GetAuditLog)))

	// ── Databases ─────────────────────────────────────
	dbH := handlers.NewDatabaseHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.HandleFunc("GET /api/v1/databases/engines", dbH.ListEngines)
	r.mux.Handle("POST /api/v1/databases", protectedPerm(auth.PermDatabaseManage, dbH.Create))

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
	sshTestH := handlers.NewSSHTestHandler(r.core.Services)
	r.mux.Handle("POST /api/v1/servers/test-ssh", protected(http.HandlerFunc(sshTestH.Test)))

	// ── Build Cache ──────────────────────────────────
	bcH := handlers.NewBuildCacheHandler(r.core.Services.Container, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/build/cache", protected(http.HandlerFunc(bcH.Stats)))
	r.mux.Handle("DELETE /api/v1/build/cache", protected(http.HandlerFunc(bcH.Clear)))

	// ── Tenant Settings ──────────────────────────────
	tsH := handlers.NewTenantSettingsHandler(r.store)
	r.mux.Handle("GET /api/v1/tenant/settings", protected(http.HandlerFunc(tsH.Get)))
	r.mux.Handle("PATCH /api/v1/tenant/settings", protected(http.HandlerFunc(tsH.Update)))

	// ── Storage Usage ─────────────────────────────────
	storageH := handlers.NewStorageHandler(r.store, r.core.Services.Container, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/storage/usage", protected(http.HandlerFunc(storageH.Usage)))

	// ── Git Sources ───────────────────────────────────
	gitH := handlers.NewGitSourceHandler(r.core.Services)
	r.mux.Handle("GET /api/v1/git/providers", protected(http.HandlerFunc(gitH.ListProviders)))
	r.mux.Handle("GET /api/v1/git/{provider}/repos", protected(http.HandlerFunc(gitH.ListRepos)))
	r.mux.Handle("GET /api/v1/git/{provider}/repos/{repo}/branches", protected(http.HandlerFunc(gitH.ListBranches)))

	// ── Compose Stacks ────────────────────────────────
	composeH := handlers.NewComposeHandler(r.store, r.core.Services.Container, r.core.Events)
	composeH.SetServerContext(r.serverCtx)
	r.mux.Handle("POST /api/v1/stacks", protectedPerm(auth.PermAppCreate, composeH.Deploy))
	r.mux.Handle("POST /api/v1/stacks/validate", protected(http.HandlerFunc(composeH.Validate)))

	// ── Secrets ───────────────────────────────────────
	var vault interface {
		Encrypt(string) (string, error)
		Decrypt(string) (string, error)
	}
	secretsMod := r.core.Registry.Get("secrets")
	if secretsMod != nil {
		type vaultProvider interface {
			Vault() interface {
				Encrypt(string) (string, error)
				Decrypt(string) (string, error)
			}
		}
		if vp, ok := secretsMod.(vaultProvider); ok {
			vault = vp.Vault()
		}
	}
	secretH := handlers.NewSecretHandler(r.store, vault, r.core.Events)
	r.mux.Handle("GET /api/v1/secrets", protected(http.HandlerFunc(secretH.List)))
	r.mux.Handle("POST /api/v1/secrets", protectedPerm(auth.PermSecretCreate, secretH.Create))

	// ── Billing ───────────────────────────────────────
	billingH := handlers.NewBillingHandler(r.store)
	r.mux.HandleFunc("GET /api/v1/billing/plans", billingH.ListPlans)
	r.mux.Handle("GET /api/v1/billing/usage", protected(http.HandlerFunc(billingH.GetUsage)))
	usageHistH := handlers.NewUsageHistoryHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/billing/usage/history", protected(http.HandlerFunc(usageHistH.Hourly)))

	// Stripe webhook endpoint — no bearer auth; Stripe signs the request with
	// the shared webhook secret and the handler verifies that signature.
	// Registered only when the billing module has Stripe wired up.
	if billingMod, ok := r.core.Registry.Get("billing").(*billing.Module); ok {
		if wh := billingMod.WebhookHandler(); wh != nil {
			stripeWH := handlers.NewStripeWebhookHandler(wh, r.core.DB.Bolt, r.core.Logger)
			r.mux.Handle("POST /api/v1/webhooks/stripe", stripeWH)
		}
	}

	// ── Marketplace (public list, auth for deploy) ────
	mpMod := r.core.Registry.Get("marketplace")
	if mpMod != nil {
		mm, ok := mpMod.(*marketplace.Module)
		if !ok {
			slog.Warn("marketplace module has unexpected type, skipping")
		}
		var reg *marketplace.TemplateRegistry
		if ok {
			reg = mm.Registry()
		}
		if reg == nil {
			reg = marketplace.NewTemplateRegistry()
		}
		mpH := handlers.NewMarketplaceHandler(reg)
		r.mux.HandleFunc("GET /api/v1/marketplace", middleware.ETag(mpH.List))
		r.mux.HandleFunc("GET /api/v1/marketplace/{slug}", middleware.ETag(mpH.Get))
		mpDeployH := handlers.NewMarketplaceDeployHandler(reg, r.core.Services.Container, r.store, r.core.Events)
		mpDeployH.SetServerContext(r.serverCtx)
		r.mux.Handle("POST /api/v1/marketplace/deploy", protected(http.HandlerFunc(mpDeployH.Deploy)))
	}

	// ── Topology Editor ───────────────────────────────────────
	topologyH := handlers.NewTopologyHandler(r.store, r.core)
	r.mux.Handle("POST /api/v1/topology", protected(http.HandlerFunc(topologyH.Save)))
	r.mux.Handle("GET /api/v1/topology/{projectId}/{environment}", protected(http.HandlerFunc(topologyH.Load)))
	r.mux.Handle("POST /api/v1/topology/compile", protected(http.HandlerFunc(topologyH.Compile)))
	r.mux.Handle("POST /api/v1/topology/validate", protected(http.HandlerFunc(topologyH.Validate)))
	r.mux.Handle("POST /api/v1/topology/deploy", protected(http.HandlerFunc(topologyH.Deploy)))
	r.mux.Handle("GET /api/v1/topology/templates", protected(http.HandlerFunc(topologyH.Templates)))

	// ── Outbound Event Webhooks ───────────────────────
	evtWhH := handlers.NewEventWebhookHandler(r.store, r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/webhooks/outbound", protected(http.HandlerFunc(evtWhH.List)))
	r.mux.Handle("POST /api/v1/webhooks/outbound", protected(http.HandlerFunc(evtWhH.Create)))
	r.mux.Handle("DELETE /api/v1/webhooks/outbound/{id}", protected(http.HandlerFunc(evtWhH.Delete)))

	// ── Deploy Freeze ─────────────────────────────────
	freezeH := handlers.NewDeployFreezeHandler(r.store, r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/deploy/freeze", protected(http.HandlerFunc(freezeH.Get)))
	r.mux.Handle("POST /api/v1/deploy/freeze", protected(http.HandlerFunc(freezeH.Create)))
	r.mux.Handle("DELETE /api/v1/deploy/freeze/{id}", protected(http.HandlerFunc(freezeH.Delete)))

	// ── Env Compare ───────────────────────────────────
	ecH := handlers.NewEnvCompareHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/env/compare", protected(http.HandlerFunc(ecH.Compare)))

	// ── Notifications ─────────────────────────────────
	notifH := handlers.NewNotificationHandler(r.core.Services.Notifications)
	r.mux.Handle("POST /api/v1/notifications/test", protected(http.HandlerFunc(notifH.Test)))

	// ── Terminal ──────────────────────────────────────
	termH := ws.NewTerminal(r.core.Services.Container, r.store, r.core.Logger)
	r.mux.Handle("GET /api/v1/apps/{id}/terminal", protected(http.HandlerFunc(termH.StreamOutput)))
	r.mux.Handle("POST /api/v1/apps/{id}/terminal", protectedPerm(auth.PermAppRestart, termH.SendCommand))

	// ── Deploy Approval Workflow ──────────────────────
	daH := handlers.NewDeployApprovalHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/deploy/approvals", protected(http.HandlerFunc(daH.ListPending)))
	r.mux.Handle("POST /api/v1/deploy/approvals/{id}/approve", protected(http.HandlerFunc(daH.Approve)))
	r.mux.Handle("POST /api/v1/deploy/approvals/{id}/reject", protected(http.HandlerFunc(daH.Reject)))

	// ── Search ────────────────────────────────────────
	searchH := handlers.NewSearchHandler(r.store)
	r.mux.Handle("GET /api/v1/search", protected(http.HandlerFunc(searchH.Search)))

	// ── Activity Feed ─────────────────────────────────
	activityH := handlers.NewActivityHandler(r.store)
	r.mux.Handle("GET /api/v1/activity", protected(http.HandlerFunc(activityH.Feed)))

	// ── SSH Keys ──────────────────────────────────────
	sshH := handlers.NewSSHKeyHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/ssh-keys", protected(http.HandlerFunc(sshH.List)))
	r.mux.Handle("POST /api/v1/ssh-keys/generate", protected(http.HandlerFunc(sshH.Generate)))

	// ── MCP Protocol ──────────────────────────────────
	mcpH := handlers.NewMCPHandler(r.core, r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /mcp/v1/tools", protected(http.HandlerFunc(mcpH.ListTools)))
	r.mux.Handle("POST /mcp/v1/tools/{name}", protected(http.HandlerFunc(mcpH.CallTool)))

	// ── Streaming (SSE) ────────────────────────────────
	logStreamer := ws.NewLogStreamer(r.core.Services.Container, r.core.Logger)
	eventStreamer := ws.NewEventStreamer(r.core.Events, r.core.Logger)
	r.mux.Handle("GET /api/v1/apps/{id}/logs/stream", protected(http.HandlerFunc(logStreamer.StreamLogs)))
	r.mux.Handle("GET /api/v1/events/stream", protected(http.HandlerFunc(eventStreamer.StreamEvents)))

	// ── Deployment Progress (WebSocket) ──────────────────
	ws.GetDeployHub().SetAllowedOrigins(r.core.Config.Server.CORSOrigins)
	r.mux.Handle("GET /api/v1/topology/deploy/{projectId}/progress", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectId")
		if projectID == "" {
			http.Error(w, "project ID required", http.StatusBadRequest)
			return
		}
		ws.GetDeployHub().ServeWS(w, r, projectID)
	})))

	// ── Admin routes ─────────────────────────────────
	registerAdminRoutes(r, adminOnly)

	// ── Prometheus metrics (internal, auth-protected) ──
	// Exposes runtime metrics — protect with auth to prevent info disclosure
	promExporter := integrations.NewPrometheusExporter(r.core.Registry, r.core.Events, r.core.Services)
	r.mux.Handle("GET /metrics", protected(promExporter.Handler()))
	r.mux.Handle("GET /metrics/api", protected(r.apiMetrics.Handler()))

	// ── pprof (opt-in, auth-protected) ───────────────
	if r.core.Config.Server.EnablePprof {
		r.mux.Handle("GET /debug/pprof/", protected(http.HandlerFunc(pprof.Index)))
		r.mux.Handle("GET /debug/pprof/cmdline", protected(http.HandlerFunc(pprof.Cmdline)))
		r.mux.Handle("GET /debug/pprof/profile", protected(http.HandlerFunc(pprof.Profile)))
		r.mux.Handle("GET /debug/pprof/symbol", protected(http.HandlerFunc(pprof.Symbol)))
		r.mux.Handle("GET /debug/pprof/trace", protected(http.HandlerFunc(pprof.Trace)))
	}

	// ── SPA fallback — embedded React UI ──────────────
	r.mux.Handle("/", newSPAHandler())
}

func (r *Router) handleHealth(w http.ResponseWriter, _ *http.Request) {
	health := r.core.Registry.HealthAll()
	status := "ok"
	httpStatus := http.StatusOK

	for _, h := range health {
		if h == core.HealthDown {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}
	}

	writeJSON(w, httpStatus, map[string]any{
		"status": status,
	})
}

// handleReadiness implements the /readyz endpoint for load balancer probing.
// Returns 200 when the server is ready to accept traffic, 503 when draining
// or when critical dependencies (database, container runtime) are unreachable.
// Use this for Kubernetes readinessProbe or cloud load balancer health checks.
func (r *Router) handleReadiness(w http.ResponseWriter, req *http.Request) {
	if r.core.IsDraining() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "draining",
		})
		return
	}

	// Verify critical dependencies with a tight timeout
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
	defer cancel()

	reasons := make([]string, 0)

	// Database must be reachable
	if r.core.Store != nil {
		if err := r.core.Store.Ping(ctx); err != nil {
			reasons = append(reasons, "database unreachable")
		}
	}

	// Container runtime must be reachable (if configured)
	if rt := r.core.Services.Container; rt != nil {
		if err := rt.Ping(); err != nil {
			reasons = append(reasons, "container runtime unreachable")
		}
	}

	if len(reasons) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "not_ready",
			"reasons": reasons,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}
