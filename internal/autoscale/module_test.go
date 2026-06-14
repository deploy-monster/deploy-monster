package autoscale

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

type testBolt struct {
	data    map[string]map[string][]byte
	getErr  error
	listErr error
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
	if b.getErr != nil {
		return b.getErr
	}
	raw, ok := b.data[bucket][key]
	if !ok {
		return fmt.Errorf("key %q: %w", key, core.ErrKVNotFound)
	}
	return json.Unmarshal(raw, dest)
}

func (b *testBolt) Delete(bucket, key string) error {
	delete(b.data[bucket], key)
	return nil
}

func (b *testBolt) List(bucket string) ([]string, error) {
	if b.listErr != nil {
		return nil, b.listErr
	}
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

type autoscaleStore struct {
	core.Store
	app       *core.Application
	getErr    error
	updated   *core.Application
	updateErr error
}

func (s *autoscaleStore) GetApp(context.Context, string) (*core.Application, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.app == nil {
		return nil, errors.New("not found")
	}
	cp := *s.app
	return &cp, nil
}

func (s *autoscaleStore) UpdateApp(_ context.Context, app *core.Application) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	cp := *app
	s.updated = &cp
	return nil
}

type autoscaleRuntime struct {
	core.ContainerRuntime
	containers []core.ContainerInfo
	stats      map[string]*core.ContainerStats
	listErr    error
}

func (r *autoscaleRuntime) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.containers, nil
}

func (r *autoscaleRuntime) Stats(_ context.Context, id string) (*core.ContainerStats, error) {
	return r.stats[id], nil
}

func testModule(store core.Store, runtime core.ContainerRuntime, bolt core.BoltStorer) *Module {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Module{
		core: &core.Core{
			DB:       &core.Database{Bolt: bolt},
			Store:    store,
			Services: &core.Services{Container: runtime},
			Events:   core.NewEventBus(logger),
			Logger:   logger,
		},
		logger: logger,
	}
}

func TestModuleLifecycle(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := New()
	c := &core.Core{Logger: logger, DB: &core.Database{Bolt: newTestBolt()}, Services: &core.Services{}, Events: core.NewEventBus(logger)}

	if m.ID() != "autoscale" || m.Name() == "" || m.Version() == "" {
		t.Fatalf("unexpected module metadata")
	}
	if len(m.Dependencies()) == 0 || len(m.Routes()) != 0 || len(m.Events()) != 0 {
		t.Fatalf("unexpected module wiring")
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("idempotent Start: %v", err)
	}
	if m.Health() != core.HealthOK {
		t.Fatalf("Health = %s", m.Health())
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("idempotent Stop: %v", err)
	}
}

func TestEvaluateAllTreatsMissingAutoscaleBucketAsEmpty(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bolt := &testBolt{
		data:    make(map[string]map[string][]byte),
		listErr: fmt.Errorf("bucket %q: %w", "autoscale", core.ErrKVNotFound),
	}
	m := testModule(&autoscaleStore{}, &autoscaleRuntime{}, bolt)
	m.logger = logger

	m.evaluateAll()

	if logs.Len() != 0 {
		t.Fatalf("missing autoscale bucket should not warn, got logs: %q", logs.String())
	}
}

func TestEvaluateAllLogsConfigReadError(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	bolt := newTestBolt()
	if err := bolt.Set("autoscale", "app-1", autoscaleConfig{Enabled: true}, 0); err != nil {
		t.Fatalf("seed autoscale config: %v", err)
	}
	bolt.getErr = errors.New("decode failed")
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Replicas: 1}}
	m := testModule(store, &autoscaleRuntime{}, bolt)
	m.logger = logger

	m.evaluateAll()

	if !bytes.Contains(logs.Bytes(), []byte("autoscale: get config entry failed")) {
		t.Fatalf("expected autoscale config warning, got logs: %q", logs.String())
	}
	if store.updated != nil {
		t.Fatalf("corrupt autoscale config should not update app, got %#v", store.updated)
	}
}

func TestEvaluateScaleUpPersistsDecisionAndUpdatesApp(t *testing.T) {
	bolt := newTestBolt()
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 1}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}, {ID: "ctr-2"}},
		stats: map[string]*core.ContainerStats{
			"ctr-1": {CPUPercent: 90, MemoryPercent: 40},
			"ctr-2": {CPUPercent: 70, MemoryPercent: 50},
		},
	}
	m := testModule(store, runtime, bolt)

	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 1, MaxReplicas: 3, CPUTarget: 75, RAMTarget: 90})

	if store.updated == nil || store.updated.Replicas != 2 {
		t.Fatalf("updated replicas = %#v, want 2", store.updated)
	}
	got, ok := m.lastDecision("app-1")
	if !ok {
		t.Fatal("expected persisted decision")
	}
	if got.Action != "scale_up" || got.DesiredReps != 2 {
		t.Fatalf("decision = %#v, want scale_up to 2", got)
	}
}

func TestEvaluateScaleUpWithNilEventBusStillPersistsDecision(t *testing.T) {
	bolt := newTestBolt()
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Name: "api", Status: "running", Replicas: 1}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}},
		stats:      map[string]*core.ContainerStats{"ctr-1": {CPUPercent: 95, MemoryPercent: 40}},
	}
	m := testModule(store, runtime, bolt)
	m.core.Events = nil

	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 1, MaxReplicas: 3, CPUTarget: 75, RAMTarget: 90})

	if store.updated == nil || store.updated.Replicas != 2 {
		t.Fatalf("updated replicas = %#v, want 2", store.updated)
	}
	got, ok := m.lastDecision("app-1")
	if !ok || got.Action != "scale_up" {
		t.Fatalf("decision = %#v, ok=%v, want scale_up", got, ok)
	}
}

func TestEvaluateCooldownDoesNotUpdateApp(t *testing.T) {
	bolt := newTestBolt()
	previous := decision{AppID: "app-1", Action: "scale_up", EvaluatedAt: time.Now().UTC()}
	if err := bolt.Set(decisionBucket, "app-1", previous, decisionTTL); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	store := &autoscaleStore{app: &core.Application{ID: "app-1", Replicas: 1}}
	runtime := &autoscaleRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1"}},
		stats:      map[string]*core.ContainerStats{"ctr-1": {CPUPercent: 99, MemoryPercent: 10}},
	}
	m := testModule(store, runtime, bolt)

	m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 1, MaxReplicas: 4, CPUTarget: 50, RAMTarget: 90, ScaleUpDelay: 60})

	if store.updated != nil {
		t.Fatalf("app updated during cooldown: %#v", store.updated)
	}
	got, ok := m.lastDecision("app-1")
	if !ok || got.Action != "cooldown" {
		t.Fatalf("decision = %#v, ok=%v, want cooldown", got, ok)
	}
}

func TestEvaluateSkipPaths(t *testing.T) {
	tests := []struct {
		name    string
		store   core.Store
		runtime core.ContainerRuntime
		reason  string
	}{
		{name: "no runtime", store: &autoscaleStore{app: &core.Application{ID: "app-1", Replicas: 1}}, reason: "container runtime not available"},
		{name: "app lookup", store: &autoscaleStore{getErr: errors.New("db down")}, runtime: &autoscaleRuntime{}, reason: "app lookup failed: db down"},
		{name: "no containers", store: &autoscaleStore{app: &core.Application{ID: "app-1", Replicas: 1}}, runtime: &autoscaleRuntime{}, reason: "no containers running"},
		{name: "no samples", store: &autoscaleStore{app: &core.Application{ID: "app-1", Replicas: 1}}, runtime: &autoscaleRuntime{containers: []core.ContainerInfo{{ID: "ctr-1"}}, stats: map[string]*core.ContainerStats{}}, reason: "no stats samples"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bolt := newTestBolt()
			m := testModule(tt.store, tt.runtime, bolt)
			m.evaluate("app-1", autoscaleConfig{Enabled: true, MinReplicas: 1, MaxReplicas: 2, CPUTarget: 50, RAMTarget: 50})
			got, ok := m.lastDecision("app-1")
			if !ok {
				t.Fatal("expected persisted decision")
			}
			if got.Action != "skip" || got.Reason != tt.reason {
				t.Fatalf("decision = %#v, want skip %q", got, tt.reason)
			}
		})
	}
}
