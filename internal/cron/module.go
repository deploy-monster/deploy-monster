// Package cron wires the per-app cron jobs persisted by the API
// (handlers/cronjobs.go) into the platform's task scheduler so they
// actually fire instead of sitting in KV storage forever.
package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module is the cron executor. It loads jobs from the cronjobs KV
// bucket on Start and re-syncs whenever the API emits cronjob lifecycle
// events. Each job's handler shells out into the app's container via
// the runtime's Exec method and records the run in the same
// app_commands history bucket the manual command runner uses.
type Module struct {
	core   *core.Core
	logger *slog.Logger
}

// New constructs the module.
func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "cron" }
func (m *Module) Name() string                { return "Cron Executor" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Subscribe directly to cron lifecycle events. The Module interface's
	// Events() method is currently advisory — the runtime never reads it
	// — so explicit Subscribe calls are the only reliable wiring.
	if m.core != nil && m.core.Events != nil {
		m.core.Events.SubscribeAsync(core.EventCronJobCreated, func(_ context.Context, _ core.Event) error {
			m.refresh()
			return nil
		})
		m.core.Events.SubscribeAsync(core.EventCronJobDeleted, func(_ context.Context, _ core.Event) error {
			m.refresh()
			return nil
		})
	}
	m.refresh()
	m.logger.Info("cron executor started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }

func (m *Module) Health() core.HealthStatus { return core.HealthOK }

// jobConfig mirrors handlers.CronJobConfig — duplicated here to avoid an
// import cycle (handlers already imports core, and core can't depend on
// handlers). Field names must stay in sync with the JSON the handler
// writes.
type jobConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Command  string `json:"command"`
	Enabled  bool   `json:"enabled"`
}

type jobList struct {
	Jobs []jobConfig `json:"jobs"`
}

// refresh resyncs the in-memory scheduler with the persisted job set.
// It removes jobs the user deleted and (re)adds the rest. ID prefix
// "appcron:" namespaces these entries so they don't collide with other
// schedule sources (backup engine etc.).
func (m *Module) refresh() {
	if m.core == nil || m.core.Scheduler == nil || m.core.DB == nil || m.core.DB.Bolt == nil {
		return
	}

	bolt := m.core.DB.Bolt
	keys, err := bolt.List("cronjobs")
	if err != nil {
		m.logger.Warn("cron: list cronjobs bucket failed", "error", err)
		return
	}

	desired := map[string]jobConfig{}
	for _, appID := range keys {
		var list jobList
		if err := bolt.Get("cronjobs", appID, &list); err != nil {
			continue
		}
		for _, j := range list.Jobs {
			if !j.Enabled || j.Schedule == "" || j.Command == "" {
				continue
			}
			desired["appcron:"+appID+":"+j.ID] = j
			m.attach(appID, j)
		}
	}

	// Drop schedules that no longer exist in bolt (user deleted them
	// or the app was removed). The scheduler exposes Jobs() so we can
	// diff without re-walking handlers state.
	for _, existing := range m.core.Scheduler.Jobs() {
		if !strings.HasPrefix(existing.ID, "appcron:") {
			continue
		}
		if _, ok := desired[existing.ID]; !ok {
			m.core.Scheduler.Remove(existing.ID)
		}
	}
}

func (m *Module) attach(appID string, cfg jobConfig) {
	jobID := "appcron:" + appID + ":" + cfg.ID
	m.core.Scheduler.Add(&core.CronJob{
		ID:       jobID,
		Name:     fmt.Sprintf("%s (app %s)", cfg.Name, appID),
		Schedule: cfg.Schedule,
		Enabled:  true,
		Handler:  m.handlerFor(appID, cfg),
	})
}

func (m *Module) handlerFor(appID string, cfg jobConfig) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		runtime := m.core.Services.Container
		if runtime == nil {
			return fmt.Errorf("container runtime not available")
		}
		containers, err := runtime.ListByLabels(ctx, map[string]string{
			"monster.app.id": appID,
		})
		if err != nil {
			return fmt.Errorf("list containers: %w", err)
		}
		if len(containers) == 0 {
			return fmt.Errorf("no container for app %s", appID)
		}

		startedAt := time.Now()
		cmd := core.SplitCommand(cfg.Command)
		if !core.CommandTokensSafe(cmd) {
			return fmt.Errorf("cron command blocked by security policy")
		}
		output, execErr := runtime.Exec(ctx, containers[0].ID, cmd)
		completedAt := time.Now()

		entryID := core.GenerateID()
		entry := map[string]any{
			"id":           entryID,
			"app_id":       appID,
			"user_id":      "cron:" + cfg.ID,
			"command":      cfg.Command,
			"container_id": shortID(containers[0].ID),
			"output":       capOutput(output, 64*1024),
			"started_at":   startedAt,
			"completed_at": completedAt,
			"success":      execErr == nil,
		}
		if execErr != nil {
			entry["error"] = execErr.Error()
		}
		if raw, err := json.Marshal(entry); err == nil {
			if err := m.core.DB.Bolt.Set("app_commands", appID+":"+entryID, json.RawMessage(raw), 30*24*3600); err != nil {
				slog.Warn("failed to store cron command history", "app_id", appID, "job_id", cfg.ID, "error", err)
			}
		}

		m.core.Events.PublishAsync(ctx, core.NewEvent("app.cron.run", "cron", map[string]any{
			"app_id":  appID,
			"job_id":  cfg.ID,
			"success": execErr == nil,
		}))
		return execErr
	}
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func capOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... [output truncated]"
}
