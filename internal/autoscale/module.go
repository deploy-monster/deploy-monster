// Package autoscale evaluates per-app autoscale rules persisted by the
// API (handlers/autoscale.go) and updates the application's desired
// replica count when the live container CPU usage crosses the
// configured thresholds. Container reconciliation (creating/removing
// extra replicas) happens on the next deploy — this evaluator owns the
// "decide what should be true" half of the loop, not the "enforce it"
// half. That split is documented on the response from the autoscale
// PUT endpoint and surfaced in the per-decision event so operators
// can see exactly when and why a scale was decided.
package autoscale

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module wires the periodic evaluator to the platform.
type Module struct {
	core   *core.Core
	logger *slog.Logger

	mu     sync.Mutex
	stop   chan struct{}
	stopWG sync.WaitGroup
}

// New constructs the module.
func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "autoscale" }
func (m *Module) Name() string                { return "Autoscale Evaluator" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

// EvalInterval controls how often the evaluator wakes. 30 seconds is
// fine-grained enough that per-minute load spikes get a chance to
// trigger a scale before they flatten out, and coarse enough that
// the runtime stat polling cost stays negligible.
const EvalInterval = 30 * time.Second

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stop != nil {
		return nil
	}
	m.stop = make(chan struct{})
	stop := m.stop
	m.stopWG.Add(1)
	go m.loop(stop)
	m.logger.Info("autoscale evaluator started", "interval", EvalInterval)
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	m.mu.Lock()
	if m.stop == nil {
		m.mu.Unlock()
		return nil
	}
	close(m.stop)
	m.stop = nil
	m.mu.Unlock()
	m.stopWG.Wait()
	return nil
}

func (m *Module) Health() core.HealthStatus { return core.HealthOK }

func (m *Module) loop(stop <-chan struct{}) {
	defer m.stopWG.Done()
	t := time.NewTicker(EvalInterval)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			m.evaluateAll()
		}
	}
}

// autoscaleConfig mirrors handlers.AutoscaleConfig — duplicated so this
// package doesn't import internal/api/handlers (which would form a
// cycle through core.Store consumers). Field tags must stay in sync.
type autoscaleConfig struct {
	Enabled        bool `json:"enabled"`
	MinReplicas    int  `json:"min_replicas"`
	MaxReplicas    int  `json:"max_replicas"`
	CPUTarget      int  `json:"cpu_target_percent"`
	RAMTarget      int  `json:"ram_target_percent"`
	ScaleUpDelay   int  `json:"scale_up_delay_sec"`
	ScaleDownDelay int  `json:"scale_down_delay_sec"`
}

// decision is what the evaluator persists per app per evaluation tick.
// The UI can read this from KV storage to show "last decision" without
// re-deriving it from raw stats.
type decision struct {
	AppID         string    `json:"app_id"`
	EvaluatedAt   time.Time `json:"evaluated_at"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryPercent float64   `json:"memory_percent"`
	Replicas      int       `json:"replicas"`
	DesiredReps   int       `json:"desired_replicas"`
	Action        string    `json:"action"` // "scale_up" | "scale_down" | "hold" | "cooldown" | "skip"
	Reason        string    `json:"reason"`
}

const (
	decisionBucket = "autoscale_decisions"
	decisionTTL    = int64(7 * 24 * 3600)
)

func (m *Module) evaluateAll() {
	if m.core == nil || m.core.DB == nil || m.core.DB.Bolt == nil {
		return
	}
	bolt := m.core.DB.Bolt
	keys, err := bolt.List("autoscale")
	if err != nil {
		return
	}
	for _, appID := range keys {
		var cfg autoscaleConfig
		if err := bolt.Get("autoscale", appID, &cfg); err != nil || !cfg.Enabled {
			continue
		}
		m.evaluate(appID, cfg)
	}
}

// evaluate runs one autoscale tick for a single app. It is exported on
// the receiver so tests in the same package can drive a single tick
// deterministically without the loop goroutine.
func (m *Module) evaluate(appID string, cfg autoscaleConfig) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d := decision{AppID: appID, EvaluatedAt: time.Now().UTC()}

	if m.core.Services == nil || m.core.Services.Container == nil {
		d.Action = "skip"
		d.Reason = "container runtime not available"
		m.persist(d)
		return
	}

	app, err := m.core.Store.GetApp(ctx, appID)
	if err != nil {
		d.Action = "skip"
		d.Reason = fmt.Sprintf("app lookup failed: %v", err)
		m.persist(d)
		return
	}
	d.Replicas = app.Replicas
	d.DesiredReps = app.Replicas

	containers, err := m.core.Services.Container.ListByLabels(ctx, map[string]string{"monster.app.id": appID})
	if err != nil || len(containers) == 0 {
		d.Action = "skip"
		d.Reason = "no containers running"
		m.persist(d)
		return
	}

	var totalCPU, totalMem float64
	var samples int
	for _, c := range containers {
		s, err := m.core.Services.Container.Stats(ctx, c.ID)
		if err != nil || s == nil {
			continue
		}
		totalCPU += s.CPUPercent
		totalMem += s.MemoryPercent
		samples++
	}
	if samples == 0 {
		d.Action = "skip"
		d.Reason = "no stats samples"
		m.persist(d)
		return
	}
	d.CPUPercent = totalCPU / float64(samples)
	d.MemoryPercent = totalMem / float64(samples)

	// Hysteresis: scale up at target, scale down only when comfortably
	// below it (target * 0.7) so a noisy workload doesn't oscillate.
	cpuTrigger := float64(cfg.CPUTarget)
	memTrigger := float64(cfg.RAMTarget)
	cpuRelease := cpuTrigger * 0.7
	memRelease := memTrigger * 0.7

	// Cooldowns: respect the last decision's window.
	if last, ok := m.lastDecision(appID); ok {
		switch last.Action {
		case "scale_up":
			if time.Since(last.EvaluatedAt) < time.Duration(cfg.ScaleUpDelay)*time.Second {
				d.Action = "cooldown"
				d.Reason = "scale-up cooldown"
				m.persist(d)
				return
			}
		case "scale_down":
			if time.Since(last.EvaluatedAt) < time.Duration(cfg.ScaleDownDelay)*time.Second {
				d.Action = "cooldown"
				d.Reason = "scale-down cooldown"
				m.persist(d)
				return
			}
		}
	}

	switch {
	case (d.CPUPercent >= cpuTrigger || d.MemoryPercent >= memTrigger) && app.Replicas < cfg.MaxReplicas:
		d.DesiredReps = app.Replicas + 1
		d.Action = "scale_up"
		d.Reason = fmt.Sprintf("cpu=%.1f%% mem=%.1f%% (cpu_target=%d mem_target=%d)", d.CPUPercent, d.MemoryPercent, cfg.CPUTarget, cfg.RAMTarget)
	case d.CPUPercent < cpuRelease && d.MemoryPercent < memRelease && app.Replicas > cfg.MinReplicas:
		d.DesiredReps = app.Replicas - 1
		d.Action = "scale_down"
		d.Reason = fmt.Sprintf("cpu=%.1f%% mem=%.1f%% (release thresholds %.1f%% / %.1f%%)", d.CPUPercent, d.MemoryPercent, cpuRelease, memRelease)
	default:
		d.Action = "hold"
		d.Reason = fmt.Sprintf("cpu=%.1f%% mem=%.1f%%", d.CPUPercent, d.MemoryPercent)
	}

	if d.DesiredReps != app.Replicas {
		app.Replicas = d.DesiredReps
		if err := m.core.Store.UpdateApp(ctx, app); err != nil {
			m.logger.Warn("autoscale: failed to update app replicas", "app_id", appID, "error", err)
			d.Action = "skip"
			d.Reason = fmt.Sprintf("replica update failed: %v", err)
			m.persist(d)
			return
		}
		m.core.Events.PublishAsync(ctx, core.NewEvent(core.EventAppScaled, "autoscale",
			core.AppEventData{AppID: appID, AppName: app.Name, Status: app.Status}))
	}

	m.persist(d)
}

func (m *Module) persist(d decision) {
	if m.core == nil || m.core.DB == nil || m.core.DB.Bolt == nil {
		return
	}
	if raw, err := json.Marshal(d); err == nil {
		_ = m.core.DB.Bolt.Set(decisionBucket, d.AppID, json.RawMessage(raw), decisionTTL)
	}
	m.logger.Info("autoscale decision",
		"app_id", d.AppID,
		"action", d.Action,
		"cpu", d.CPUPercent,
		"mem", d.MemoryPercent,
		"replicas", d.Replicas,
		"desired", d.DesiredReps,
		"reason", d.Reason,
	)
}

func (m *Module) lastDecision(appID string) (decision, bool) {
	var d decision
	if m.core == nil || m.core.DB == nil || m.core.DB.Bolt == nil {
		return d, false
	}
	if err := m.core.DB.Bolt.Get(decisionBucket, appID, &d); err != nil {
		return d, false
	}
	return d, true
}
