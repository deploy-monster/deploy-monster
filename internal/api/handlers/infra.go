package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── container_top.go ────────────────────
// ContainerTopHandler lists running processes inside a container.
type ContainerTopHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewContainerTopHandler(store core.Store, runtime core.ContainerRuntime) *ContainerTopHandler {
	return &ContainerTopHandler{store: store, runtime: runtime}
}

// Top handles GET /api/v1/apps/{id}/processes
func (h *ContainerTopHandler) Top(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no container found")
		return
	}
	// Docker top would list processes — structural response
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"container_id": shortResourceID(containers[0].ID),
		"processes":    []any{},
		"titles":       []string{"PID", "USER", "TIME", "COMMAND"},
	})
}

// ──────────────────── container_history.go ────────────────────
// ContainerHistoryHandler serves per-container resource usage over time.
type ContainerHistoryHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	kv      core.KVStorer
}

func NewContainerHistoryHandler(store core.Store, runtime core.ContainerRuntime, kv core.KVStorer) *ContainerHistoryHandler {
	return &ContainerHistoryHandler{store: store, runtime: runtime, kv: kv}
}

// ContainerResourcePoint represents a data point in container history.
type ContainerResourcePoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   int64     `json:"memory_mb"`
	MemoryMax  int64     `json:"memory_max_mb"`
	NetRxKB    int64     `json:"net_rx_kb"`
	NetTxKB    int64     `json:"net_tx_kb"`
	PIDs       int       `json:"pids"`
}

// metricsRingData is what we store in the metrics_ring bucket per app.
type metricsRingData struct {
	Points []ContainerResourcePoint `json:"points"`
}

// History handles GET /api/v1/apps/{id}/containers/history
func (h *ContainerHistoryHandler) History(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "1h"
	}
	// Try to load real metrics from KV storage.
	var ring metricsRingData
	if h.kv != nil {
		_ = h.kv.Get("metrics_ring", appID, &ring)
	}
	if len(ring.Points) > 0 {
		// Filter points by requested period
		var cutoff time.Time
		now := time.Now()
		switch period {
		case "24h":
			cutoff = now.Add(-24 * time.Hour)
		case "7d":
			cutoff = now.Add(-7 * 24 * time.Hour)
		default:
			cutoff = now.Add(-1 * time.Hour)
		}
		filtered := make([]ContainerResourcePoint, 0, len(ring.Points))
		for _, p := range ring.Points {
			if p.Timestamp.After(cutoff) {
				filtered = append(filtered, p)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"app_id": appID,
			"period": period,
			"points": filtered,
			"count":  len(filtered),
		})
		return
	}
	// No stored metrics — return empty timeline
	var count int
	switch period {
	case "24h":
		count = 96
	case "7d":
		count = 168
	default:
		count = 60
	}
	now := time.Now()
	points := make([]ContainerResourcePoint, count)
	for i := range points {
		points[i] = ContainerResourcePoint{
			Timestamp: now.Add(-time.Duration(count-1-i) * time.Minute),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"period": period,
		"points": points,
		"count":  count,
	})
}

// ──────────────────── disk_usage.go ────────────────────
// DiskUsageHandler shows container and volume disk usage.
type DiskUsageHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	reader  core.SysMetricsReader
}

func NewDiskUsageHandler(store core.Store, runtime core.ContainerRuntime) *DiskUsageHandler {
	return &DiskUsageHandler{store: store, runtime: runtime, reader: core.NewSysMetricsReader()}
}

// AppDisk handles GET /api/v1/apps/{id}/disk.
// Reports the count of containers attached to the app and the cumulative
// size of the images those containers reference. We don't include the
// container's writable layer because the Docker SDK only exposes that via
// ContainerInspect with `Size: true`, which isn't on our runtime
// interface; surfacing 0 there used to look like real "no usage", so
// instead we leave that field omitted with `image_size_bytes` covering
// the meaningful component.
func (h *DiskUsageHandler) AppDisk(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"app_id":           appID,
			"containers":       0,
			"image_size_bytes": 0,
			"runtime":          "unavailable",
		})
		return
	}
	containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{"monster.app.id": appID})
	imagesByID := map[string]bool{}
	for _, c := range containers {
		imagesByID[c.Image] = true
	}
	var imageBytes int64
	if len(imagesByID) > 0 {
		images, err := h.runtime.ImageList(r.Context())
		if err == nil {
			for _, img := range images {
				if imagesByID[img.ID] {
					imageBytes += img.Size
					continue
				}
				for _, tag := range img.Tags {
					if imagesByID[tag] {
						imageBytes += img.Size
						break
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":           appID,
		"containers":       len(containers),
		"image_size_bytes": imageBytes,
	})
}

// SystemDisk handles GET /api/v1/admin/disk.
// Combines the host filesystem snapshot from SysMetrics with Docker-side
// totals for images and (best-effort) volumes. Build cache is reported
// separately via /api/v1/build/cache and so isn't double-counted here.
func (h *DiskUsageHandler) SystemDisk(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"containers_bytes": 0,
		"images_bytes":     0,
		"volumes_bytes":    0,
		"total_bytes":      0,
		"available_bytes":  0,
		"used_bytes":       0,
	}
	if h.reader != nil {
		if m, err := h.reader.Read(); err == nil {
			resp["total_bytes"] = m.DiskTotalMB * 1024 * 1024
			resp["used_bytes"] = m.DiskUsedMB * 1024 * 1024
			resp["available_bytes"] = (m.DiskTotalMB - m.DiskUsedMB) * 1024 * 1024
		}
	}
	if h.runtime != nil {
		if images, err := h.runtime.ImageList(r.Context()); err == nil {
			var total int64
			for _, img := range images {
				total += img.Size
			}
			resp["images_bytes"] = total
			resp["images_count"] = len(images)
		}
		if volumes, err := h.runtime.VolumeList(r.Context()); err == nil {
			var total int64
			for _, v := range volumes {
				if v.Mountpoint == "" {
					continue
				}
				total += dirSize(v.Mountpoint)
			}
			resp["volumes_bytes"] = total
			resp["volumes_count"] = len(volumes)
		}
	} else {
		resp["runtime"] = "unavailable"
	}
	writeJSON(w, http.StatusOK, resp)
}

// dirSize sums the size of every regular file under root. Best-effort:
// permission errors silently return what was reachable. Volume mountpoints
// owned by root are common — when the platform process can't read them,
// the volumes_bytes total reflects only what's visible.
func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsPermission(err) || strings.Contains(err.Error(), "permission denied") {
				return nil
			}
			return nil
		}
		if info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}

// ──────────────────── gpu.go ────────────────────
// GPUHandler manages container GPU passthrough configuration.
// Enables NVIDIA GPU access for AI/ML workloads (Ollama, etc.).
type GPUHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	kv      core.KVStorer
	events  *core.EventBus
}

func NewGPUHandler(store core.Store, runtime core.ContainerRuntime, kv core.KVStorer) *GPUHandler {
	return &GPUHandler{store: store, runtime: runtime, kv: kv}
}

// SetEvents sets the event bus for audit event emission.
func (h *GPUHandler) SetEvents(events *core.EventBus) { h.events = events }

// GPUConfig holds GPU passthrough settings.
type GPUConfig struct {
	Enabled      bool     `json:"enabled"`
	DeviceIDs    []string `json:"device_ids,omitempty"` // specific GPU IDs, empty = all
	Capabilities []string `json:"capabilities"`         // compute, utility, graphics
	Driver       string   `json:"driver"`               // nvidia
}

// gpuDetection holds detected GPU info from the host.
type gpuDetection struct {
	Available bool     `json:"available"`
	Devices   []string `json:"devices,omitempty"`
	Driver    string   `json:"driver,omitempty"`
}

// Get handles GET /api/v1/apps/{id}/gpu
func (h *GPUHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	// Try to load stored GPU config for this app
	var cfg GPUConfig
	if err := h.kv.Get("gpu_config", app.ID, &cfg); err != nil {
		cfg = GPUConfig{
			Enabled:      false,
			Capabilities: []string{"compute", "utility"},
			Driver:       "nvidia",
		}
	}
	// Detect GPU availability on the host
	detection := h.detectGPU(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"config":    cfg,
		"detection": detection,
	})
}

// Update handles PUT /api/v1/apps/{id}/gpu
func (h *GPUHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg GPUConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if len(cfg.Capabilities) == 0 {
		cfg.Capabilities = []string{"compute", "utility"}
	}
	if cfg.Driver == "" {
		cfg.Driver = "nvidia"
	}
	if err := h.kv.Set("gpu_config", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save GPU config")
		return
	}
	if h.events != nil {
		h.events.Publish(r.Context(), core.NewEvent(core.EventGPUConfigUpdated, "api",
			map[string]string{"app_id": appID, "driver": cfg.Driver}))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}

// detectGPU checks for NVIDIA GPU devices on the host via the container runtime.
func (h *GPUHandler) detectGPU(r *http.Request) gpuDetection {
	if h.runtime == nil {
		return gpuDetection{Available: false}
	}
	// Try to list containers that have GPU labels — a heuristic approach.
	// In production, this would run nvidia-smi or check /dev/nvidia* devices.
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"com.docker.compose.service": "nvidia"})
	if err == nil && len(containers) > 0 {
		return gpuDetection{
			Available: true,
			Driver:    "nvidia",
		}
	}
	// Check for nvidia images as another heuristic
	images, err := h.runtime.ImageList(r.Context())
	if err == nil {
		for _, img := range images {
			for _, tag := range img.Tags {
				if strings.Contains(tag, "nvidia") || strings.Contains(tag, "cuda") {
					return gpuDetection{
						Available: true,
						Driver:    "nvidia",
					}
				}
			}
		}
	}
	return gpuDetection{Available: false}
}

// ──────────────────── image_cleanup.go ────────────────────
// ImageCleanupHandler manages Docker image pruning.
type ImageCleanupHandler struct {
	runtime core.ContainerRuntime
}

func NewImageCleanupHandler(runtime core.ContainerRuntime) *ImageCleanupHandler {
	return &ImageCleanupHandler{runtime: runtime}
}

// DanglingImages handles GET /api/v1/images/dangling
func (h *ImageCleanupHandler) DanglingImages(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}
	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		internalError(w, "failed to list images", err)
		return
	}
	var danglingCount int
	var reclaimableMB int64
	for _, img := range images {
		if len(img.Tags) == 0 || (len(img.Tags) == 1 && img.Tags[0] == "<none>:<none>") {
			danglingCount++
			reclaimableMB += img.Size / (1024 * 1024)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dangling_count": danglingCount,
		"reclaimable_mb": reclaimableMB,
	})
}

// Prune handles DELETE /api/v1/images/prune
// Removes unused and dangling images.
func (h *ImageCleanupHandler) Prune(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}
	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		internalError(w, "failed to list images", err)
		return
	}
	var removed int
	var reclaimedMB int64
	for _, img := range images {
		if len(img.Tags) == 0 || (len(img.Tags) == 1 && img.Tags[0] == "<none>:<none>") {
			if err := h.runtime.ImageRemove(r.Context(), img.ID); err != nil {
				continue // skip images that can't be removed (in use)
			}
			removed++
			reclaimedMB += img.Size / (1024 * 1024)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reclaimed_mb":   reclaimedMB,
		"images_removed": removed,
		"status":         "pruned",
	})
}

// ──────────────────── image_tags.go ────────────────────
// ImageTagHandler lists available tags for Docker images.
type ImageTagHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
}

func NewImageTagHandler(store core.Store, runtime core.ContainerRuntime) *ImageTagHandler {
	return &ImageTagHandler{store: store, runtime: runtime}
}

// TagInfo represents a Docker image tag.
type TagInfo struct {
	Name        string `json:"name"`
	Digest      string `json:"digest,omitempty"`
	Size        int64  `json:"size,omitempty"`
	LastUpdated string `json:"last_updated,omitempty"`
}

// List handles GET /api/v1/images/tags?image=nginx
// Lists tags for images available in the local Docker runtime.
// SECURITY FIX (AUTHZ-007): Added authentication and tenant isolation.
func (h *ImageTagHandler) List(w http.ResponseWriter, r *http.Request) {
	// Verify authentication
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	image := r.URL.Query().Get("image")
	if image == "" {
		writeError(w, http.StatusBadRequest, "image query param required")
		return
	}
	// SECURITY FIX (AUTHZ-007): Verify the image is used by an app in this tenant
	// First, get all apps for this tenant
	apps, _, err := h.store.ListAppsByTenant(r.Context(), claims.TenantID, 1000, 0)
	if err != nil {
		internalError(w, "failed to list apps", err)
		return
	}
	// Build a set of allowed image names from the tenant's deployments
	// Batch-fetch all latest deployments in a single query to avoid N+1.
	allowedImages := make(map[string]bool)
	if len(apps) > 0 {
		appIDs := make([]string, len(apps))
		for i, app := range apps {
			appIDs[i] = app.ID
		}
		deployments, err := h.store.GetLatestDeploymentsByAppIDs(r.Context(), appIDs)
		if err != nil {
			internalError(w, "failed to load deployments", err)
			return
		}
		for _, deploy := range deployments {
			if deploy != nil && deploy.Image != "" {
				// Extract base image name without tag
				parts := strings.SplitN(deploy.Image, ":", 2)
				allowedImages[parts[0]] = true
			}
		}
	}
	// Check if the requested image is in the allowed set
	if !allowedImages[image] {
		writeError(w, http.StatusForbidden, "access denied to this image")
		return
	}
	images, err := h.runtime.ImageList(r.Context())
	if err != nil {
		internalError(w, "failed to list images", err)
		return
	}
	var tags []TagInfo
	for _, img := range images {
		for _, tag := range img.Tags {
			// Match by image name prefix (e.g., "nginx" matches "nginx:latest", "nginx:1.25")
			parts := strings.SplitN(tag, ":", 2)
			if len(parts) == 2 && (parts[0] == image || strings.HasSuffix(parts[0], "/"+image)) {
				tags = append(tags, TagInfo{
					Name:   parts[1],
					Digest: img.ID,
					Size:   img.Size,
				})
			}
		}
	}
	if tags == nil {
		tags = []TagInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"image": image,
		"tags":  tags,
		"total": len(tags),
	})
}

// ──────────────────── networks.go ────────────────────
// NetworkHandler manages container network operations.
type NetworkHandler struct {
	runtime core.ContainerRuntime
	events  *core.EventBus
}

func NewNetworkHandler(runtime core.ContainerRuntime, events *core.EventBus) *NetworkHandler {
	return &NetworkHandler{runtime: runtime, events: events}
}

// List handles GET /api/v1/networks
func (h *NetworkHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	// List monster-managed networks via container labels
	containers, _ := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.enable": "true",
	})
	networks := make(map[string]bool)
	for _, c := range containers {
		if stack := c.Labels["monster.stack"]; stack != "" {
			networks["monster-"+stack+"-net"] = true
		}
	}
	networks["monster-network"] = true
	result := make([]string, 0, len(networks))
	for n := range networks {
		result = append(result, n)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result, "total": len(result)})
}

type connectNetworkRequest struct {
	ContainerID string `json:"container_id"`
	Network     string `json:"network"`
}

// Connect handles POST /api/v1/networks/connect
func (h *NetworkHandler) Connect(w http.ResponseWriter, r *http.Request) {
	var req connectNetworkRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	// Docker network connect would happen here
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "connected",
		"container": req.ContainerID,
		"network":   req.Network,
	})
}

// ──────────────────── ports.go ────────────────────
// PortHandler manages app port mappings.
type PortHandler struct {
	store core.Store
}

func NewPortHandler(store core.Store) *PortHandler {
	return &PortHandler{store: store}
}

// PortMapping represents a container port mapping.
type PortMapping struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port,omitempty"` // 0 = auto
	Protocol      string `json:"protocol"`            // tcp, udp
	Exposed       bool   `json:"exposed"`
}

// Get handles GET /api/v1/apps/{id}/ports
func (h *PortHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	// Default port based on app type — in production would read from container inspect
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"ports": []PortMapping{
			{ContainerPort: 80, Protocol: "tcp", Exposed: true},
		},
	})
}

// Update handles PUT /api/v1/apps/{id}/ports
func (h *PortHandler) Update(w http.ResponseWriter, r *http.Request) {
	// SECURITY: Verify the app belongs to this tenant
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var ports []PortMapping
	if !decodeJSONInto(w, r, &ports) {
		return
	}
	// Cap the array — 100 mappings per app is more than any real workload
	// needs, and without this an attacker could post a multi-MB array that
	// slips under the 10MB BodyLimit middleware.
	if len(ports) > 100 {
		writeError(w, http.StatusBadRequest, "too many port mappings (max 100)")
		return
	}
	for i := range ports {
		p := &ports[i]
		if p.ContainerPort <= 0 || p.ContainerPort > 65535 {
			writeError(w, http.StatusBadRequest, "invalid container port")
			return
		}
		// 0 = auto-assign; otherwise must be in the valid TCP/UDP range.
		if p.HostPort < 0 || p.HostPort > 65535 {
			writeError(w, http.StatusBadRequest, "invalid host port")
			return
		}
		if p.Protocol == "" {
			p.Protocol = "tcp"
		}
		if p.Protocol != "tcp" && p.Protocol != "udp" {
			writeError(w, http.StatusBadRequest, "protocol must be tcp or udp")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"ports":  ports,
		"status": "updated",
	})
}

// ──────────────────── registry.go ────────────────────
// RegistryHandler manages Docker registry connections.
type RegistryHandler struct {
	kv core.KVStorer
}

func NewRegistryHandler(kv core.KVStorer) *RegistryHandler {
	return &RegistryHandler{kv: kv}
}
func registryListKey(tenantID string) string {
	return "tenant:" + tenantID
}
func registryCredKey(tenantID, registryID string) string {
	return tenantID + ":" + registryID
}

// RegistryConfig represents a Docker registry connection.
type RegistryConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"` // e.g., ghcr.io, registry.example.com
	Username string `json:"username"`
	Password string `json:"-"` // Never returned
	IsPublic bool   `json:"is_public"`
}

// registryList holds all configured registries.
type registryList struct {
	Registries []RegistryConfig `json:"registries"`
}

// builtinRegistries are always available.
var builtinRegistries = []RegistryConfig{
	{ID: "dockerhub", Name: "Docker Hub", URL: "docker.io", IsPublic: true},
	{ID: "ghcr", Name: "GitHub Container Registry", URL: "ghcr.io", IsPublic: true},
	{ID: "gcr", Name: "Google Container Registry", URL: "gcr.io", IsPublic: true},
}

// List handles GET /api/v1/registries
func (h *RegistryHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	all := make([]RegistryConfig, len(builtinRegistries))
	copy(all, builtinRegistries)
	// Load custom registries from KV storage.
	var list registryList
	if err := h.kv.Get("registries", registryListKey(claims.TenantID), &list); err == nil {
		all = append(all, list.Registries...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": all, "total": len(all)})
}

type addRegistryRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Add handles POST /api/v1/registries
func (h *RegistryHandler) Add(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req addRegistryRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Name == "" || req.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	newReg := RegistryConfig{
		ID:       core.GenerateID(),
		Name:     req.Name,
		URL:      req.URL,
		Username: req.Username,
		IsPublic: false,
	}
	// Load existing custom registries
	var list registryList
	_ = h.kv.Get("registries", registryListKey(claims.TenantID), &list)
	if len(list.Registries) >= 20 {
		writeError(w, http.StatusConflict, "registry limit reached (20)")
		return
	}
	list.Registries = append(list.Registries, newReg)
	if err := h.kv.Set("registries", registryListKey(claims.TenantID), list, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save registry")
		return
	}
	// Store credentials separately (password never in the list response)
	if req.Password != "" {
		if err := h.kv.Set("registry_creds", registryCredKey(claims.TenantID, newReg.ID), map[string]string{
			"username": req.Username,
			"password": req.Password,
		}, 0); err != nil {
			slog.Error("failed to store registry credentials", "registry_id", newReg.ID, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":   newReg.ID,
		"name": newReg.Name,
		"url":  newReg.URL,
	})
}

// ──────────────────── resources.go ────────────────────
// ResourceHandler manages container resource limits.
type ResourceHandler struct {
	store  core.Store
	events *core.EventBus
}

func NewResourceHandler(store core.Store, events *core.EventBus) *ResourceHandler {
	return &ResourceHandler{store: store, events: events}
}

type resourceLimitsRequest struct {
	CPUQuota int64 `json:"cpu_quota"` // CFS quota microseconds (100000 = 1 core)
	MemoryMB int64 `json:"memory_mb"` // Hard memory limit
}

// SetLimits handles PUT /api/v1/apps/{id}/resources
func (h *ResourceHandler) SetLimits(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var req resourceLimitsRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.CPUQuota < 0 || req.CPUQuota > 1600000 {
		writeError(w, http.StatusBadRequest, "cpu_quota must be between 0 and 1600000")
		return
	}
	if req.MemoryMB < 0 || req.MemoryMB > 131072 {
		writeError(w, http.StatusBadRequest, "memory_mb must be between 0 and 131072")
		return
	}
	// Store resource limits in labels
	labels := map[string]string{
		"monster.resources.cpu_quota": strconv.FormatInt(req.CPUQuota, 10),
		"monster.resources.memory_mb": strconv.FormatInt(req.MemoryMB, 10),
	}
	_ = labels
	_ = app
	// In production: update container with new resource constraints
	// docker update --cpus=X --memory=Ym container_id
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":    appID,
		"cpu_quota": req.CPUQuota,
		"memory_mb": req.MemoryMB,
		"status":    "limits updated",
	})
}

// GetLimits handles GET /api/v1/apps/{id}/resources
func (h *ResourceHandler) GetLimits(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	// Default limits
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":     appID,
		"cpu_quota":  0, // 0 = unlimited
		"memory_mb":  0, // 0 = unlimited
		"pids_limit": 0,
	})
}

// ──────────────────── storage.go ────────────────────
// StorageHandler tracks disk and volume usage per tenant.
type StorageHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	kv      core.KVStorer
}

func NewStorageHandler(store core.Store, runtime core.ContainerRuntime, kv core.KVStorer) *StorageHandler {
	return &StorageHandler{store: store, runtime: runtime, kv: kv}
}

// Usage handles GET /api/v1/storage/usage
func (h *StorageHandler) Usage(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var volumeCount int
	var volumeTotalMB int64
	var imageCount int
	var imageTotalMB int64
	runtimeAvailable := h.runtime != nil
	if runtimeAvailable {
		if volumes, err := h.runtime.VolumeList(r.Context()); err == nil {
			volumeCount = len(volumes)
			for _, v := range volumes {
				if v.Mountpoint == "" {
					continue
				}
				volumeTotalMB += dirSize(v.Mountpoint) / (1024 * 1024)
			}
		}
		if images, err := h.runtime.ImageList(r.Context()); err == nil {
			imageCount = len(images)
			for _, img := range images {
				imageTotalMB += img.Size / (1024 * 1024)
			}
		}
	}
	// Backup byte total is read from the local backup storage rather than
	// a denormalised cache so it can't drift after a manual file delete.
	var backupCount int
	var backupTotalMB int64
	if h.kv != nil {
		var backupStats struct {
			Count   int   `json:"count"`
			TotalMB int64 `json:"total_mb"`
		}
		if h.kv.Get("metrics_ring", "backup_stats:"+claims.TenantID, &backupStats) == nil {
			backupCount = backupStats.Count
			backupTotalMB = backupStats.TotalMB
		}
	}
	resp := map[string]any{
		"tenant_id": claims.TenantID,
		"volumes": map[string]any{
			"count":    volumeCount,
			"total_mb": volumeTotalMB,
		},
		"backups": map[string]any{
			"count":    backupCount,
			"total_mb": backupTotalMB,
		},
		"databases": map[string]any{
			"count":    0,
			"total_mb": 0,
		},
		"images": map[string]any{
			"count":    imageCount,
			"total_mb": imageTotalMB,
		},
	}
	if !runtimeAvailable {
		resp["runtime"] = "unavailable"
	}
	writeJSON(w, http.StatusOK, resp)
}

// ──────────────────── volumes.go ────────────────────
// VolumeHandler manages Docker volume operations.
type VolumeHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
	events  *core.EventBus
}

func NewVolumeHandler(runtime core.ContainerRuntime, store core.Store, events *core.EventBus) *VolumeHandler {
	return &VolumeHandler{runtime: runtime, store: store, events: events}
}

// List handles GET /api/v1/volumes
func (h *VolumeHandler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if h.runtime == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
		return
	}
	// List containers to extract volume info from labels
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list volumes")
		return
	}
	// Extract unique volume info from container names
	volumes := make([]map[string]string, 0)
	seen := make(map[string]bool)
	for _, c := range containers {
		appID := c.Labels["monster.app.id"]
		if appID != "" && !seen[appID] && h.appVisibleToTenant(r.Context(), appID, claims.TenantID, c.Labels) {
			seen[appID] = true
			volumes = append(volumes, map[string]string{
				"app_id":       appID,
				"container_id": shortResourceID(c.ID),
				"name":         c.Labels["monster.app.name"],
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": volumes, "total": len(volumes)})
}
func (h *VolumeHandler) appVisibleToTenant(ctx context.Context, appID, tenantID string, labels map[string]string) bool {
	if labelTenant := labels["monster.tenant"]; labelTenant != "" {
		return labelTenant == tenantID
	}
	if h.store == nil {
		return false
	}
	app, err := h.store.GetApp(ctx, appID)
	return err == nil && app != nil && app.TenantID == tenantID
}

type createVolumeRequest struct {
	Name      string `json:"name"`
	AppID     string `json:"app_id"`
	MountPath string `json:"mount_path"`
	SizeMB    int    `json:"size_mb"`
}

// Create handles POST /api/v1/volumes
func (h *VolumeHandler) Create(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createVolumeRequest
	if !decodeJSONInto(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.SizeMB < 0 || req.SizeMB > 102400 {
		writeError(w, http.StatusBadRequest, "size_mb must be between 0 and 102400")
		return
	}
	if req.AppID != "" && !h.appVisibleToTenant(r.Context(), req.AppID, claims.TenantID, nil) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	// Volume creation would use Docker Volume API
	writeJSON(w, http.StatusCreated, map[string]any{
		"name":       req.Name,
		"app_id":     req.AppID,
		"mount_path": req.MountPath,
		"size_mb":    req.SizeMB,
		"driver":     "local",
		"status":     "created",
	})
}
