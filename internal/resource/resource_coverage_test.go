package resource

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// ═══════════════════════════════════════════════════════════════════════════════
// mockBolt implements core.BoltStorer for testing batchStoreMetrics/appendPoint
// ═══════════════════════════════════════════════════════════════════════════════

type mockBolt struct {
	data      map[string]map[string][]byte // bucket -> key -> json
	batchErr  error
	batchKeys []string // track batch set keys
}

func newMockBolt() *mockBolt {
	return &mockBolt{data: make(map[string]map[string][]byte)}
}

func (b *mockBolt) Set(bucket, key string, value any, _ int64) error {
	if b.data[bucket] == nil {
		b.data[bucket] = make(map[string][]byte)
	}
	raw, _ := json.Marshal(value)
	b.data[bucket][key] = raw
	return nil
}

func (b *mockBolt) BatchSet(items []core.BoltBatchItem) error {
	if b.batchErr != nil {
		return b.batchErr
	}
	for _, item := range items {
		b.batchKeys = append(b.batchKeys, item.Key)
		if b.data[item.Bucket] == nil {
			b.data[item.Bucket] = make(map[string][]byte)
		}
		raw, _ := json.Marshal(item.Value)
		b.data[item.Bucket][item.Key] = raw
	}
	return nil
}

func (b *mockBolt) Get(bucket, key string, dest any) error {
	if bkt, ok := b.data[bucket]; ok {
		if raw, ok := bkt[key]; ok {
			return json.Unmarshal(raw, dest)
		}
	}
	return nil // not found is not an error — returns zero value
}

func (b *mockBolt) Delete(_, _ string) error        { return nil }
func (b *mockBolt) List(_ string) ([]string, error) { return nil, nil }
func (b *mockBolt) Close() error                    { return nil }
func (b *mockBolt) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, nil
}
func (b *mockBolt) GetWebhookSecret(_ string) (string, error) { return "", nil }

// ═══════════════════════════════════════════════════════════════════════════════
// batchStoreMetrics — covers module.go:105 (11.8% → ~90%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestBatchStoreMetrics_ServerAndContainers(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{
		bolt:   bolt,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	server := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 45.5,
		RAMUsedMB:  2048,
	}

	containers := []core.ContainerMetrics{
		{
			AppID:       "app-1",
			Timestamp:   time.Now(),
			CPUPercent:  12.3,
			RAMUsedMB:   512,
			NetworkRxMB: 10,
			NetworkTxMB: 5,
		},
		{
			AppID:      "app-2",
			Timestamp:  time.Now(),
			CPUPercent: 8.0,
			RAMUsedMB:  256,
		},
	}

	m.batchStoreMetrics(server, containers)

	// Should have 3 batch items (1 server + 2 containers)
	if len(bolt.batchKeys) != 3 {
		t.Errorf("expected 3 batch keys, got %d: %v", len(bolt.batchKeys), bolt.batchKeys)
	}
}

func TestBatchStoreMetrics_ServerOnly(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{
		bolt:   bolt,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	server := &core.ServerMetrics{
		ServerID:   "srv-1",
		Timestamp:  time.Now(),
		CPUPercent: 80.0,
		RAMUsedMB:  4096,
	}

	m.batchStoreMetrics(server, nil)

	if len(bolt.batchKeys) != 1 {
		t.Errorf("expected 1 batch key, got %d", len(bolt.batchKeys))
	}
}

func TestBatchStoreMetrics_ContainersOnly(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{
		bolt:   bolt,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	containers := []core.ContainerMetrics{
		{AppID: "app-x", Timestamp: time.Now(), CPUPercent: 5.0, RAMUsedMB: 128},
	}

	m.batchStoreMetrics(nil, containers)

	if len(bolt.batchKeys) != 1 {
		t.Errorf("expected 1 batch key, got %d", len(bolt.batchKeys))
	}
}

func TestBatchStoreMetrics_NilBolt(t *testing.T) {
	m := &Module{
		bolt:   nil,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Should not panic
	m.batchStoreMetrics(&core.ServerMetrics{ServerID: "s1"}, nil)
}

func TestBatchStoreMetrics_EmptyContainerAppID(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{
		bolt:   bolt,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	containers := []core.ContainerMetrics{
		{AppID: "", Timestamp: time.Now(), CPUPercent: 5.0}, // empty AppID → skip
		{AppID: "app-valid", Timestamp: time.Now(), CPUPercent: 10.0},
	}

	m.batchStoreMetrics(nil, containers)

	if len(bolt.batchKeys) != 1 {
		t.Errorf("expected 1 batch key (empty AppID skipped), got %d", len(bolt.batchKeys))
	}
}

func TestBatchStoreMetrics_NothingToStore(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{
		bolt:   bolt,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// nil server, empty containers
	m.batchStoreMetrics(nil, nil)

	if len(bolt.batchKeys) != 0 {
		t.Errorf("expected 0 batch keys, got %d", len(bolt.batchKeys))
	}
}

func TestBatchStoreMetrics_BatchSetError(t *testing.T) {
	bolt := newMockBolt()
	bolt.batchErr = core.ErrNotFound // simulate error
	m := &Module{
		bolt:   bolt,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Should not panic — error is logged
	m.batchStoreMetrics(&core.ServerMetrics{ServerID: "s1", Timestamp: time.Now()}, nil)
}

// ═══════════════════════════════════════════════════════════════════════════════
// appendPoint — covers module.go:149 (0% → 100%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestAppendPoint_NewKey(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{bolt: bolt}

	point := metricsPoint{
		Timestamp:  time.Now(),
		CPUPercent: 50.0,
		MemoryMB:   1024,
	}

	ring := m.appendPoint("test:key", point)

	if len(ring.Points) != 1 {
		t.Errorf("expected 1 point, got %d", len(ring.Points))
	}
	if ring.Points[0].CPUPercent != 50.0 {
		t.Errorf("CPUPercent = %f, want 50.0", ring.Points[0].CPUPercent)
	}
}

func TestAppendPoint_ExistingRing(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{bolt: bolt}

	// Pre-populate with 2 points
	existingRing := metricsRing{
		Points: []metricsPoint{
			{Timestamp: time.Now().Add(-2 * time.Minute), CPUPercent: 10.0},
			{Timestamp: time.Now().Add(-1 * time.Minute), CPUPercent: 20.0},
		},
	}
	bolt.Set("metrics_ring", "test:key", existingRing, 0)

	ring := m.appendPoint("test:key", metricsPoint{
		Timestamp:  time.Now(),
		CPUPercent: 30.0,
	})

	if len(ring.Points) != 3 {
		t.Errorf("expected 3 points, got %d", len(ring.Points))
	}
}

func TestAppendPoint_TrimToMax(t *testing.T) {
	bolt := newMockBolt()
	m := &Module{bolt: bolt}

	// Pre-populate with maxRingPoints points
	existing := metricsRing{Points: make([]metricsPoint, maxRingPoints)}
	for i := range existing.Points {
		existing.Points[i] = metricsPoint{
			Timestamp:  time.Now().Add(time.Duration(-maxRingPoints+i) * time.Minute),
			CPUPercent: float64(i),
		}
	}
	bolt.Set("metrics_ring", "full:key", existing, 0)

	ring := m.appendPoint("full:key", metricsPoint{
		Timestamp:  time.Now(),
		CPUPercent: 99.9,
	})

	// Should trim to maxRingPoints
	if len(ring.Points) != maxRingPoints {
		t.Errorf("expected %d points after trim, got %d", maxRingPoints, len(ring.Points))
	}

	// Last point should be the newly added one
	if ring.Points[len(ring.Points)-1].CPUPercent != 99.9 {
		t.Errorf("last point CPUPercent = %f, want 99.9", ring.Points[len(ring.Points)-1].CPUPercent)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// collectOnce with BBolt — integration test
// ═══════════════════════════════════════════════════════════════════════════════

func TestCollectOnce_WithBolt(t *testing.T) {
	bolt := newMockBolt()

	rt := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Name: "web", Status: "running", Labels: map[string]string{"monster.app.id": "app-1"}},
		},
		stats: &core.ContainerStats{CPUPercent: 15.0, MemoryUsage: 256 * 1024 * 1024},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := &Module{
		collector: NewCollector(rt, logger),
		alerter:   NewAlertEngine(core.NewEventBus(logger), logger),
		bolt:      bolt,
		logger:    logger,
	}

	m.collectOnce()

	// Should have stored at least server metrics
	if len(bolt.batchKeys) == 0 {
		t.Error("expected at least 1 batch key from collectOnce")
	}
}
