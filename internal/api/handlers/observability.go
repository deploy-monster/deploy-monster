package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ──────────────────── build_logs.go ────────────────────
// BuildLogHandler serves build log retrieval and download.
type BuildLogHandler struct {
	store core.Store
}

func NewBuildLogHandler(store core.Store) *BuildLogHandler {
	return &BuildLogHandler{store: store}
}

// Get handles GET /api/v1/apps/{id}/builds/{version}/log
func (h *BuildLogHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	_ = r.PathValue("version")
	dep, err := h.store.GetLatestDeployment(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no deployment found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":  appID,
		"version": dep.Version,
		"log":     dep.BuildLog,
		"status":  dep.Status,
	})
}

// Download handles GET /api/v1/apps/{id}/builds/{version}/log/download
func (h *BuildLogHandler) Download(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	dep, err := h.store.GetLatestDeployment(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusNotFound, "no deployment found")
		return
	}
	filename := fmt.Sprintf("%s-build-v%d-%s.log", core.ShortID(appID, 8), dep.Version, time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(filename))
	if dep.BuildLog != "" {
		w.Write([]byte(dep.BuildLog))
	} else {
		w.Write([]byte("No build log available for this deployment.\n"))
	}
}

// ──────────────────── health_detailed.go ────────────────────
// DetailedHealthHandler provides deep health checks for each subsystem.
type DetailedHealthHandler struct {
	core      *core.Core
	rateLimit *middleware.GlobalRateLimiter
}

func NewDetailedHealthHandler(c *core.Core) *DetailedHealthHandler {
	return &DetailedHealthHandler{core: c}
}

// SetRateLimiter sets the global rate limiter for stats reporting.
func (h *DetailedHealthHandler) SetRateLimiter(rl *middleware.GlobalRateLimiter) {
	h.rateLimit = rl
}

// DetailedHealth handles GET /health/detailed
func (h *DetailedHealthHandler) DetailedHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	checks := make(map[string]any)
	overallOK := true
	// Module health
	for id, status := range h.core.Registry.HealthAll() {
		ok := status == core.HealthOK || status == core.HealthDegraded
		if !ok {
			overallOK = false
		}
		checks[id] = map[string]any{
			"status":  status.String(),
			"healthy": ok,
		}
	}
	// Database connectivity
	dbOK := false
	if h.core.Store != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.core.Store.Ping(ctx); err == nil {
			dbOK = true
		}
	}
	checks["database"] = map[string]any{"healthy": dbOK, "driver": h.core.Config.Database.Driver}
	// Docker connectivity.
	dockerOK := false
	if h.core.Services.Container != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if rt, ok := h.core.Services.Container.(interface{ PingContext(context.Context) error }); ok {
			dockerOK = rt.PingContext(ctx) == nil
		} else {
			dockerOK = h.core.Services.Container.Ping() == nil
		}
	}
	checks["docker"] = map[string]any{"healthy": dockerOK}
	// Event bus
	evStats := eventBusStats(h.core.Events)
	eventsHealthy := h.core.Events != nil
	checks["events"] = map[string]any{
		"healthy":       eventsHealthy,
		"published":     evStats.PublishCount,
		"errors":        evStats.ErrorCount,
		"subscriptions": evStats.SubscriptionCount,
	}
	// Rate limiter
	if h.rateLimit != nil {
		rlStats := h.rateLimit.Stats()
		checks["rate_limiter"] = map[string]any{
			"healthy":         true,
			"rate_per_minute": rlStats.Rate,
			"active_clients":  rlStats.ActiveClients,
		}
	}
	// Runtime
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	checks["runtime"] = map[string]any{
		"healthy":    true,
		"goroutines": runtime.NumGoroutine(),
		"alloc_mb":   mem.Alloc / 1024 / 1024,
		"sys_mb":     mem.Sys / 1024 / 1024,
		"gc_runs":    mem.NumGC,
	}
	status := "healthy"
	httpStatus := http.StatusOK
	if !overallOK || !dbOK {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}
	writeJSON(w, httpStatus, map[string]any{
		"status":   status,
		"version":  h.core.Build.Version,
		"checks":   checks,
		"duration": time.Since(start).String(),
	})
}

// ──────────────────── healthcheck.go ────────────────────
// HealthCheckHandler manages per-app health check configuration.
type HealthCheckHandler struct {
	store core.Store
}

func NewHealthCheckHandler(store core.Store) *HealthCheckHandler {
	return &HealthCheckHandler{store: store}
}

// HealthCheckConfig defines how to check if an app is healthy.
type HealthCheckConfig struct {
	Type     string `json:"type"`     // http, tcp, exec, none
	Path     string `json:"path"`     // HTTP health check path
	Port     int    `json:"port"`     // Port to check
	Interval int    `json:"interval"` // Seconds between checks
	Timeout  int    `json:"timeout"`  // Seconds before timeout
	Retries  int    `json:"retries"`  // Failures before unhealthy
}

// Get handles GET /api/v1/apps/{id}/healthcheck
func (h *HealthCheckHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	// Default health check config
	writeJSON(w, http.StatusOK, HealthCheckConfig{
		Type:     "http",
		Path:     "/health",
		Port:     0, // Use app's primary port
		Interval: 10,
		Timeout:  5,
		Retries:  3,
	})
}

// Update handles PUT /api/v1/apps/{id}/healthcheck
func (h *HealthCheckHandler) Update(w http.ResponseWriter, r *http.Request) {
	// SECURITY: Verify the app belongs to this tenant
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg HealthCheckConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	valid := map[string]bool{"http": true, "tcp": true, "exec": true, "none": true}
	if !valid[cfg.Type] {
		writeError(w, http.StatusBadRequest, "type must be: http, tcp, exec, none")
		return
	}
	// Cap Path so a malicious admin can't store a multi-MB string that
	// slips under the 10MB BodyLimit middleware and then gets reloaded on
	// every health probe.
	if len(cfg.Path) > 2048 {
		writeError(w, http.StatusBadRequest, "path must be 2048 characters or fewer")
		return
	}
	// 0 = use app's primary port; otherwise must be in TCP/UDP range.
	if cfg.Port < 0 || cfg.Port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be between 0 and 65535")
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 10
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5
	}
	if cfg.Retries <= 0 {
		cfg.Retries = 3
	}
	// Upper bounds to prevent pathological values. A 1-hour interval is
	// the longest anyone reasonably wants; same for timeout. 100 retries
	// is more than any real workload needs and caps the worst-case
	// memory the health checker allocates per app.
	if cfg.Interval > 3600 {
		writeError(w, http.StatusBadRequest, "interval must be 3600 seconds or fewer")
		return
	}
	if cfg.Timeout > 300 {
		writeError(w, http.StatusBadRequest, "timeout must be 300 seconds or fewer")
		return
	}
	if cfg.Retries > 100 {
		writeError(w, http.StatusBadRequest, "retries must be 100 or fewer")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}

// ──────────────────── log_download.go ────────────────────
// LogDownloadHandler exports container logs as a downloadable file.
type LogDownloadHandler struct {
	runtime core.ContainerRuntime
}

func NewLogDownloadHandler(runtime core.ContainerRuntime) *LogDownloadHandler {
	return &LogDownloadHandler{runtime: runtime}
}

// Download handles GET /api/v1/apps/{id}/logs/download
func (h *LogDownloadHandler) Download(w http.ResponseWriter, r *http.Request) {
	appID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no container found")
		return
	}
	reader, err := h.runtime.Logs(r.Context(), containers[0].ID, "5000", false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read logs")
		return
	}
	defer reader.Close()
	filename := fmt.Sprintf("%s-logs-%s.txt", core.ShortID(appID, 8), time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(filename))
	ctx := r.Context()
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := reader.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}

// ──────────────────── log_retention.go ────────────────────
// LogRetentionHandler manages per-app log retention settings.
type LogRetentionHandler struct {
	store core.Store
	kv    core.KVStorer
}

func NewLogRetentionHandler(store core.Store, kv core.KVStorer) *LogRetentionHandler {
	return &LogRetentionHandler{store: store, kv: kv}
}

// LogRetentionConfig defines how long to keep container logs.
type LogRetentionConfig struct {
	MaxSizeMB int    `json:"max_size_mb"` // Max log file size before rotation
	MaxFiles  int    `json:"max_files"`   // Number of rotated files to keep
	Driver    string `json:"driver"`      // json-file, local, syslog
}

// defaultLogRetention returns sensible defaults.
func defaultLogRetention() LogRetentionConfig {
	return LogRetentionConfig{
		MaxSizeMB: 50,
		MaxFiles:  5,
		Driver:    "json-file",
	}
}

// Get handles GET /api/v1/apps/{id}/log-retention
func (h *LogRetentionHandler) Get(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	var cfg LogRetentionConfig
	if err := h.kv.Get("log_retention", app.ID, &cfg); err != nil {
		// Return defaults if not configured
		writeJSON(w, http.StatusOK, defaultLogRetention())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// Update handles PUT /api/v1/apps/{id}/log-retention
func (h *LogRetentionHandler) Update(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	var cfg LogRetentionConfig
	if !decodeJSONInto(w, r, &cfg) {
		return
	}
	if cfg.MaxSizeMB <= 0 {
		cfg.MaxSizeMB = 50
	}
	if cfg.MaxFiles <= 0 {
		cfg.MaxFiles = 5
	}
	if cfg.Driver == "" {
		cfg.Driver = "json-file"
	}
	const maxLogSizeMB = 10240
	const maxLogFiles = 100
	if cfg.MaxSizeMB > maxLogSizeMB {
		writeError(w, http.StatusBadRequest, "max_size_mb exceeds 10240 (10 GB)")
		return
	}
	if cfg.MaxFiles > maxLogFiles {
		writeError(w, http.StatusBadRequest, "max_files exceeds 100")
		return
	}
	switch cfg.Driver {
	case "json-file", "local", "syslog":
	default:
		writeError(w, http.StatusBadRequest, "driver must be one of: json-file, local, syslog")
		return
	}
	if err := h.kv.Set("log_retention", appID, cfg, 0); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save log retention config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"config": cfg,
		"status": "updated",
	})
}

// ──────────────────── logs.go ────────────────────
// LogHandler serves application logs.
type LogHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
}

func NewLogHandler(runtime core.ContainerRuntime, store core.Store) *LogHandler {
	return &LogHandler{runtime: runtime, store: store}
}

// GetLogs handles GET /api/v1/apps/{id}/logs
// Returns the last N lines of container logs.
func (h *LogHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}
	if _, err := strconv.Atoi(tail); err != nil {
		tail = "100"
	}
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		writeError(w, http.StatusNotFound, "no running container found")
		return
	}
	reader, err := h.runtime.Logs(r.Context(), containers[0].ID, tail, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read logs")
		return
	}
	defer reader.Close()
	buf := make([]byte, 256*1024) // 256KB max
	n, _ := reader.Read(buf)
	// Parse lines
	lines := splitLines(string(buf[:n]))
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":       appID,
		"container_id": shortResourceID(containers[0].ID),
		"lines":        lines,
		"count":        len(lines),
	})
}
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// ──────────────────── metrics_export.go ────────────────────
// MetricsExportHandler exports metrics data as CSV or JSON.
type MetricsExportHandler struct {
	store   core.Store
	kv      core.KVStorer
	runtime core.ContainerRuntime
}

func NewMetricsExportHandler(store core.Store, kv core.KVStorer, runtime core.ContainerRuntime) *MetricsExportHandler {
	return &MetricsExportHandler{store: store, kv: kv, runtime: runtime}
}

// metricsPoint is a single metrics data point stored in KV storage.
type metricsPoint struct {
	Timestamp  string  `json:"timestamp"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   int64   `json:"memory_mb"`
	Requests   int64   `json:"requests"`
}

// Export handles GET /api/v1/apps/{id}/metrics/export?format=csv
func (h *MetricsExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}
	// Try to load real metrics from KV storage.
	var storedPoints []metricsPoint
	_ = h.kv.Get("metrics_export", appID, &storedPoints)
	// If no stored metrics, get current stats from runtime and generate points
	now := time.Now()
	points := storedPoints
	if len(points) == 0 {
		// Attempt to read live stats for the app
		var currentCPU float64
		var currentMemMB int64
		if h.runtime != nil {
			containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"app": appID})
			if err == nil && len(containers) > 0 {
				stats, err := h.runtime.Stats(r.Context(), containers[0].ID)
				if err == nil {
					currentCPU = stats.CPUPercent
					currentMemMB = stats.MemoryUsage / (1024 * 1024)
				}
			}
		}
		// Generate 24 points, last one with current data
		points = make([]metricsPoint, 24)
		for i := range points {
			points[i] = metricsPoint{
				Timestamp: now.Add(-time.Duration(23-i) * time.Hour).Format(time.RFC3339),
			}
		}
		// Fill in the latest point with real data
		if currentCPU > 0 || currentMemMB > 0 {
			points[23].CPUPercent = currentCPU
			points[23].MemoryMB = currentMemMB
		}
	}
	switch format {
	case "csv":
		filename := fmt.Sprintf("%s-metrics-%s.csv", core.ShortID(appID, 8), now.Format("20060102"))
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename="+safeFilename(filename))
		writer := csv.NewWriter(w)
		writer.Write([]string{"timestamp", "cpu_percent", "memory_mb", "requests"})
		for _, p := range points {
			writer.Write([]string{
				p.Timestamp,
				fmt.Sprintf("%.2f", p.CPUPercent),
				fmt.Sprint(p.MemoryMB),
				fmt.Sprint(p.Requests),
			})
		}
		writer.Flush()
	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"app_id": appID,
			"period": "24h",
			"points": points,
		})
	}
}

// ──────────────────── metrics_history.go ────────────────────
// MetricsHistoryHandler serves historical metrics data for charts.
type MetricsHistoryHandler struct {
	store   core.Store
	runtime core.ContainerRuntime
	kv      core.KVStorer
}

func NewMetricsHistoryHandler(store core.Store, runtime core.ContainerRuntime, kv core.KVStorer) *MetricsHistoryHandler {
	return &MetricsHistoryHandler{store: store, runtime: runtime, kv: kv}
}

// MetricsPoint represents a single data point in a time series.
type MetricsPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   int64     `json:"memory_mb"`
	NetworkRx  int64     `json:"network_rx_mb"`
	NetworkTx  int64     `json:"network_tx_mb"`
}

// metricsRing wraps persisted metrics history for an app.
type metricsRing struct {
	Points []MetricsPoint `json:"points"`
}

// AppMetrics handles GET /api/v1/apps/{id}/metrics
func (h *MetricsHistoryHandler) AppMetrics(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	period := r.URL.Query().Get("period") // 1h, 24h, 7d, 30d
	if period == "" {
		period = "24h"
	}
	// Try to read stored metrics from KV storage.
	bucketKey := appID + ":" + period
	var ring metricsRing
	if err := h.kv.Get("metrics_ring", bucketKey, &ring); err == nil && len(ring.Points) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"app_id": appID,
			"period": period,
			"points": ring.Points,
			"count":  len(ring.Points),
		})
		return
	}
	// If no stored metrics, try to get current stats from runtime
	var points []MetricsPoint
	if h.runtime != nil {
		containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{"app_id": appID})
		if err == nil && len(containers) > 0 {
			stats, err := h.runtime.Stats(r.Context(), containers[0].ID)
			if err == nil {
				points = append(points, MetricsPoint{
					Timestamp:  time.Now(),
					CPUPercent: stats.CPUPercent,
					MemoryMB:   stats.MemoryUsage / (1024 * 1024),
					NetworkRx:  stats.NetworkRx / (1024 * 1024),
					NetworkTx:  stats.NetworkTx / (1024 * 1024),
				})
			}
		}
	}
	if points == nil {
		points = []MetricsPoint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id": appID,
		"period": period,
		"points": points,
		"count":  len(points),
	})
}

// ServerMetrics handles GET /api/v1/servers/{id}/metrics
func (h *MetricsHistoryHandler) ServerMetrics(w http.ResponseWriter, r *http.Request) {
	serverID, ok := requirePathParam(w, r, "id")
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}
	// Read stored server metrics from KV storage.
	bucketKey := "server:" + serverID + ":" + period
	var ring metricsRing
	if err := h.kv.Get("metrics_ring", bucketKey, &ring); err == nil && len(ring.Points) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"server_id": serverID,
			"period":    period,
			"points":    ring.Points,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"server_id": serverID,
		"period":    period,
		"points":    []MetricsPoint{},
	})
}

// ──────────────────── stats.go ────────────────────
// StatsHandler serves container and server resource metrics.
type StatsHandler struct {
	runtime core.ContainerRuntime
	store   core.Store
}

func NewStatsHandler(runtime core.ContainerRuntime, store core.Store) *StatsHandler {
	return &StatsHandler{runtime: runtime, store: store}
}

// AppStats handles GET /api/v1/apps/{id}/stats.
// Returns aggregated CPU/memory/network usage from all containers attached
// to the app via the monster.app.id label. Empty list returns zeros instead
// of an error so the UI can show a calm "not running" state.
func (h *StatsHandler) AppStats(w http.ResponseWriter, r *http.Request) {
	app := requireTenantApp(w, r, h.store)
	if app == nil {
		return
	}
	appID := app.ID
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list containers")
		return
	}
	type containerStats struct {
		ID            string  `json:"id"`
		Name          string  `json:"name"`
		State         string  `json:"state"`
		CPUPercent    float64 `json:"cpu_percent"`
		MemoryUsage   int64   `json:"memory_usage"`
		MemoryLimit   int64   `json:"memory_limit"`
		MemoryPercent float64 `json:"memory_percent"`
		NetworkRx     int64   `json:"network_rx"`
		NetworkTx     int64   `json:"network_tx"`
		Health        string  `json:"health"`
		Running       bool    `json:"running"`
	}
	perContainer := make([]containerStats, 0, len(containers))
	var aggCPU, aggMemPct float64
	var aggMemUsage, aggMemLimit, aggNetRx, aggNetTx int64
	runningCount := 0
	for _, c := range containers {
		entry := containerStats{
			ID:    c.ID,
			Name:  c.Name,
			State: c.State,
		}
		s, statsErr := h.runtime.Stats(r.Context(), c.ID)
		if statsErr == nil && s != nil {
			entry.CPUPercent = s.CPUPercent
			entry.MemoryUsage = s.MemoryUsage
			entry.MemoryLimit = s.MemoryLimit
			entry.MemoryPercent = s.MemoryPercent
			entry.NetworkRx = s.NetworkRx
			entry.NetworkTx = s.NetworkTx
			entry.Health = s.Health
			entry.Running = s.Running
			aggCPU += s.CPUPercent
			aggMemUsage += s.MemoryUsage
			aggMemLimit += s.MemoryLimit
			aggMemPct += s.MemoryPercent
			aggNetRx += s.NetworkRx
			aggNetTx += s.NetworkTx
			if s.Running {
				runningCount++
			}
		}
		perContainer = append(perContainer, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app_id":         appID,
		"containers":     perContainer,
		"count":          len(perContainer),
		"running":        runningCount,
		"cpu_percent":    aggCPU,
		"memory_usage":   aggMemUsage,
		"memory_limit":   aggMemLimit,
		"memory_percent": aggMemPct,
		"network_rx":     aggNetRx,
		"network_tx":     aggNetTx,
	})
}

// ServerStats handles GET /api/v1/servers/stats
func (h *StatsHandler) ServerStats(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		writeError(w, http.StatusServiceUnavailable, "container runtime not available")
		return
	}
	containers, err := h.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}
	running := 0
	stopped := 0
	for _, c := range containers {
		if c.State == "running" {
			running++
		} else {
			stopped++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total":   len(containers),
		"running": running,
		"stopped": stopped,
	})
}

// ──────────────────── usage_history.go ────────────────────
// UsageHistoryHandler serves resource usage over time for billing and charts.
type UsageHistoryHandler struct {
	kv core.KVStorer
}

func NewUsageHistoryHandler(kv core.KVStorer) *UsageHistoryHandler {
	return &UsageHistoryHandler{kv: kv}
}

// UsageBucket represents aggregated usage for a time period.
type UsageBucket struct {
	Hour         string  `json:"hour"`
	CPUSeconds   float64 `json:"cpu_seconds"`
	RAMMBHours   float64 `json:"ram_mb_hours"`
	BandwidthMB  float64 `json:"bandwidth_mb"`
	BuildSeconds float64 `json:"build_seconds"`
}

// usageHistory is the persisted usage data for a tenant.
type usageHistory struct {
	Buckets []UsageBucket `json:"buckets"`
}

// Hourly handles GET /api/v1/billing/usage/history
func (h *UsageHistoryHandler) Hourly(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "24h"
	}
	var count int
	switch period {
	case "7d":
		count = 168
	case "30d":
		count = 720
	default:
		count = 24
	}
	// Try to load real usage data from KV storage.
	bucketKey := claims.TenantID + ":" + period
	var stored usageHistory
	if err := h.kv.Get("usage_history", bucketKey, &stored); err == nil && len(stored.Buckets) > 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant_id": claims.TenantID,
			"period":    period,
			"buckets":   stored.Buckets,
			"count":     len(stored.Buckets),
		})
		return
	}
	// No stored data — return empty time series
	now := time.Now()
	buckets := make([]UsageBucket, count)
	for i := range buckets {
		buckets[i] = UsageBucket{
			Hour: now.Add(-time.Duration(count-1-i) * time.Hour).Format("2006-01-02T15:00"),
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": claims.TenantID,
		"period":    period,
		"buckets":   buckets,
		"count":     count,
	})
}
