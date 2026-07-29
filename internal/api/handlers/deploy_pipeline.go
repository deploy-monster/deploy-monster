package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/deploy"
)

// ──────────────────── deploy_approval.go ────────────────────
// DeployApprovalHandler manages deployment approval workflows.
// When enabled, deploys require admin approval before executing.
type DeployApprovalHandler struct {
	store   core.Store
	events  *core.EventBus
	mu      sync.RWMutex
	pending map[string]*ApprovalRequest
}

func NewDeployApprovalHandler(store core.Store, events *core.EventBus) *DeployApprovalHandler {
	return &DeployApprovalHandler{
		store:   store,
		events:  events,
		pending: make(map[string]*ApprovalRequest),
	}
}

// ApprovalRequest represents a pending deployment approval.
type ApprovalRequest struct {
	ID          string     `json:"id"`
	AppID       string     `json:"app_id"`
	TenantID    string     `json:"tenant_id"` // required for tenant isolation
	RequestedBy string     `json:"requested_by"`
	Image       string     `json:"image"`
	Branch      string     `json:"branch"`
	Status      string     `json:"status"` // pending, approved, rejected
	Reason      string     `json:"reason,omitempty"`
	ReviewedBy  string     `json:"reviewed_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty"`
}

// ListPending handles GET /api/v1/deploy/approvals
func (h *DeployApprovalHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	items := make([]*ApprovalRequest, 0)
	for _, req := range h.pending {
		if req.Status == "pending" && req.TenantID == claims.TenantID {
			items = append(items, req)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

// Approve handles POST /api/v1/deploy/approvals/{id}/approve
func (h *DeployApprovalHandler) Approve(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	approvalID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	h.mu.Lock()
	req, exists := h.pending[approvalID]
	if !exists {
		h.mu.Unlock()
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}
	if req.Status != "pending" {
		h.mu.Unlock()
		writeError(w, http.StatusConflict, "approval already processed")
		return
	}
	// Tenant isolation: only the owning tenant can approve its own deploys
	if req.TenantID != claims.TenantID {
		h.mu.Unlock()
		writeError(w, http.StatusForbidden, "not authorized to approve this request")
		return
	}
	now := time.Now()
	req.Status = "approved"
	req.ReviewedBy = claims.UserID
	req.ReviewedAt = &now
	h.mu.Unlock()
	if h.events != nil {
		h.events.PublishAsync(r.Context(), core.NewEvent("deploy.approved", "api",
			map[string]string{"approval_id": approvalID, "approved_by": claims.UserID}))
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"approval_id": approvalID,
		"status":      "approved",
		"approved_by": claims.UserID,
	})
}

// Reject handles POST /api/v1/deploy/approvals/{id}/reject
func (h *DeployApprovalHandler) Reject(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	approvalID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if !decodeOptionalJSONInto(w, r, &body) { // Reason is optional
		return
	}
	h.mu.Lock()
	req, exists := h.pending[approvalID]
	if !exists {
		h.mu.Unlock()
		writeError(w, http.StatusNotFound, "approval request not found")
		return
	}
	if req.Status != "pending" {
		h.mu.Unlock()
		writeError(w, http.StatusConflict, "approval already processed")
		return
	}
	// Tenant isolation: only the owning tenant can reject its own deploys
	if req.TenantID != claims.TenantID {
		h.mu.Unlock()
		writeError(w, http.StatusForbidden, "not authorized to reject this request")
		return
	}
	now := time.Now()
	req.Status = "rejected"
	req.Reason = body.Reason
	req.ReviewedBy = claims.UserID
	req.ReviewedAt = &now
	h.mu.Unlock()
	if h.events != nil {
		h.events.PublishAsync(r.Context(), core.NewEvent("deploy.rejected", "api",
			map[string]string{"approval_id": approvalID, "reason": body.Reason}))
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"approval_id": approvalID,
		"status":      "rejected",
		"reason":      body.Reason,
	})
}

// ──────────────────── deploy_diff.go ────────────────────
// DeployDiffHandler compares two deployment versions.
type DeployDiffHandler struct {
	store core.Store
}

func NewDeployDiffHandler(store core.Store) *DeployDiffHandler {
	return &DeployDiffHandler{store: store}
}

// Diff handles GET /api/v1/apps/{id}/deployments/diff?from=1&to=2
func (h *DeployDiffHandler) Diff(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	fromVer, _ := strconv.Atoi(r.URL.Query().Get("from"))
	toVer, _ := strconv.Atoi(r.URL.Query().Get("to"))
	if fromVer <= 0 || toVer <= 0 {
		writeError(w, http.StatusBadRequest, "from and to version numbers required")
		return
	}
	deployments, err := h.store.ListDeploymentsByApp(r.Context(), appID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var fromDep, toDep *core.Deployment
	for i := range deployments {
		if deployments[i].Version == fromVer {
			fromDep = &deployments[i]
		}
		if deployments[i].Version == toVer {
			toDep = &deployments[i]
		}
	}
	if fromDep == nil || toDep == nil {
		writeError(w, http.StatusNotFound, "one or both versions not found")
		return
	}
	diff := map[string]any{
		"app_id": appID,
		"from":   fromVer,
		"to":     toVer,
		"changes": map[string]any{
			"image": map[string]string{
				"from": fromDep.Image,
				"to":   toDep.Image,
			},
			"commit": map[string]string{
				"from": fromDep.CommitSHA,
				"to":   toDep.CommitSHA,
			},
			"strategy": map[string]string{
				"from": fromDep.Strategy,
				"to":   toDep.Strategy,
			},
			"triggered_by": map[string]string{
				"from": fromDep.TriggeredBy,
				"to":   toDep.TriggeredBy,
			},
		},
	}
	writeJSON(w, http.StatusOK, diff)
}

// ──────────────────── deploy_freeze.go ────────────────────
// DeployFreezeHandler manages deployment freeze windows.
// When a freeze is active, new deployments are blocked.
type DeployFreezeHandler struct {
	store  core.Store
	events *core.EventBus
	kv     core.KVStorer
}

func NewDeployFreezeHandler(store core.Store, events *core.EventBus, kv core.KVStorer) *DeployFreezeHandler {
	return &DeployFreezeHandler{store: store, events: events, kv: kv}
}

// FreezeWindow defines a time range where deployments are blocked.
type FreezeWindow struct {
	ID       string    `json:"id"`
	Reason   string    `json:"reason"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Active   bool      `json:"active"`
}

// freezeWindowList holds all freeze windows.
type freezeWindowList struct {
	Windows []FreezeWindow `json:"windows"`
}

func activeDeployFreeze(kv core.KVStorer, tenantID string) bool {
	if kv == nil || tenantID == "" {
		return false
	}
	var list freezeWindowList
	if err := kv.Get("deploy_freeze", tenantID, &list); err != nil {
		return false
	}
	now := time.Now()
	for _, fw := range list.Windows {
		if fw.Active && now.After(fw.StartsAt) && now.Before(fw.EndsAt) {
			return true
		}
	}
	return false
}

// Get handles GET /api/v1/deploy/freeze
func (h *DeployFreezeHandler) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var list freezeWindowList
	if err := h.kv.Get("deploy_freeze", claims.TenantID, &list); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "frozen": false})
		return
	}
	// Check if any freeze is currently active
	now := time.Now()
	frozen := false
	active := make([]FreezeWindow, 0)
	for _, fw := range list.Windows {
		if fw.Active && now.After(fw.StartsAt) && now.Before(fw.EndsAt) {
			frozen = true
		}
		if fw.Active {
			active = append(active, fw)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": active, "frozen": frozen})
}

// Create handles POST /api/v1/deploy/freeze
func (h *DeployFreezeHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req struct {
		Reason   string `json:"reason"`
		StartsAt string `json:"starts_at"`
		EndsAt   string `json:"ends_at"`
	}
	if !decodeJSONInto(w, r, &req) {
		return
	}
	startsAt, _ := time.Parse(time.RFC3339, req.StartsAt)
	endsAt, _ := time.Parse(time.RFC3339, req.EndsAt)
	if startsAt.IsZero() {
		startsAt = time.Now()
	}
	if endsAt.IsZero() {
		endsAt = startsAt.Add(24 * time.Hour)
	}
	freeze := FreezeWindow{
		ID:       core.GenerateID(),
		Reason:   req.Reason,
		StartsAt: startsAt,
		EndsAt:   endsAt,
		Active:   true,
	}
	var list freezeWindowList
	_ = h.kv.Get("deploy_freeze", claims.TenantID, &list)
	if len(list.Windows) >= 50 {
		writeError(w, http.StatusConflict, "freeze window limit reached (50)")
		return
	}
	list.Windows = append(list.Windows, freeze)
	if err := h.kv.Set("deploy_freeze", claims.TenantID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save freeze window")
		return
	}
	publishEventAsync(r.Context(), h.events, core.NewEvent("deploy.freeze.created", "api",
		map[string]string{"freeze_id": freeze.ID, "reason": freeze.Reason}))
	writeJSON(w, http.StatusCreated, freeze)
}

// Delete handles DELETE /api/v1/deploy/freeze/{id}
func (h *DeployFreezeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	freezeID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	var list freezeWindowList
	if err := h.kv.Get("deploy_freeze", claims.TenantID, &list); err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for i := range list.Windows {
		if list.Windows[i].ID == freezeID {
			list.Windows[i].Active = false
			break
		}
	}
	if err := h.kv.Set("deploy_freeze", claims.TenantID, list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update freeze window")
		return
	}
	publishEventAsync(r.Context(), h.events, core.NewEvent("deploy.freeze.deleted", "api",
		map[string]string{"freeze_id": freezeID}))
	w.WriteHeader(http.StatusNoContent)
}

// ──────────────────── deploy_notify.go ────────────────────
// DeployNotifyHandler configures per-app deployment notifications.
type DeployNotifyHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewDeployNotifyHandler(store core.Store, kv core.KVStorer) *DeployNotifyHandler {
	return &DeployNotifyHandler{store: store, kv: kv}
}

// DeployNotifyConfig defines what notifications to send on deployment events.
type DeployNotifyConfig struct {
	OnSuccess  []NotifyTarget `json:"on_success"`
	OnFailure  []NotifyTarget `json:"on_failure"`
	OnRollback []NotifyTarget `json:"on_rollback"`
}

// NotifyTarget defines where to send a notification.
type NotifyTarget struct {
	Channel   string `json:"channel"` // slack, discord, telegram, email, webhook
	Recipient string `json:"recipient"`
}

// Get handles GET /api/v1/apps/{id}/deploy-notifications
func (h *DeployNotifyHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg DeployNotifyConfig
	if err := h.kv.Get("deploy_notify", app.ID, &cfg); err != nil {
		writeJSON(w, http.StatusOK, DeployNotifyConfig{})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/deploy-notifications
func (h *DeployNotifyHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg DeployNotifyConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if err := h.kv.Set("deploy_notify", app.ID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save notification config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"app_id": app.ID, "config": cfg, "status": "updated"})
}

// ──────────────────── deploy_preview.go ────────────────────
// DeployPreviewHandler provides deployment dry-run / preview.
type DeployPreviewHandler struct {
	store core.Store
}

func NewDeployPreviewHandler(store core.Store) *DeployPreviewHandler {
	return &DeployPreviewHandler{store: store}
}

// Preview handles POST /api/v1/apps/{id}/deploy/preview
// Shows what would happen without actually deploying.
func (h *DeployPreviewHandler) Preview(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	currentDep, err := h.store.GetLatestDeployment(r.Context(), appID)
	if err != nil {
		slog.Warn("deploy preview: failed to get latest deployment", "app_id", appID, "error", err)
	}
	nextVersion, err := h.store.GetNextDeployVersion(r.Context(), appID)
	if err != nil {
		slog.Warn("deploy preview: failed to get next version", "app_id", appID, "error", err)
	}
	// Detect what would be built
	var detectedType string
	switch app.SourceType {
	case "git":
		detectedType = "would clone and auto-detect"
	case "image":
		detectedType = "image pull: " + app.SourceURL
	default:
		detectedType = app.SourceType
	}
	// Check Dockerfile template availability
	var dockerfile string
	if app.SourceType == "git" {
		dockerfile = "auto-generated based on project type"
	} else if app.Dockerfile != "" {
		dockerfile = "custom: " + app.Dockerfile
	}
	preview := map[string]any{
		"app_id":          appID,
		"app_name":        app.Name,
		"source_type":     app.SourceType,
		"source_url":      app.SourceURL,
		"branch":          app.Branch,
		"current_version": 0,
		"next_version":    nextVersion,
		"strategy":        "recreate",
		"detected_type":   detectedType,
		"dockerfile":      dockerfile,
		"supported_types": []string{
			string(build.TypeNodeJS), string(build.TypeNextJS), string(build.TypeGo),
			string(build.TypePython), string(build.TypeRust), string(build.TypePHP),
			string(build.TypeJava), string(build.TypeDotNet), string(build.TypeRuby),
			string(build.TypeVite), string(build.TypeNuxt), string(build.TypeStatic),
		},
		"dry_run": true,
	}
	if currentDep != nil {
		preview["current_version"] = currentDep.Version
		preview["current_image"] = currentDep.Image
	}
	writeJSON(w, http.StatusOK, preview)
}

// ──────────────────── deploy_trigger.go ────────────────────
// DeployTriggerHandler triggers manual builds and deployments.
type DeployTriggerHandler struct {
	store     core.Store
	runtime   core.ContainerRuntime
	nodes     core.NodeManager
	events    *core.EventBus
	freeze    core.KVStorer
	buildGit  func(ctx context.Context, opts build.BuildOpts, logWriter io.Writer) (*build.BuildResult, error)
	buildRepo string
	buildPush bool
	buildUser string
	buildPass string
	serverCtx context.Context // canceled on graceful shutdown; goroutines should select on this
}
type deployRuntime interface {
	CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error)
	Stop(ctx context.Context, containerID string, timeoutSec int) error
	Remove(ctx context.Context, containerID string, force bool) error
	ListByLabels(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error)
}
type networkEnsurer interface {
	EnsureNetwork(ctx context.Context, name string) error
}

func NewDeployTriggerHandler(ctx context.Context, store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *DeployTriggerHandler {
	h := &DeployTriggerHandler{store: store, runtime: runtime, events: events, serverCtx: ctx}
	h.buildGit = func(ctx context.Context, opts build.BuildOpts, logWriter io.Writer) (*build.BuildResult, error) {
		builder := build.NewBuilder(h.runtime, h.events)
		return builder.Build(ctx, opts, logWriter)
	}
	return h
}

// SetServerContext sets the server-lifetime context used by background goroutines.
func (h *DeployTriggerHandler) SetServerContext(ctx context.Context) { h.serverCtx = ctx }

// SetNodeManager enables deploy placement on connected remote agents.
func (h *DeployTriggerHandler) SetNodeManager(nodes core.NodeManager) { h.nodes = nodes }

// SetBuildImageRegistry configures the registry/repository prefix used for built images.
func (h *DeployTriggerHandler) SetBuildImageRegistry(prefix string) {
	h.buildRepo = strings.Trim(strings.TrimSpace(prefix), "/")
}

// SetBuildImagePush enables pushing built images after docker build.
func (h *DeployTriggerHandler) SetBuildImagePush(enabled bool) { h.buildPush = enabled }

// SetBuildRegistryAuth configures Docker registry credentials for build/push.
func (h *DeployTriggerHandler) SetBuildRegistryAuth(username, password string) {
	h.buildUser = username
	h.buildPass = password
}

// SetDeployFreezeStore enables deploy-freeze enforcement for manual deploys.
func (h *DeployTriggerHandler) SetDeployFreezeStore(kv core.KVStorer) { h.freeze = kv }

// buildDeployLabels creates container labels including HTTP routing labels from domains.
func (h *DeployTriggerHandler) buildDeployLabels(ctx context.Context, app *core.Application, version int) map[string]string {
	labels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         app.ID,
		"monster.app.name":       app.Name,
		"monster.project":        app.ProjectID,
		"monster.tenant":         app.TenantID,
		"monster.deploy.version": fmt.Sprintf("%d", version),
	}
	// Fetch domains for this app and add HTTP routing labels
	domains, err := h.store.ListDomainsByApp(ctx, app.ID, app.TenantID)
	if err == nil && len(domains) > 0 {
		// Get port from app or default to 80
		port := app.Port
		if port <= 0 {
			port = 80
		}
		// Add routing labels for each domain
		for i, domain := range domains {
			routerName := fmt.Sprintf("%s-%d", app.Name, i)
			// Host rule for routing
			labels[fmt.Sprintf("monster.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain.FQDN)
			// Backend port
			labels[fmt.Sprintf("monster.http.services.%s.loadbalancer.server.port", routerName)] = fmt.Sprintf("%d", port)
		}
	}
	return labels
}
func (h *DeployTriggerHandler) publishAsync(ctx context.Context, event core.Event) {
	if h.events != nil {
		h.events.PublishAsync(ctx, event)
	}
}
func (h *DeployTriggerHandler) deployRuntimeForApp(app *core.Application) (deployRuntime, error) {
	if app != nil && app.ServerID != "" && app.ServerID != "local" {
		if h.nodes == nil {
			return nil, fmt.Errorf("server %s is not connected", app.ServerID)
		}
		exec, err := h.nodes.Get(app.ServerID)
		if err != nil {
			return nil, fmt.Errorf("server %s is not connected: %w", app.ServerID, err)
		}
		return exec, nil
	}
	if h.runtime == nil {
		return nil, fmt.Errorf("container runtime not available")
	}
	return h.runtime, nil
}
func isRemoteApp(app *core.Application) bool {
	return app != nil && app.ServerID != "" && app.ServerID != "local"
}
func imageRefHasRegistry(ref string) bool {
	for i, r := range ref {
		if r == '/' {
			first := ref[:i]
			return first == "localhost" || containsAny(first, ".:")
		}
	}
	return false
}
func buildImageTagForRegistry(prefix string, app *core.Application, commitSHA string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" || app == nil {
		return ""
	}
	tag := core.ShortID(commitSHA, 12)
	if tag == "" {
		tag = core.ShortID(core.GenerateID(), 12)
	}
	return prefix + "/" + imageNamePart(app.Name, app.ID) + ":" + tag
}
func imageNamePart(name, fallback string) string {
	source := strings.ToLower(strings.TrimSpace(name))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(fallback))
	}
	var b strings.Builder
	lastSep := false
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSep = false
		case r == '.', r == '_', r == '-', r == ' ':
			if b.Len() > 0 && !lastSep {
				b.WriteByte('-')
				lastSep = true
			}
		}
	}
	part := strings.Trim(b.String(), "-")
	if part == "" {
		part = "app-" + core.ShortID(core.GenerateID(), 8)
	}
	return part
}
func containsAny(s, chars string) bool {
	for _, c := range s {
		for _, want := range chars {
			if c == want {
				return true
			}
		}
	}
	return false
}
func (h *DeployTriggerHandler) cleanupPreviousAppContainers(ctx context.Context, runtime deployRuntime, appID, keepContainerID string) {
	if runtime == nil {
		return
	}
	containers, err := runtime.ListByLabels(ctx, map[string]string{"monster.app.id": appID})
	if err != nil {
		slog.Warn("deploy: failed to list previous app containers", "app_id", appID, "error", err)
		return
	}
	for _, c := range containers {
		if c.ID == "" || c.ID == keepContainerID {
			continue
		}
		if err := runtime.Stop(ctx, c.ID, 30); err != nil {
			slog.Warn("deploy: failed to stop previous container", "app_id", appID, "container_id", c.ID, "error", err)
		}
		if err := runtime.Remove(ctx, c.ID, true); err != nil {
			slog.Warn("deploy: failed to remove previous container", "app_id", appID, "container_id", c.ID, "error", err)
		}
	}
}
func ensureDeployNetwork(ctx context.Context, runtime deployRuntime) error {
	nr, ok := runtime.(networkEnsurer)
	if !ok {
		return nil
	}
	return nr.EnsureNetwork(ctx, "monster-network")
}

// SubscribeWebhookDeploys wires inbound git webhooks to the same build+deploy
// path used by manual git deployments. Inbound webhook secrets are stored under
// the app ID, so WebhookID is treated as the target app ID.
func (h *DeployTriggerHandler) SubscribeWebhookDeploys() {
	if h.events == nil {
		return
	}
	h.events.SubscribeAsync(core.EventWebhookReceived, func(_ context.Context, event core.Event) error {
		data, ok := event.Data.(core.WebhookEventData)
		if !ok || data.WebhookID == "" {
			return nil
		}
		ctx := h.serverCtx
		app, err := h.store.GetApp(ctx, data.WebhookID)
		if err != nil {
			slog.Warn("webhook deploy: app lookup failed", "webhook_id", data.WebhookID, "error", err)
			return nil
		}
		if app.SourceType != "git" {
			slog.Info("webhook deploy: ignoring non-git app", "app_id", app.ID, "source_type", app.SourceType)
			return nil
		}
		if app.Branch != "" && data.Branch != "" && app.Branch != data.Branch {
			slog.Info("webhook deploy: branch ignored", "app_id", app.ID, "app_branch", app.Branch, "webhook_branch", data.Branch)
			return nil
		}
		if activeDeployFreeze(h.freeze, app.TenantID) {
			h.publishAsync(ctx, core.NewEvent(core.EventDeployFailed, "webhook_deploy", map[string]string{
				"app_id": app.ID,
				"error":  "deployments are currently frozen",
			}))
			return nil
		}
		if err := h.deployGitApp(ctx, app, "webhook", data.CommitSHA, io.Discard); err != nil {
			slog.Error("webhook deploy failed", "app_id", app.ID, "error", err)
		}
		return nil
	})
}
func (h *DeployTriggerHandler) deployGitApp(ctx context.Context, app *core.Application, triggeredBy, commitSHA string, logWriter io.Writer) error {
	if h.runtime == nil {
		err := fmt.Errorf("container runtime not available")
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, app.TenantID, err.Error())
		return err
	}
	if err := h.store.UpdateAppStatus(ctx, app.ID, "building", app.TenantID); err != nil {
		slog.Error("deploy: failed to update app status", "app_id", app.ID, "error", err)
	}
	buildOpts := build.BuildOpts{
		AppID:      app.ID,
		AppName:    app.Name,
		SourceURL:  app.SourceURL,
		Branch:     app.Branch,
		CommitSHA:  commitSHA,
		Dockerfile: app.Dockerfile,
	}
	if isRemoteApp(app) && h.buildRepo != "" {
		buildOpts.ImageTag = buildImageTagForRegistry(h.buildRepo, app, commitSHA)
		buildOpts.Push = h.buildPush
		buildOpts.RegistryUsername = h.buildUser
		buildOpts.RegistryPassword = h.buildPass
	}
	result, err := h.buildGit(ctx, buildOpts, logWriter)
	if err != nil {
		h.failAppAndPublish(ctx, core.EventBuildFailed, app.ID, app.TenantID, err.Error())
		return err
	}
	if isRemoteApp(app) && !imageRefHasRegistry(result.ImageTag) {
		err := fmt.Errorf("remote git deploy requires a registry-qualified image tag for %q; configure build push/pull before targeting server %s", result.ImageTag, app.ServerID)
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, app.TenantID, err.Error())
		return err
	}
	if sErr := h.store.UpdateAppStatus(ctx, app.ID, "deploying", app.TenantID); sErr != nil {
		slog.Error("deploy: failed to update app status", "app_id", app.ID, "error", sErr)
	}
	deployRT, err := h.deployRuntimeForApp(app)
	if err != nil {
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, app.TenantID, err.Error())
		return err
	}
	// Reserve the version AND insert the deployment row atomically up front
	// (RACE-002b). Previously the version was allocated here but the row was
	// only written after the container started, leaving a wide window in which
	// a concurrent deploy of the same app read the same MAX(version). The row
	// is created in "deploying" state and finalized (container id + status)
	// once the container is running.
	dep := &core.Deployment{
		AppID:       app.ID,
		Image:       result.ImageTag,
		CommitSHA:   result.CommitSHA,
		Status:      "deploying",
		TriggeredBy: triggeredBy,
		Strategy:    "recreate",
	}
	if err := h.store.CreateDeploymentAtomicVersion(ctx, dep); err != nil {
		slog.Error("deploy: failed to reserve deployment version", "app_id", app.ID, "error", err)
		// Row not reserved: use failAppAndPublish (not failReserved).
		h.failAppAndPublish(ctx, core.EventDeployFailed, app.ID, app.TenantID, err.Error())
		return err
	}
	version := dep.Version
	labels := h.buildDeployLabels(ctx, app, version)
	containerName := fmt.Sprintf("dm-%s-%d", app.ID, version)
	if err := ensureDeployNetwork(ctx, deployRT); err != nil {
		h.failReserved(ctx, app.ID, app.TenantID, dep, err.Error())
		return err
	}
	containerID, err := deployRT.CreateAndStart(ctx, core.ContainerOpts{
		Name:          containerName,
		Image:         result.ImageTag,
		Labels:        labels,
		Network:       "monster-network",
		RestartPolicy: "unless-stopped",
	})
	if err != nil {
		h.failReserved(ctx, app.ID, app.TenantID, dep, err.Error())
		return err
	}
	h.cleanupPreviousAppContainers(ctx, deployRT, app.ID, containerID)
	// Finalize the reserved deployment row now that the container is running.
	dep.ContainerID = containerID
	dep.Status = "running"
	if err := h.store.UpdateDeployment(ctx, dep); err != nil {
		slog.Error("deploy: failed to update deployment", "app_id", app.ID, "error", err)
	}
	if err := h.store.UpdateAppStatus(ctx, app.ID, "running", app.TenantID); err != nil {
		slog.Error("deploy: failed to update app status", "app_id", app.ID, "error", err)
	}
	h.publishAsync(ctx, core.NewEvent(core.EventAppDeployed, "deploy_trigger", core.DeployEventData{
		AppID:        app.ID,
		DeploymentID: dep.ID,
		Version:      version,
		Image:        result.ImageTag,
		ContainerID:  containerID,
		Strategy:     "recreate",
		CommitSHA:    result.CommitSHA,
	}))
	return nil
}

// failReservedDeployment marks both the app and the reserved deployment row
// as failed when a deploy aborts after the row was reserved (RACE-002b). It
// keeps the deployments table truthful so the UI and the restart-storm reclaim
// sweep don't see a row stuck in "deploying".
func (h *DeployTriggerHandler) failReservedDeployment(ctx context.Context, appID, tenantID string, dep *core.Deployment) {
	if sErr := h.store.UpdateAppStatus(ctx, appID, "failed", tenantID); sErr != nil {
		slog.Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
	}
	if dep != nil && dep.ID != "" {
		now := time.Now()
		dep.Status = "failed"
		dep.FinishedAt = &now
		if uErr := h.store.UpdateDeployment(ctx, dep); uErr != nil {
			slog.Error("deploy: failed to mark deployment failed", "app_id", appID, "error", uErr)
		}
	}
}

// failReserved marks the app and the reserved deployment row as failed, then
// emits EventDeployFailed. Use when a deploy aborts after CreateDeploymentAtomicVersion
// succeeded (RACE-002b path).
//
// P1-7: Merges the failReservedDeployment+publishAsync(EventDeployFailed) pair
// that was copy-pasted at three sites in deployGitApp.
func (h *DeployTriggerHandler) failReserved(ctx context.Context, appID, tenantID string, dep *core.Deployment, errMsg string) {
	h.failReservedDeployment(ctx, appID, tenantID, dep)
	h.publishDeployFailed(ctx, "deploy_trigger", appID, errMsg)
}

// failApp marks the app as failed. Use when the deployment row has not yet been reserved.
// P1-7: Extracted from 7× inline copy-paste blocks in deployGitApp.
func (h *DeployTriggerHandler) failApp(ctx context.Context, appID, tenantID string) {
	if sErr := h.store.UpdateAppStatus(ctx, appID, "failed", tenantID); sErr != nil {
		slog.Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
	}
}

// failAppAndPublish marks the app as failed and emits the appropriate event.
// P1-7: Consolidates the failApp+publishAsync(EventDeployFailed) pair used at 4 sites.
func (h *DeployTriggerHandler) failAppAndPublish(ctx context.Context, eventType string, appID, tenantID, errMsg string) {
	h.failApp(ctx, appID, tenantID)
	h.publishAsync(ctx, core.NewEvent(eventType, "deploy_trigger", map[string]string{
		"app_id": appID,
		"error":  errMsg,
	}))
}

// publishDeployFailed emits an EventDeployFailed event.
// P1-7: Centralizes the publishAsync(EventDeployFailed) call that was copy-pasted 7×.
func (h *DeployTriggerHandler) publishDeployFailed(ctx context.Context, source, appID, errMsg string) {
	h.publishAsync(ctx, core.NewEvent(core.EventDeployFailed, source, map[string]string{
		"app_id": appID,
		"error":  errMsg,
	}))
}

// TriggerDeploy handles POST /api/v1/apps/{id}/deploy
// Triggers a manual build+deploy for a git-sourced app.
func (h *DeployTriggerHandler) TriggerDeploy(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	if activeDeployFreeze(h.freeze, app.TenantID) {
		writeError(w, http.StatusLocked, "deployments are currently frozen")
		return
	}
	if app.SourceType == "image" {
		// For image-type apps, just redeploy the same image
		if err := h.store.UpdateAppStatus(r.Context(), appID, "deploying", app.TenantID); err != nil {
			slog.Error("deploy: failed to update app status", "app_id", appID, "error", err)
		}
		dep := &core.Deployment{
			AppID:       appID,
			Image:       app.SourceURL,
			Status:      "deploying",
			TriggeredBy: "manual",
			Strategy:    "recreate",
		}
		// Reserve the version and insert the row atomically (RACE-002b) so a
		// concurrent deploy of the same app cannot claim the same version.
		if err := h.store.CreateDeploymentAtomicVersion(r.Context(), dep); err != nil {
			slog.Error("deploy: failed to create deployment", "app_id", appID, "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		version := dep.Version
		deployRT, err := h.deployRuntimeForApp(app)
		if err == nil {
			// Build labels with HTTP routing from domains
			labels := h.buildDeployLabels(r.Context(), app, version)
			containerName := fmt.Sprintf("dm-%s-%d", app.ID, version)
			if err := ensureDeployNetwork(r.Context(), deployRT); err != nil {
				if sErr := h.store.UpdateAppStatus(r.Context(), appID, "failed", app.TenantID); sErr != nil {
					ctxLogger(r.Context()).Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
				}
				internalErrorCtx(r.Context(), w, "deploy failed", err)
				return
			}
			containerID, err := deployRT.CreateAndStart(r.Context(), core.ContainerOpts{
				Name:          containerName,
				Image:         app.SourceURL,
				Labels:        labels,
				Network:       "monster-network",
				RestartPolicy: "unless-stopped",
			})
			if err != nil {
				if sErr := h.store.UpdateAppStatus(r.Context(), appID, "failed", app.TenantID); sErr != nil {
					ctxLogger(r.Context()).Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
				}
				internalErrorCtx(r.Context(), w, "deploy failed", err)
				return
			}
			dep.ContainerID = containerID
			if err := h.store.UpdateDeployment(r.Context(), dep); err != nil {
				slog.Error("deploy: failed to update deployment container", "app_id", appID, "error", err)
			}
			h.cleanupPreviousAppContainers(r.Context(), deployRT, app.ID, containerID)
		} else if app.ServerID != "" {
			if sErr := h.store.UpdateAppStatus(r.Context(), appID, "failed", app.TenantID); sErr != nil {
				ctxLogger(r.Context()).Error("deploy: failed to update app status", "app_id", appID, "error", sErr)
			}
			internalErrorCtx(r.Context(), w, "deploy failed", err)
			return
		}
		if err := h.store.UpdateAppStatus(r.Context(), appID, "running", app.TenantID); err != nil {
			slog.Error("deploy: failed to update app status", "app_id", appID, "error", err)
		}
		h.publishAsync(r.Context(), core.NewEvent(core.EventAppDeployed, "deploy_trigger", core.DeployEventData{
			AppID:        appID,
			DeploymentID: dep.ID,
			Version:      version,
			Image:        app.SourceURL,
			ContainerID:  dep.ContainerID,
			Strategy:     "recreate",
		}))
		writeJSON(w, http.StatusOK, map[string]any{
			"deployment": dep,
			"status":     "deployed",
		})
		return
	}
	// Use server-scoped context — outlives the request but cancels on shutdown
	safeGo(func() {
		_ = h.deployGitApp(h.serverCtx, app, "manual", "", io.Discard)
	}, func(recovered any) {
		h.store.UpdateAppStatus(h.serverCtx, appID, "failed", app.TenantID)
		h.publishAsync(h.serverCtx, core.NewEvent(core.EventDeployFailed, "deploy_trigger", map[string]string{
			"app_id": appID,
			"error":  fmt.Sprintf("panic: %v", recovered),
		}))
	})
	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "building",
		"message": "build and deploy pipeline triggered",
	})
}

// ──────────────────── deployments.go ────────────────────
// DeploymentHandler handles deployment endpoints.
type DeploymentHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewDeploymentHandler(store core.Store, events *core.EventBus) *DeploymentHandler {
	return &DeploymentHandler{store: store, events: events}
}

// ListByApp handles GET /api/v1/apps/{id}/deployments
func (h *DeploymentHandler) ListByApp(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	pg := parsePagination(r)
	deployments, err := h.store.ListDeploymentsByApp(r.Context(), app.ID, pg.PerPage)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     deployments,
		"total":    len(deployments),
		"per_page": pg.PerPage,
	})
}

// GetLatest handles GET /api/v1/apps/{id}/deployments/latest
func (h *DeploymentHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	dep, err := h.store.GetLatestDeployment(r.Context(), app.ID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "no deployments found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, dep)
}

// ──────────────────── rollback.go ────────────────────
// RollbackHandler manages deployment rollback operations.
type RollbackHandler struct {
	store  core.Store
	engine *deploy.RollbackEngine
}

func NewRollbackHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *RollbackHandler {
	return &RollbackHandler{
		store:  store,
		engine: deploy.NewRollbackEngine(store, runtime, events),
	}
}

type rollbackRequest struct {
	Version int `json:"version"`
}

// Rollback handles POST /api/v1/apps/{id}/rollback
func (h *RollbackHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var req rollbackRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Version <= 0 {
		writeError(w, http.StatusBadRequest, "version must be positive")
		return
	}
	dep, err := h.engine.Rollback(r.Context(), app.ID, req.Version)
	if err != nil {
		internalErrorCtx(r.Context(), w, "rollback failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"deployment":     dep,
		"rolled_back_to": req.Version,
	})
}

// ListVersions handles GET /api/v1/apps/{id}/versions
func (h *RollbackHandler) ListVersions(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	versions, err := h.engine.ListVersions(r.Context(), app.ID, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list versions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": versions})
}

// ──────────────────── commit_rollback.go ────────────────────
// CommitRollbackHandler handles rollback to a specific git commit.
type CommitRollbackHandler struct {
	store  core.Store
	events *core.EventBus
	engine *deploy.RollbackEngine
}

func NewCommitRollbackHandler(store core.Store, runtime core.ContainerRuntime, events *core.EventBus) *CommitRollbackHandler {
	return &CommitRollbackHandler{
		store:  store,
		events: events,
		engine: deploy.NewRollbackEngine(store, runtime, events),
	}
}

type commitRollbackRequest struct {
	CommitSHA string `json:"commit_sha"`
}

// RollbackToCommit handles POST /api/v1/apps/{id}/rollback-to-commit
// Finds the deployment that matches the commit and redeploys it.
func (h *CommitRollbackHandler) RollbackToCommit(w http.ResponseWriter, r *http.Request) {
	tApp := requireTenantApp(w, r, h.store)
	if tApp == nil {
		return
	}
	appID := tApp.ID
	var req commitRollbackRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.CommitSHA == "" {
		writeError(w, http.StatusBadRequest, "commit_sha required")
		return
	}
	// Find deployment with matching commit
	deployments, err := h.store.ListDeploymentsByApp(r.Context(), appID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var target *core.Deployment
	for i := range deployments {
		if deployments[i].CommitSHA == req.CommitSHA {
			target = &deployments[i]
			break
		}
		// Partial match (first 7+ chars)
		if len(req.CommitSHA) >= 7 && len(deployments[i].CommitSHA) >= len(req.CommitSHA) &&
			deployments[i].CommitSHA[:len(req.CommitSHA)] == req.CommitSHA {
			target = &deployments[i]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "no deployment found for commit "+req.CommitSHA)
		return
	}
	// Trigger the actual rollback via the deploy engine
	dep, err := h.engine.Rollback(r.Context(), appID, target.Version)
	if err != nil {
		internalErrorCtx(r.Context(), w, "rollback failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":           appID,
		"commit":           target.CommitSHA,
		"version":          target.Version,
		"image":            target.Image,
		"deployment":       dep,
		"rollback_version": target.Version,
	})
}
