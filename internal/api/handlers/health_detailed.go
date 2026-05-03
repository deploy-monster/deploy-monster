package handlers

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
	evStats := h.core.Events.Stats()
	checks["events"] = map[string]any{
		"healthy":       true,
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
