package ingress

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// HealthStatus represents the health status of the ingress.
type HealthStatus struct {
	Status      string            `json:"status"`
	Timestamp   time.Time         `json:"timestamp"`
	Routes      int               `json:"routes"`
	ActiveConns int64             `json:"active_connections"`
	Circuits    map[string]string `json:"circuits,omitempty"`
	Uptime      time.Duration     `json:"uptime"`
	Version     string            `json:"version"`
}

// startTime tracks when the module started.
var startTime atomic.Value

func init() {
	startTime.Store(time.Now())
}

// healthHandler returns a handler for health check endpoints.
func (m *Module) healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := "healthy"

		// Check if router is initialized
		if m.router == nil {
			status = "unhealthy"
		}

		// Get route count
		routeCount := 0
		if m.router != nil {
			m.router.mu.RLock()
			routeCount = len(m.router.routes)
			m.router.mu.RUnlock()
		}

		// Get active connections
		var activeConns int64
		if m.proxy != nil && m.proxy.tracker != nil {
			activeConns = m.proxy.tracker.Total()
		}

		// Get circuit breaker status
		var circuits map[string]string
		if m.proxy != nil && m.proxy.circuit != nil {
			allStats := m.proxy.circuit.AllStats()
			circuits = make(map[string]string, len(allStats))
			for backend, stats := range allStats {
				circuits[backend] = stats.State.String()
			}
		}

		health := HealthStatus{
			Status:      status,
			Timestamp:   time.Now(),
			Routes:      routeCount,
			ActiveConns: activeConns,
			Circuits:    circuits,
			Uptime: func() time.Duration {
				if t, ok := startTime.Load().(time.Time); ok {
					return time.Since(t).Round(time.Second)
				}
				return 0
			}(),
			Version: m.Version(),
		}

		w.Header().Set("Content-Type", "application/json")
		if status == "unhealthy" {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(health)
	}
}

// readyHandler returns a handler for readiness check endpoints.
// Returns 200 only if the ingress can accept traffic.
func (m *Module) readyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check basic readiness
		if m.router == nil {
			http.Error(w, "router not initialized", http.StatusServiceUnavailable)
			return
		}

		if m.proxy == nil {
			http.Error(w, "proxy not initialized", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// liveHandler returns a handler for liveness check endpoints.
// Always returns 200 if the process is running.
func (m *Module) liveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}
