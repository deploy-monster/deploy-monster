package cron

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

type testBolt struct {
	data map[string]map[string][]byte
}

func newTestBolt() *testBolt {
	return &testBolt{data: make(map[string]map[string][]byte)}
}

func (b *testBolt) Set(bucket, key string, value any, _ int64) error {
	if b.data[bucket] == nil {
		b.data[bucket] = make(map[string][]byte)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	b.data[bucket][key] = raw
	return nil
}

func (b *testBolt) BatchSet(items []core.BoltBatchItem) error {
	for _, item := range items {
		if err := b.Set(item.Bucket, item.Key, item.Value, item.TTL); err != nil {
			return err
		}
	}
	return nil
}

func (b *testBolt) Get(bucket, key string, dest any) error {
	raw, ok := b.data[bucket][key]
	if !ok {
		return errors.New("key not found")
	}
	return json.Unmarshal(raw, dest)
}

func (b *testBolt) Delete(bucket, key string) error {
	delete(b.data[bucket], key)
	return nil
}

func (b *testBolt) List(bucket string) ([]string, error) {
	keys := make([]string, 0, len(b.data[bucket]))
	for key := range b.data[bucket] {
		keys = append(keys, key)
	}
	return keys, nil
}

func (b *testBolt) Close() error { return nil }
func (b *testBolt) GetAPIKeyByPrefix(context.Context, string) (*models.APIKey, error) {
	return nil, errors.New("not implemented")
}
func (b *testBolt) GetWebhookSecret(string) (string, error) {
	return "", errors.New("not implemented")
}

type cronRuntime struct {
	core.ContainerRuntime
	containers []core.ContainerInfo
	output     string
	execErr    error
	cmd        []string
}

func (r *cronRuntime) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	return r.containers, nil
}

func (r *cronRuntime) Exec(_ context.Context, _ string, cmd []string) (string, error) {
	r.cmd = cmd
	return r.output, r.execErr
}

func testCore(bolt core.BoltStorer, runtime core.ContainerRuntime) *core.Core {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &core.Core{
		DB:        &core.Database{Bolt: bolt},
		Scheduler: core.NewScheduler(logger),
		Services:  &core.Services{Container: runtime},
		Events:    core.NewEventBus(logger),
		Logger:    logger,
	}
}

func TestModuleLifecycleAndRefresh(t *testing.T) {
	bolt := newTestBolt()
	if err := bolt.Set("cronjobs", "app-1", jobList{Jobs: []jobConfig{
		{ID: "enabled", Name: "Enabled", Schedule: "@every 1m", Command: "echo ok", Enabled: true},
		{ID: "disabled", Name: "Disabled", Schedule: "@every 1m", Command: "echo no", Enabled: false},
		{ID: "missing-command", Name: "Missing", Schedule: "@every 1m", Enabled: true},
	}}, 0); err != nil {
		t.Fatalf("seed cronjobs: %v", err)
	}
	c := testCore(bolt, &cronRuntime{})
	c.Scheduler.Add(&core.CronJob{ID: "appcron:app-2:old", Name: "old", Schedule: "@every 1m", Handler: func(context.Context) error { return nil }})

	m := New()
	if m.ID() != "cron" || m.Name() == "" || m.Version() == "" || len(m.Dependencies()) == 0 {
		t.Fatalf("unexpected module metadata")
	}
	if len(m.Routes()) != 0 || len(m.Events()) != 0 {
		t.Fatalf("unexpected module wiring")
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.Health() != core.HealthOK {
		t.Fatalf("Health = %s", m.Health())
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	var gotIDs []string
	for _, job := range c.Scheduler.Jobs() {
		gotIDs = append(gotIDs, job.ID)
	}
	if len(gotIDs) != 1 || gotIDs[0] != "appcron:app-1:enabled" {
		t.Fatalf("scheduler jobs = %v, want only enabled persisted job", gotIDs)
	}
}

func TestHandlerForExecutesAndStoresHistory(t *testing.T) {
	bolt := newTestBolt()
	runtime := &cronRuntime{
		containers: []core.ContainerInfo{{ID: "container-1234567890"}},
		output:     strings.Repeat("x", 70*1024),
	}
	m := &Module{
		core:   testCore(bolt, runtime),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "echo ok"})(context.Background())
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if got := strings.Join(runtime.cmd, " "); got != "echo ok" {
		t.Fatalf("exec cmd = %q", got)
	}
	keys, err := bolt.List("app_commands")
	if err != nil {
		t.Fatalf("list command history: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("history keys = %v, want one", keys)
	}
	var entry map[string]any
	if err := bolt.Get("app_commands", keys[0], &entry); err != nil {
		t.Fatalf("get command history: %v", err)
	}
	if entry["success"] != true {
		t.Fatalf("entry success = %v, want true", entry["success"])
	}
	if output := entry["output"].(string); !strings.Contains(output, "[output truncated]") {
		t.Fatalf("expected truncated output marker")
	}
	if id := entry["container_id"]; id != "container-12" {
		t.Fatalf("container_id = %v, want container-12", id)
	}
}

func TestHandlerForBlocksShellEvalCommand(t *testing.T) {
	runtime := &cronRuntime{
		containers: []core.ContainerInfo{{ID: "container-1234567890"}},
	}
	m := &Module{
		core:   testCore(newTestBolt(), runtime),
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "bash -c id"})(context.Background())
	if err == nil {
		t.Fatal("expected blocked command error")
	}
	if len(runtime.cmd) != 0 {
		t.Fatalf("runtime Exec called with %v", runtime.cmd)
	}
}

func TestHandlerForErrorPathsAndHelpers(t *testing.T) {
	m := &Module{core: testCore(newTestBolt(), nil), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "true"})(context.Background()); err == nil {
		t.Fatal("expected nil runtime error")
	}

	m.core.Services.Container = &cronRuntime{}
	if err := m.handlerFor("app-1", jobConfig{ID: "job-1", Command: "true"})(context.Background()); err == nil {
		t.Fatal("expected no container error")
	}

	if got := shortID("abc"); got != "abc" {
		t.Fatalf("shortID short = %q", got)
	}
	if got := shortID("abcdefghijklmnop"); got != "abcdefghijkl" {
		t.Fatalf("shortID long = %q", got)
	}
	if got := capOutput("abc", 10); got != "abc" {
		t.Fatalf("capOutput short = %q", got)
	}
	if got := capOutput("abcdef", 3); !strings.Contains(got, "[output truncated]") {
		t.Fatalf("capOutput long = %q", got)
	}
}
