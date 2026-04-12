package webhooks

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

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
