package integrations

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PrometheusExporter serves an OpenMetrics /metrics endpoint
// for scraping by Prometheus or compatible systems.
type PrometheusExporter struct {
	registry  *core.Registry
	events    *core.EventBus
	services  *core.Services
	startTime time.Time
}

// NewPrometheusExporter creates a Prometheus metrics exporter.
func NewPrometheusExporter(registry *core.Registry, events *core.EventBus, services *core.Services) *PrometheusExporter {
	return &PrometheusExporter{
		registry:  registry,
		events:    events,
		services:  services,
		startTime: time.Now(),
	}
}

// Handler returns an HTTP handler for GET /metrics
func (p *PrometheusExporter) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		var b strings.Builder

		// Uptime
		uptime := time.Since(p.startTime).Seconds()
		fmt.Fprintf(&b, "# HELP deploymonster_uptime_seconds Time since server started\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_uptime_seconds gauge\n")
		fmt.Fprintf(&b, "deploymonster_uptime_seconds %.2f\n\n", uptime)

		// Go runtime
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		fmt.Fprintf(&b, "# HELP deploymonster_go_goroutines Number of goroutines\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_go_goroutines gauge\n")
		fmt.Fprintf(&b, "deploymonster_go_goroutines %d\n\n", runtime.NumGoroutine())

		fmt.Fprintf(&b, "# HELP deploymonster_go_memory_bytes Memory usage in bytes\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_go_memory_bytes gauge\n")
		fmt.Fprintf(&b, "deploymonster_go_memory_bytes{type=\"alloc\"} %d\n", mem.Alloc)
		fmt.Fprintf(&b, "deploymonster_go_memory_bytes{type=\"sys\"} %d\n\n", mem.Sys)

		// Module health
		fmt.Fprintf(&b, "# HELP deploymonster_module_health Module health status (0=down, 1=degraded, 2=ok)\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_module_health gauge\n")
		for id, status := range p.registry.HealthAll() {
			fmt.Fprintf(&b, "deploymonster_module_health{module=%q} %d\n", id, status)
		}
		b.WriteString("\n")

		// Event bus stats
		stats := p.events.Stats()
		fmt.Fprintf(&b, "# HELP deploymonster_events_published_total Total events published\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_events_published_total counter\n")
		fmt.Fprintf(&b, "deploymonster_events_published_total %d\n\n", stats.PublishCount)

		fmt.Fprintf(&b, "# HELP deploymonster_events_errors_total Total event handler errors\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_events_errors_total counter\n")
		fmt.Fprintf(&b, "deploymonster_events_errors_total %d\n\n", stats.ErrorCount)

		fmt.Fprintf(&b, "# HELP deploymonster_events_subscriptions Active event subscriptions\n")
		fmt.Fprintf(&b, "# TYPE deploymonster_events_subscriptions gauge\n")
		fmt.Fprintf(&b, "deploymonster_events_subscriptions %d\n\n", stats.SubscriptionCount)

		// Container count
		if p.services.Container != nil {
			containers, err := p.services.Container.ListByLabels(r.Context(), map[string]string{"monster.enable": "true"})
			if err == nil {
				fmt.Fprintf(&b, "# HELP deploymonster_containers_total Managed containers\n")
				fmt.Fprintf(&b, "# TYPE deploymonster_containers_total gauge\n")
				fmt.Fprintf(&b, "deploymonster_containers_total %d\n\n", len(containers))
			}
		}

		w.Write([]byte(b.String()))
	}
}
