package api

import (
	"net/http"
	"net/http/pprof"
	"time"

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
	mux              *http.ServeMux
	core             *core.Core
	authMod          *auth.Module
	store            core.Store
	apiMetrics       *middleware.APIMetrics
	gracefulShutdown *middleware.GracefulShutdown
}

// NewRouter creates a new API router with all routes registered.
func NewRouter(c *core.Core, authMod *auth.Module, store core.Store) *Router {
	r := &Router{
		mux:              http.NewServeMux(),
		core:             c,
		authMod:          authMod,
		store:            store,
		apiMetrics:       middleware.NewAPIMetrics(),
		gracefulShutdown: middleware.NewGracefulShutdown(),
	}
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
		middleware.SecurityHeaders,
		r.apiMetrics.Middleware,
		middleware.APIVersion(r.core.Build.Version),
		middleware.BodyLimit(10<<20),       // 10MB max request body
		middleware.Timeout(30*time.Second), // 30s request timeout
		middleware.Recovery(r.core.Logger),
		middleware.RequestLogger(r.core.Logger),
		middleware.CORS(r.core.Config.Server.CORSOrigins),
		middleware.CSRFProtect,
		middleware.AuditLog(r.store, r.core.Logger),
	)
}

func (r *Router) registerRoutes() {
	protected := middleware.RequireAuth(r.authMod.JWT(), r.core.DB.Bolt)

	// ── Health ──────────────────────────────────────────
	r.mux.HandleFunc("GET /health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)
	r.mux.HandleFunc("GET /readyz", r.handleReadiness)
	detailedH := handlers.NewDetailedHealthHandler(r.core)
	r.mux.HandleFunc("GET /health/detailed", detailedH.DetailedHealth)

	// ── OpenAPI Spec (cacheable) ──────────────────────
	openAPIH := handlers.NewOpenAPIHandler(r.core.Build.Version)
	r.mux.HandleFunc("GET /api/v1/openapi.json", middleware.ETag(openAPIH.Spec))

	// ── Auth (public, rate-limited) ────────────────────
	loginRL := middleware.NewAuthRateLimiter(r.core.DB.Bolt, 5, time.Minute, "login")
	registerRL := middleware.NewAuthRateLimiter(r.core.DB.Bolt, 3, time.Minute, "register")
	refreshRL := middleware.NewAuthRateLimiter(r.core.DB.Bolt, 5, time.Minute, "refresh")
	authH := handlers.NewAuthHandler(r.authMod, r.store, r.core.DB.Bolt)
	r.mux.HandleFunc("POST /api/v1/auth/login", loginRL.Wrap(authH.Login))
	r.mux.HandleFunc("POST /api/v1/auth/register", registerRL.Wrap(authH.Register))
	r.mux.HandleFunc("POST /api/v1/auth/refresh", refreshRL.Wrap(authH.Refresh))
	r.mux.HandleFunc("POST /api/v1/auth/logout", authH.Logout)

	// ── Session / Profile ─────────────────────────────
	sessionH := handlers.NewSessionHandler(r.store)
	r.mux.Handle("GET /api/v1/auth/me", protected(http.HandlerFunc(sessionH.GetCurrentUser)))
	r.mux.Handle("PATCH /api/v1/auth/me", protected(http.HandlerFunc(sessionH.UpdateProfile)))
	r.mux.Handle("POST /api/v1/auth/change-password", protected(http.HandlerFunc(sessionH.ChangePassword)))

	// ── Webhooks (signature-verified, not JWT) ─────────
	webhookRecv := webhooks.NewReceiver(r.store, r.core.DB.Bolt, r.core.Events, r.core.Logger)
	r.mux.HandleFunc("POST /hooks/v1/{webhookID}", webhookRecv.HandleWebhook)

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
	r.mux.Handle("POST /api/v1/apps", protected(http.HandlerFunc(appH.Create)))
	ixH := handlers.NewImportExportHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/import", protected(http.HandlerFunc(ixH.Import)))
	r.mux.Handle("GET /api/v1/apps/{id}/export", protected(http.HandlerFunc(ixH.Export)))
	r.mux.Handle("GET /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Get)))
	r.mux.Handle("PATCH /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Update)))
	r.mux.Handle("DELETE /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Delete)))
	r.mux.Handle("POST /api/v1/apps/{id}/restart", protected(http.HandlerFunc(appH.Restart)))
	r.mux.Handle("POST /api/v1/apps/{id}/stop", protected(http.HandlerFunc(appH.Stop)))
	r.mux.Handle("POST /api/v1/apps/{id}/start", protected(http.HandlerFunc(appH.Start)))
	deployTriggerH := handlers.NewDeployTriggerHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/deploy", protected(http.HandlerFunc(deployTriggerH.TriggerDeploy)))

	// ── App Suspend/Resume & Transfer ────────────────
	suspH := handlers.NewSuspendHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/suspend", protected(http.HandlerFunc(suspH.Suspend)))
	r.mux.Handle("POST /api/v1/apps/{id}/resume", protected(http.HandlerFunc(suspH.Resume)))
	txfrH := handlers.NewTransferHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/transfer", protected(http.HandlerFunc(txfrH.TransferApp)))

	// ── Metrics Export ────────────────────────────────
	mxExportH := handlers.NewMetricsExportHandler(r.core.DB.Bolt, r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/metrics/export", protected(http.HandlerFunc(mxExportH.Export)))

	// ── App Rename ────────────────────────────────────
	renameH := handlers.NewRenameHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rename", protected(http.HandlerFunc(renameH.Rename)))

	// ── GPU Config ────────────────────────────────────
	gpuH := handlers.NewGPUHandler(r.store, r.core.Services.Container, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/gpu", protected(http.HandlerFunc(gpuH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/gpu", protected(http.HandlerFunc(gpuH.Update)))

	// ── App Pin ───────────────────────────────────────
	pinH := handlers.NewPinHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/pin", protected(http.HandlerFunc(pinH.Pin)))
	r.mux.Handle("DELETE /api/v1/apps/{id}/pin", protected(http.HandlerFunc(pinH.Unpin)))

	// ── Save as Template ──────────────────────────────
	saveTmplH := handlers.NewSaveTemplateHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/{id}/save-template", protected(http.HandlerFunc(saveTmplH.Save)))

	// ── Commit Rollback ───────────────────────────────
	crH := handlers.NewCommitRollbackHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rollback-to-commit", protected(http.HandlerFunc(crH.RollbackToCommit)))

	// ── Canary Deployments ────────────────────────────
	canaryH := handlers.NewCanaryHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/canary", protected(http.HandlerFunc(canaryH.Get)))
	r.mux.Handle("POST /api/v1/apps/{id}/canary", protected(http.HandlerFunc(canaryH.Start)))
	r.mux.Handle("POST /api/v1/apps/{id}/canary/promote", protected(http.HandlerFunc(canaryH.Promote)))
	r.mux.Handle("DELETE /api/v1/apps/{id}/canary", protected(http.HandlerFunc(canaryH.Cancel)))

	// ── Snapshots ─────────────────────────────────────
	snapH := handlers.NewSnapshotHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/snapshots", protected(http.HandlerFunc(snapH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/snapshots", protected(http.HandlerFunc(snapH.Create)))

	// ── Service Links (Mesh) ──────────────────────────
	meshH := handlers.NewServiceMeshHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/links", protected(http.HandlerFunc(meshH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/links", protected(http.HandlerFunc(meshH.Create)))
	r.mux.Handle("DELETE /api/v1/apps/{id}/links/{targetId}", protected(http.HandlerFunc(meshH.Delete)))

	// ── Webhook Replay ────────────────────────────────
	whReplayH := handlers.NewWebhookReplayHandler(r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/webhooks/{logId}/replay", protected(http.HandlerFunc(whReplayH.Replay)))

	// ── App Clone & Bulk Ops ──────────────────────────
	cloneH := handlers.NewCloneHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/clone", protected(http.HandlerFunc(cloneH.Clone)))
	bulkH := handlers.NewBulkHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/bulk", protected(http.HandlerFunc(bulkH.Execute)))

	// ── App Restart Policy ────────────────────────────
	rpH := handlers.NewRestartPolicyHandler(r.store, r.core.Services.Container)
	r.mux.Handle("PUT /api/v1/apps/{id}/restart-policy", protected(http.HandlerFunc(rpH.Update)))

	// ── App Labels ────────────────────────────────────
	labelsH := handlers.NewLabelsHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/labels", protected(http.HandlerFunc(labelsH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/labels", protected(http.HandlerFunc(labelsH.Update)))

	// ── Disk Usage ────────────────────────────────────
	diskH := handlers.NewDiskUsageHandler(r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/disk", protected(http.HandlerFunc(diskH.AppDisk)))

	// ── Webhook Test ──────────────────────────────────
	whTestH := handlers.NewWebhookTestDeliveryHandler(r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/webhooks/test", protected(http.HandlerFunc(whTestH.TestDeliver)))

	// ── App Ports ─────────────────────────────────────
	portH := handlers.NewPortHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/ports", protected(http.HandlerFunc(portH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/ports", protected(http.HandlerFunc(portH.Update)))

	// ── Health Check Config ───────────────────────────
	hcH := handlers.NewHealthCheckHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/healthcheck", protected(http.HandlerFunc(hcH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/healthcheck", protected(http.HandlerFunc(hcH.Update)))

	// ── Custom Commands ──────────────────────────────
	cmdH := handlers.NewCommandHandler(r.core.Services.Container, r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/commands", protected(http.HandlerFunc(cmdH.Run)))
	r.mux.Handle("GET /api/v1/apps/{id}/commands", protected(http.HandlerFunc(cmdH.History)))

	// ── IP Access Control ─────────────────────────────
	ipH := handlers.NewIPWhitelistHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/access", protected(http.HandlerFunc(ipH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/access", protected(http.HandlerFunc(ipH.Update)))

	// ── Log Retention ─────────────────────────────────
	lrH := handlers.NewLogRetentionHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/log-retention", protected(http.HandlerFunc(lrH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/log-retention", protected(http.HandlerFunc(lrH.Update)))

	// ── App Middleware Config ─────────────────────────
	amwH := handlers.NewAppMiddlewareHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/middleware", protected(http.HandlerFunc(amwH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/middleware", protected(http.HandlerFunc(amwH.Update)))

	// ── Restart History ───────────────────────────────
	rstHistH := handlers.NewRestartHistoryHandler(r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/restarts", protected(http.HandlerFunc(rstHistH.List)))

	// ── Webhook Secret Rotation ───────────────────────
	whRotH := handlers.NewWebhookRotateHandler(r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/webhooks/rotate", protected(http.HandlerFunc(whRotH.Rotate)))

	// ── Deploy Preview, Diff & Schedule ───────────────
	dpH := handlers.NewDeployPreviewHandler(r.store)
	r.mux.Handle("POST /api/v1/apps/{id}/deploy/preview", protected(http.HandlerFunc(dpH.Preview)))
	diffH := handlers.NewDeployDiffHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/deployments/diff", protected(http.HandlerFunc(diffH.Diff)))
	schedH := handlers.NewDeployScheduleHandler(r.store, r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/deploy/schedule", protected(http.HandlerFunc(schedH.Schedule)))
	r.mux.Handle("GET /api/v1/apps/{id}/deploy/scheduled", protected(http.HandlerFunc(schedH.ListScheduled)))
	r.mux.Handle("DELETE /api/v1/apps/{id}/deploy/scheduled/{scheduleId}", protected(http.HandlerFunc(schedH.CancelScheduled)))

	// ── Build Logs ────────────────────────────────────
	bldLogH := handlers.NewBuildLogHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/builds/latest/log", protected(http.HandlerFunc(bldLogH.Get)))
	r.mux.Handle("GET /api/v1/apps/{id}/builds/latest/log/download", protected(http.HandlerFunc(bldLogH.Download)))

	// ── Maintenance Mode ──────────────────────────────
	maintH := handlers.NewMaintenanceHandler(r.store, r.core.Events, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/maintenance", protected(http.HandlerFunc(maintH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/maintenance", protected(http.HandlerFunc(maintH.Update)))

	// ── Redirects ─────────────────────────────────────
	redirH := handlers.NewRedirectHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/redirects", protected(http.HandlerFunc(redirH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/redirects", protected(http.HandlerFunc(redirH.Create)))
	r.mux.Handle("DELETE /api/v1/apps/{id}/redirects/{ruleId}", protected(http.HandlerFunc(redirH.Delete)))

	// ── Error Pages ───────────────────────────────────
	epH := handlers.NewErrorPageHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/error-pages", protected(http.HandlerFunc(epH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/error-pages", protected(http.HandlerFunc(epH.Update)))

	// ── Sticky Sessions ───────────────────────────────
	stickyH := handlers.NewStickySessionHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/sticky-sessions", protected(http.HandlerFunc(stickyH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/sticky-sessions", protected(http.HandlerFunc(stickyH.Update)))

	// ── Autoscale ─────────────────────────────────────
	asH := handlers.NewAutoscaleHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/autoscale", protected(http.HandlerFunc(asH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/autoscale", protected(http.HandlerFunc(asH.Update)))

	// ── Response Headers ──────────────────────────────
	rhH := handlers.NewResponseHeadersHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/response-headers", protected(http.HandlerFunc(rhH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/response-headers", protected(http.HandlerFunc(rhH.Update)))

	// ── Container History ─────────────────────────────
	chH := handlers.NewContainerHistoryHandler(r.core.Services.Container, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/containers/history", protected(http.HandlerFunc(chH.History)))

	// ── Deploy Notifications ──────────────────────────
	dnH := handlers.NewDeployNotifyHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/deploy-notifications", protected(http.HandlerFunc(dnH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/deploy-notifications", protected(http.HandlerFunc(dnH.Update)))

	// ── Basic Auth ────────────────────────────────────
	baH := handlers.NewBasicAuthHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/basic-auth", protected(http.HandlerFunc(baH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/basic-auth", protected(http.HandlerFunc(baH.Update)))

	// ── Container Processes ───────────────────────────
	topH := handlers.NewContainerTopHandler(r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/processes", protected(http.HandlerFunc(topH.Top)))

	// ── Webhook Logs ──────────────────────────────────
	whLogH := handlers.NewWebhookLogHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/webhooks/logs", protected(http.HandlerFunc(whLogH.List)))

	// ── Cron Jobs ─────────────────────────────────────
	cronH := handlers.NewCronJobHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/apps/{id}/cron", protected(http.HandlerFunc(cronH.List)))
	r.mux.Handle("POST /api/v1/apps/{id}/cron", protected(http.HandlerFunc(cronH.Create)))
	r.mux.Handle("DELETE /api/v1/apps/{id}/cron/{jobId}", protected(http.HandlerFunc(cronH.Delete)))

	// ── Log Download ──────────────────────────────────
	logDlH := handlers.NewLogDownloadHandler(r.core.Services.Container)
	r.mux.Handle("GET /api/v1/apps/{id}/logs/download", protected(http.HandlerFunc(logDlH.Download)))

	// ── Rollback & Versions ───────────────────────────
	rollbackH := handlers.NewRollbackHandler(r.store, r.core.Services.Container, r.core.Events)
	r.mux.Handle("POST /api/v1/apps/{id}/rollback", protected(http.HandlerFunc(rollbackH.Rollback)))
	r.mux.Handle("GET /api/v1/apps/{id}/versions", protected(http.HandlerFunc(rollbackH.ListVersions)))

	// ── Deployments ────────────────────────────────────
	depH := handlers.NewDeploymentHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/apps/{id}/deployments", protected(http.HandlerFunc(depH.ListByApp)))
	r.mux.Handle("GET /api/v1/apps/{id}/deployments/latest", protected(http.HandlerFunc(depH.GetLatest)))

	// ── File Browser ──────────────────────────────────
	fbH := handlers.NewFileBrowserHandler(r.core.Services.Container)
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
	r.mux.Handle("POST /api/v1/apps/{id}/env/import", protected(http.HandlerFunc(envImH.Import)))
	r.mux.Handle("GET /api/v1/apps/{id}/env/export", protected(http.HandlerFunc(envImH.Export)))

	// ── DNS Records ───────────────────────────────────
	dnsRecH := handlers.NewDNSRecordHandler(r.core.Services)
	r.mux.Handle("GET /api/v1/dns/records", protected(http.HandlerFunc(dnsRecH.List)))
	r.mux.Handle("POST /api/v1/dns/records", protected(http.HandlerFunc(dnsRecH.Create)))
	r.mux.Handle("DELETE /api/v1/dns/records/{id}", protected(http.HandlerFunc(dnsRecH.Delete)))

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
	imgTagH := handlers.NewImageTagHandler(r.core.Services.Container)
	r.mux.HandleFunc("GET /api/v1/images/tags", imgTagH.List)
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
	r.mux.Handle("POST /api/v1/projects", protected(http.HandlerFunc(projH.Create)))
	r.mux.Handle("GET /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Get)))
	r.mux.Handle("DELETE /api/v1/projects/{id}", protected(http.HandlerFunc(projH.Delete)))

	// ── Env Vars ──────────────────────────────────────
	envH := handlers.NewEnvVarHandler(r.store)
	r.mux.Handle("GET /api/v1/apps/{id}/env", protected(http.HandlerFunc(envH.Get)))
	r.mux.Handle("PUT /api/v1/apps/{id}/env", protected(http.HandlerFunc(envH.Update)))

	// ── Docker Registries ─────────────────────────────
	regH := handlers.NewRegistryHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/registries", protected(http.HandlerFunc(regH.List)))
	r.mux.Handle("POST /api/v1/registries", protected(http.HandlerFunc(regH.Add)))

	// ── Domains ────────────────────────────────────────
	domH := handlers.NewDomainHandler(r.store, r.core.Events)
	r.mux.Handle("GET /api/v1/domains", protected(http.HandlerFunc(domH.List)))
	r.mux.Handle("POST /api/v1/domains", protected(http.HandlerFunc(domH.Create)))
	r.mux.Handle("DELETE /api/v1/domains/{id}", protected(http.HandlerFunc(domH.Delete)))

	// ── Container Exec ────────────────────────────────
	execH := handlers.NewExecHandler(r.core.Services.Container, r.store, r.core.Logger, r.core.DB.Bolt)
	r.mux.Handle("POST /api/v1/apps/{id}/exec", protected(http.HandlerFunc(execH.Exec)))

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
	r.mux.Handle("POST /api/v1/databases", protected(http.HandlerFunc(dbH.Create)))
	dbPoolH := handlers.NewDBPoolHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/databases/{id}/pool", protected(http.HandlerFunc(dbPoolH.Get)))
	r.mux.Handle("PUT /api/v1/databases/{id}/pool", protected(http.HandlerFunc(dbPoolH.Update)))

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

	// ── Server Management ─────────────────────────────
	srvMgmtH := handlers.NewServerManageHandler(r.core.Services, r.store, r.core.Events)
	r.mux.Handle("POST /api/v1/servers/{id}/decommission", protected(http.HandlerFunc(srvMgmtH.Decommission)))
	r.mux.Handle("POST /api/v1/servers/{id}/reboot", protected(http.HandlerFunc(srvMgmtH.Reboot)))

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
	r.mux.Handle("POST /api/v1/stacks", protected(http.HandlerFunc(composeH.Deploy)))
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
	r.mux.Handle("POST /api/v1/secrets", protected(http.HandlerFunc(secretH.Create)))

	// ── Billing ───────────────────────────────────────
	billingH := handlers.NewBillingHandler(r.store)
	r.mux.HandleFunc("GET /api/v1/billing/plans", billingH.ListPlans)
	r.mux.Handle("GET /api/v1/billing/usage", protected(http.HandlerFunc(billingH.GetUsage)))
	usageHistH := handlers.NewUsageHistoryHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/billing/usage/history", protected(http.HandlerFunc(usageHistH.Hourly)))

	// ── Marketplace (public list, auth for deploy) ────
	mpMod := r.core.Registry.Get("marketplace")
	if mpMod != nil {
		reg := mpMod.(*marketplace.Module).Registry()
		if reg == nil {
			reg = marketplace.NewTemplateRegistry()
		}
		mpH := handlers.NewMarketplaceHandler(reg)
		r.mux.HandleFunc("GET /api/v1/marketplace", middleware.ETag(mpH.List))
		r.mux.HandleFunc("GET /api/v1/marketplace/{slug}", middleware.ETag(mpH.Get))
		mpDeployH := handlers.NewMarketplaceDeployHandler(reg, r.core.Services.Container, r.store, r.core.Events)
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
	r.mux.Handle("POST /api/v1/apps/{id}/terminal", protected(http.HandlerFunc(termH.SendCommand)))

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
	r.mux.HandleFunc("GET /mcp/v1/tools", mcpH.ListTools)
	r.mux.HandleFunc("POST /mcp/v1/tools/{name}", mcpH.CallTool)

	// ── Streaming (SSE) ────────────────────────────────
	logStreamer := ws.NewLogStreamer(r.core.Services.Container, r.core.Logger)
	eventStreamer := ws.NewEventStreamer(r.core.Events, r.core.Logger)
	r.mux.Handle("GET /api/v1/apps/{id}/logs/stream", protected(http.HandlerFunc(logStreamer.StreamLogs)))
	r.mux.Handle("GET /api/v1/events/stream", protected(http.HandlerFunc(eventStreamer.StreamEvents)))

	// ── Deployment Progress (WebSocket) ──────────────────
	r.mux.Handle("GET /api/v1/topology/deploy/{projectId}/progress", protected(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID := r.PathValue("projectId")
		if projectID == "" {
			http.Error(w, "project ID required", http.StatusBadRequest)
			return
		}
		ws.GetDeployHub().ServeWS(w, r, projectID)
	})))

	// ── Announcements ─────────────────────────────────
	announcH := handlers.NewAnnouncementHandler(r.core.DB.Bolt)
	r.mux.HandleFunc("GET /api/v1/announcements", announcH.List) // public
	r.mux.Handle("POST /api/v1/admin/announcements", protected(http.HandlerFunc(announcH.Create)))
	r.mux.Handle("DELETE /api/v1/admin/announcements/{id}", protected(http.HandlerFunc(announcH.Dismiss)))

	// ── Admin Disk Usage ──────────────────────────────
	r.mux.Handle("GET /api/v1/admin/disk", protected(http.HandlerFunc(diskH.SystemDisk)))

	// ── Tenant Rate Limits (super admin) ──────────────
	trlH := handlers.NewTenantRateLimitHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/admin/tenants/{id}/ratelimit", protected(http.HandlerFunc(trlH.Get)))
	r.mux.Handle("PUT /api/v1/admin/tenants/{id}/ratelimit", protected(http.HandlerFunc(trlH.Update)))

	// ── Platform Stats (super admin) ──────────────────
	platH := handlers.NewPlatformStatsHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/stats", protected(http.HandlerFunc(platH.Overview)))

	// ── License ──────────────────────────────────────
	licH := handlers.NewLicenseHandler(r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/admin/license", protected(http.HandlerFunc(licH.Get)))
	r.mux.Handle("POST /api/v1/admin/license", protected(http.HandlerFunc(licH.Activate)))

	// ── DB Backup (admin) ─────────────────────────────
	dbBackupH := handlers.NewDBBackupHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/db/backup", protected(http.HandlerFunc(dbBackupH.Backup)))
	r.mux.Handle("GET /api/v1/admin/db/status", protected(http.HandlerFunc(dbBackupH.Status)))

	// ── Admin API Keys ────────────────────────────────
	adminKeyH := handlers.NewAdminAPIKeyHandler(r.store, r.core.DB.Bolt)
	r.mux.Handle("GET /api/v1/admin/api-keys", protected(http.HandlerFunc(adminKeyH.List)))
	r.mux.Handle("POST /api/v1/admin/api-keys", protected(http.HandlerFunc(adminKeyH.Generate)))
	r.mux.Handle("DELETE /api/v1/admin/api-keys/{prefix}", protected(http.HandlerFunc(adminKeyH.Revoke)))

	// ── DB Migrations ─────────────────────────────────
	migH := handlers.NewMigrationHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/db/migrations", protected(http.HandlerFunc(migH.Status)))

	// ── Admin (super admin only) ──────────────────────
	adminH := handlers.NewAdminHandler(r.core, r.store)
	r.mux.Handle("GET /api/v1/admin/system", protected(http.HandlerFunc(adminH.SystemInfo)))
	r.mux.Handle("PATCH /api/v1/admin/settings", protected(http.HandlerFunc(adminH.UpdateSettings)))
	r.mux.Handle("GET /api/v1/admin/tenants", protected(http.HandlerFunc(adminH.ListTenants)))

	// ── Self-Update ──────────────────────────────────
	updateH := handlers.NewSelfUpdateHandler(r.core)
	r.mux.Handle("GET /api/v1/admin/updates", protected(http.HandlerFunc(updateH.CheckUpdate)))

	// ── Branding (public GET, admin PATCH) ────────────
	brandingH := handlers.NewBrandingHandler()
	r.mux.HandleFunc("GET /api/v1/branding", brandingH.Get)
	r.mux.Handle("PATCH /api/v1/admin/branding", protected(http.HandlerFunc(brandingH.Update)))

	// ── Prometheus metrics (no auth — internal) ───────
	promExporter := integrations.NewPrometheusExporter(r.core.Registry, r.core.Events, r.core.Services)
	r.mux.HandleFunc("GET /metrics", promExporter.Handler())
	r.mux.HandleFunc("GET /metrics/api", r.apiMetrics.Handler())

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

// handleReadiness implements the /readyz endpoint for load balancer probing.
// Returns 200 when the server is ready to accept traffic, 503 when draining.
// Use this for Kubernetes readinessProbe or cloud load balancer health checks.
func (r *Router) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	if r.core.IsDraining() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "draining",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ready",
	})
}
