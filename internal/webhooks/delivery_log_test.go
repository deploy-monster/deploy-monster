package webhooks

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

type mockBoltStore struct {
	data map[string]map[string][]byte
}

func newMockBolt() *mockBoltStore {
	return &mockBoltStore{data: make(map[string]map[string][]byte)}
}

func (m *mockBoltStore) Set(bucket, key string, value any, _ int64) error {
	if m.data[bucket] == nil {
		m.data[bucket] = make(map[string][]byte)
	}
	b, _ := json.Marshal(value)
	m.data[bucket][key] = b
	return nil
}

func (m *mockBoltStore) Get(bucket, key string, dest any) error {
	if m.data[bucket] == nil {
		return nil
	}
	b, ok := m.data[bucket][key]
	if !ok {
		return nil
	}
	return json.Unmarshal(b, dest)
}

func (m *mockBoltStore) Delete(bucket, key string) error {
	if m.data[bucket] != nil {
		delete(m.data[bucket], key)
	}
	return nil
}

func (m *mockBoltStore) List(bucket string) ([]string, error) {
	keys := make([]string, 0)
	if m.data[bucket] != nil {
		for k := range m.data[bucket] {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

func (m *mockBoltStore) Close() error { return nil }

func (m *mockBoltStore) BatchSet(items []core.BoltBatchItem) error {
	for _, item := range items {
		if err := m.Set(item.Bucket, item.Key, item.Value, 0); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockBoltStore) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, nil
}

func (m *mockBoltStore) GetWebhookSecret(_ string) (string, error) {
	return "", nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestDeliveryTracker_RecordAndRetrieve(t *testing.T) {
	bolt := newMockBolt()
	events := core.NewEventBus(testLogger())

	tracker := NewDeliveryTracker(bolt, events)
	tracker.Start()

	// Emit a failure event
	events.Publish(context.Background(), core.NewEvent(
		core.EventOutboundFailed, "test",
		core.NotificationEventData{
			Channel:   "webhook",
			Recipient: "https://example.com/hook",
			Error:     "connection refused",
		},
	))

	// Give async handler time to process
	time.Sleep(50 * time.Millisecond)

	failures, err := tracker.RecentFailures(10)
	if err != nil {
		t.Fatalf("RecentFailures error: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].URL != "https://example.com/hook" {
		t.Errorf("URL = %q, want https://example.com/hook", failures[0].URL)
	}
	if failures[0].Error != "connection refused" {
		t.Errorf("Error = %q, want connection refused", failures[0].Error)
	}
}

func TestDeliveryTracker_RecordSuccess(t *testing.T) {
	bolt := newMockBolt()
	events := core.NewEventBus(testLogger())

	tracker := NewDeliveryTracker(bolt, events)
	tracker.Start()

	events.Publish(context.Background(), core.NewEvent(
		core.EventOutboundSent, "test",
		core.NotificationEventData{
			Channel:   "webhook",
			Recipient: "https://example.com/hook",
		},
	))

	time.Sleep(50 * time.Millisecond)

	// Success deliveries should not appear in RecentFailures
	failures, err := tracker.RecentFailures(10)
	if err != nil {
		t.Fatalf("RecentFailures error: %v", err)
	}
	if len(failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(failures))
	}

	// But it should be stored in the bucket
	keys, _ := bolt.List(deliveryLogBucket)
	if len(keys) != 1 {
		t.Errorf("expected 1 entry in bucket, got %d", len(keys))
	}
}

func TestDeliveryTracker_Cleanup(t *testing.T) {
	bolt := newMockBolt()
	events := core.NewEventBus(testLogger())

	tracker := NewDeliveryTracker(bolt, events)

	// Manually record an old entry
	_ = tracker.record(DeliveryLog{
		ID:        "old-1",
		URL:       "https://old.example.com",
		Status:    "failed",
		Error:     "timeout",
		Timestamp: time.Now().Add(-48 * time.Hour).Unix(),
	})

	// And a recent one
	_ = tracker.record(DeliveryLog{
		ID:        "new-1",
		URL:       "https://new.example.com",
		Status:    "failed",
		Error:     "500",
		Timestamp: time.Now().Unix(),
	})

	deleted, err := tracker.Cleanup(24 * time.Hour)
	if err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	keys, _ := bolt.List(deliveryLogBucket)
	if len(keys) != 1 {
		t.Errorf("expected 1 remaining entry, got %d", len(keys))
	}
}
