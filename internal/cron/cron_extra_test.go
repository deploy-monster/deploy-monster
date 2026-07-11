package cron

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// refresh — nil dependency early return
// =============================================================================

func TestRefresh_NilCore(t *testing.T) {
	m := &Module{}
	// Should not panic when core is nil
	m.refresh()
}

func TestRefresh_NilScheduler(t *testing.T) {
	m := &Module{
		core:   &core.Core{},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	m.refresh()
}

func TestRefresh_NilDB(t *testing.T) {
	m := &Module{
		core: &core.Core{
			Scheduler: core.NewScheduler(slog.New(slog.NewTextHandler(io.Discard, nil))),
		},
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	m.refresh()
}

// =============================================================================
// refresh — list error (non-KVNotFound, logs warning)
// =============================================================================

func TestRefresh_ListErrorLogsWarning(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bolt := &testBolt{
		data:    make(map[string]map[string][]byte),
		listErr: errors.New("permission denied"),
	}
	m := &Module{
		core:   testCore(bolt, &cronRuntime{}),
		logger: logger,
	}

	m.refresh()

	if !strings.Contains(logs.String(), "cron: list cronjobs bucket failed") {
		t.Fatalf("expected warning log, got: %q", logs.String())
	}
}

// =============================================================================
// refresh — non-appcron jobs in scheduler are skipped
// =============================================================================

func TestRefresh_NonAppcronJobSkipped(t *testing.T) {
	bolt := newTestBolt()
	if err := bolt.Set("cronjobs", "app-1", jobList{Jobs: []jobConfig{
		{ID: "j1", Name: "J1", Schedule: "@every 1m", Command: "echo ok", Enabled: true},
	}}, 0); err != nil {
		t.Fatalf("seed cronjobs: %v", err)
	}
	c := testCore(bolt, &cronRuntime{})
	// Add a non-cron (non-appcron prefixed) job that should be skipped in refresh
	c.Scheduler.Add(&core.CronJob{
		ID:       "not-cron:abc",
		Name:     "other",
		Schedule: "@every 1m",
		Handler:  func(context.Context) error { return nil },
	})
	// Add an appcron job that IS in the desired set
	c.Scheduler.Add(&core.CronJob{
		ID:       "appcron:app-1:j1",
		Name:     "exists",
		Schedule: "@every 1m",
		Handler:  func(context.Context) error { return nil },
	})

	m := &Module{
		core:   c,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	m.refresh()

	// After refresh, "not-cron:abc" should still exist (it's not removed by the prefix check)
	// and "appcron:app-1:j1" should still exist (it's in the desired set).
	jobs := c.Scheduler.Jobs()
	foundNonCron := false
	foundCron := false
	for _, job := range jobs {
		if job.ID == "not-cron:abc" {
			foundNonCron = true
		}
		if job.ID == "appcron:app-1:j1" {
			foundCron = true
		}
	}
	if !foundNonCron {
		t.Error("expected non-appcron job to remain in scheduler")
	}
	if !foundCron {
		t.Error("expected appcron job to remain in scheduler")
	}
}

// =============================================================================
// Start — subscribe callbacks for EventCronJobCreated and EventCronJobDeleted
// =============================================================================

func TestStart_EventSubscriptions(t *testing.T) {
	bolt := newTestBolt()
	if err := bolt.Set("cronjobs", "app-1", jobList{Jobs: []jobConfig{
		{ID: "j1", Name: "J1", Schedule: "@every 1m", Command: "echo ok", Enabled: true},
	}}, 0); err != nil {
		t.Fatalf("seed cronjobs: %v", err)
	}
	c := testCore(bolt, &cronRuntime{})
	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Fire EventCronJobCreated — this invokes the subscribed callback
	if c.Events != nil {
		c.Events.PublishAsync(context.Background(), core.NewEvent(core.EventCronJobCreated, "test", nil))
	}
	// Fire EventCronJobDeleted
	if c.Events != nil {
		c.Events.PublishAsync(context.Background(), core.NewEvent(core.EventCronJobDeleted, "test", nil))
	}

	_ = m.Stop(context.Background())
}

// =============================================================================
// handlerFor — exec error path (stores error in entry)
// =============================================================================

func TestHandlerFor_ExecError(t *testing.T) {
	bolt := newTestBolt()
	runtime := &cronRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123"}},
		output:     "",
		execErr:    errors.New("command failed with exit 1"),
	}
	m := &Module{
		core:   testCore(bolt, runtime),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "ls"})(context.Background())
	if err == nil {
		t.Fatal("expected exec error")
	}
	keys, _ := bolt.List("app_commands")
	if len(keys) != 1 {
		t.Fatalf("expected one command history entry, got %d", len(keys))
	}
	var entry map[string]any
	if err := bolt.Get("app_commands", keys[0], &entry); err != nil {
		t.Fatalf("get command history: %v", err)
	}
	if entry["success"] != false {
		t.Errorf("expected success=false, got %v", entry["success"])
	}
	if entry["error"] != "command failed with exit 1" {
		t.Errorf("expected error in entry, got: %v", entry["error"])
	}
}

// =============================================================================
// handlerFor — ListByLabels error
// =============================================================================

func TestHandlerFor_ListByLabelsError(t *testing.T) {
	bolt := newTestBolt()
	runtime := &cronRuntime{
		containers: nil,
		listErr:    errors.New("list error"),
	}
	m := &Module{
		core:   testCore(bolt, runtime),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Override runtime to use a wrapper that returns list error
	// Use the mock directly by setting the listErr on the existing runtime struct
	err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "echo ok"})(context.Background())
	if err == nil {
		t.Fatal("expected list error")
	}
	if !strings.Contains(err.Error(), "list error") {
		t.Errorf("expected list error in message, got: %v", err)
	}
}

// =============================================================================
// Start — with nil Events (does not subscribe)
// =============================================================================

func TestStart_NilEvents(t *testing.T) {
	bolt := newTestBolt()
	c := testCore(bolt, &cronRuntime{})
	c.Events = nil
	m := &Module{
		core:   c,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start should succeed even without Events: %v", err)
	}
}

// =============================================================================
// handlerFor — Bolt.Set error (logs warning, continues)
// =============================================================================

func TestHandlerFor_BoltSetError(t *testing.T) {
	bolt := newTestBolt()
	bolt.setErr = errors.New("disk full")
	runtime := &cronRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc"}},
		output:     "ok",
	}
	m := &Module{
		core:   testCore(bolt, runtime),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "echo ok"})(context.Background())
	// The handler should succeed even if storing history fails
	if err != nil {
		t.Fatalf("handler should succeed even with Bolt.Set error: %v", err)
	}
}

// =============================================================================
// handlerFor — runtime nil error
// =============================================================================

func TestHandlerFor_NilRuntime(t *testing.T) {
	bolt := newTestBolt()
	c := testCore(bolt, nil) // nil runtime
	c.Services.Container = nil
	m := &Module{
		core:   c,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "true"})(context.Background())
	if err == nil {
		t.Fatal("expected nil runtime error")
	}
	if !strings.Contains(err.Error(), "container runtime not available") {
		t.Errorf("expected runtime error, got: %v", err)
	}
}
