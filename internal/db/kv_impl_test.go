package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// =============================================================================
// BoltStore.GetAPIKeyByPrefix tests
// =============================================================================

func TestBoltKV_GetAPIKeyByPrefix_Found(t *testing.T) {
	store := testBolt(t)

	// Insert raw JSON directly since APIKey.KeyHash has json:"-" tag
	rawJSON := `{"id":"ak-123","user_id":"user-1","tenant_id":"tenant-1","name":"Test Key","key_hash":"hashed-secret-key","key_prefix":"dm_test","scopes_json":"[]","created_at":"2024-01-01T00:00:00Z"}`
	insertKVRaw(t, store, "api_keys", "ak-123", rawJSON, 0)

	ctx := context.Background()
	got, err := store.GetAPIKeyByPrefix(ctx, "dm_test")
	if err != nil {
		t.Fatalf("GetAPIKeyByPrefix: %v", err)
	}

	if got.ID != "ak-123" {
		t.Errorf("ID = %q, want ak-123", got.ID)
	}
	if got.KeyPrefix != "dm_test" {
		t.Errorf("KeyPrefix = %q, want dm_test", got.KeyPrefix)
	}
	if got.UserID != "user-1" {
		t.Errorf("UserID = %q, want user-1", got.UserID)
	}
}

func TestBoltKV_GetAPIKeyByPrefix_NotFound(t *testing.T) {
	store := testBolt(t)

	ctx := context.Background()
	_, err := store.GetAPIKeyByPrefix(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent prefix")
	}
}

func TestBoltKV_GetAPIKeyByPrefix_CorruptJSON(t *testing.T) {
	store := testBolt(t)

	insertKVRaw(t, store, "api_keys", "ak-corrupt", "not valid json {{", 0)

	ctx := context.Background()
	_, err := store.GetAPIKeyByPrefix(ctx, "any-prefix")
	// Should not find a match since JSON parsing fails
	if err == nil {
		t.Fatal("expected error when no valid key found")
	}
}

func TestBoltKV_GetAPIKeyByPrefix_MultipleKeys(t *testing.T) {
	store := testBolt(t)

	// Insert multiple API keys
	insertKVRaw(t, store, "api_keys", "ak-1", `{"id":"ak-1","key_prefix":"dm_alpha","user_id":"user-1","tenant_id":"t1","name":"Key 1","scopes_json":"[]","created_at":"2024-01-01T00:00:00Z"}`, 0)
	insertKVRaw(t, store, "api_keys", "ak-2", `{"id":"ak-2","key_prefix":"dm_beta","user_id":"user-2","tenant_id":"t1","name":"Key 2","scopes_json":"[]","created_at":"2024-01-01T00:00:00Z"}`, 0)

	ctx := context.Background()

	// Find first key
	got1, err := store.GetAPIKeyByPrefix(ctx, "dm_alpha")
	if err != nil {
		t.Fatalf("GetAPIKeyByPrefix dm_alpha: %v", err)
	}
	if got1.ID != "ak-1" {
		t.Errorf("ID = %q, want ak-1", got1.ID)
	}

	// Find second key
	got2, err := store.GetAPIKeyByPrefix(ctx, "dm_beta")
	if err != nil {
		t.Fatalf("GetAPIKeyByPrefix dm_beta: %v", err)
	}
	if got2.ID != "ak-2" {
		t.Errorf("ID = %q, want ak-2", got2.ID)
	}
}

// =============================================================================
// BoltStore.GetWebhookSecret tests
// =============================================================================

func TestBoltKV_GetWebhookSecret_Found(t *testing.T) {
	store := testBolt(t)

	// Insert raw JSON directly. GetWebhookSecret also supports normal
	// BoltStore.Set records.
	rawJSON := `{"id":"wh-123","app_id":"app-1","secret_hash":"super-secret-hash-abc","events_json":"[\"push\"]","branch_filter":"main","auto_deploy":true,"status":"active","created_at":"2024-01-01T00:00:00Z"}`
	insertKVRaw(t, store, "webhooks", "wh-123", rawJSON, 0)

	got, err := store.GetWebhookSecret("wh-123")
	if err != nil {
		t.Fatalf("GetWebhookSecret: %v", err)
	}

	if got != "super-secret-hash-abc" {
		t.Fatalf("secret = %q, want super-secret-hash-abc", got)
	}
}

func TestBoltKV_GetWebhookSecret_SetRecord(t *testing.T) {
	store := testBolt(t)

	record := struct {
		ID         string `json:"id"`
		AppID      string `json:"app_id"`
		SecretHash string `json:"secret_hash"`
		Status     string `json:"status"`
	}{
		ID:         "wh-set",
		AppID:      "app-1",
		SecretHash: "rotated-secret",
		Status:     "active",
	}
	if err := store.Set("webhooks", "wh-set", record, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := store.GetWebhookSecret("wh-set")
	if err != nil {
		t.Fatalf("GetWebhookSecret: %v", err)
	}
	if got != "rotated-secret" {
		t.Fatalf("secret = %q, want rotated-secret", got)
	}
}

func TestBoltKV_GetWebhookSecret_NotFound(t *testing.T) {
	store := testBolt(t)

	_, err := store.GetWebhookSecret("nonexistent-wh")
	if err == nil {
		t.Fatal("expected error for non-existent webhook")
	}
}

func TestBoltKV_GetWebhookSecret_CorruptJSON(t *testing.T) {
	store := testBolt(t)

	insertKVRaw(t, store, "webhooks", "wh-corrupt", "not valid json {{", 0)

	_, err := store.GetWebhookSecret("wh-corrupt")
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func insertKVRaw(t *testing.T, store *BoltStore, bucket, key, raw string, expiresAt int64) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, bucket); err != nil {
		t.Fatalf("insert raw kv bucket %s: %v", bucket, err)
	}
	_, err := store.db.Exec(`
		INSERT INTO kv_store(bucket, key, data, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(bucket, key) DO UPDATE SET
			data = excluded.data,
			expires_at = excluded.expires_at
	`, bucket, key, []byte(raw), expiresAt)
	if err != nil {
		t.Fatalf("insert raw kv %s/%s: %v", bucket, key, err)
	}
}

// =============================================================================
// Verify models.APIKey and models.Webhook structures match JSON
// =============================================================================

func TestBoltKV_APIKeyJSONRoundTrip(t *testing.T) {
	// Verify that we can parse our raw JSON into the struct
	rawJSON := `{"id":"ak-rt","user_id":"user-rt","tenant_id":"tenant-rt","name":"Round Trip Key","key_prefix":"dm_rt","scopes_json":"[\"read\",\"write\"]","created_at":"2024-01-01T00:00:00Z"}`

	var key models.APIKey
	if err := json.Unmarshal([]byte(rawJSON), &key); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if key.ID != "ak-rt" {
		t.Errorf("ID = %q, want ak-rt", key.ID)
	}
	if key.KeyPrefix != "dm_rt" {
		t.Errorf("KeyPrefix = %q, want dm_rt", key.KeyPrefix)
	}
	// KeyHash should be empty due to json:"-" tag
	if key.KeyHash != "" {
		t.Errorf("KeyHash = %q, want empty (json:'-' tag)", key.KeyHash)
	}
}

func TestBoltKV_WebhookJSONRoundTrip(t *testing.T) {
	// Verify that we can parse our raw JSON into the struct
	rawJSON := `{"id":"wh-rt","app_id":"app-rt","secret_hash":"should-be-ignored","events_json":"[\"push\"]","branch_filter":"main","auto_deploy":true,"status":"active","created_at":"2024-01-01T00:00:00Z"}`

	var wh models.Webhook
	if err := json.Unmarshal([]byte(rawJSON), &wh); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if wh.ID != "wh-rt" {
		t.Errorf("ID = %q, want wh-rt", wh.ID)
	}
	if wh.AppID != "app-rt" {
		t.Errorf("AppID = %q, want app-rt", wh.AppID)
	}
	// SecretHash should be empty due to json:"-" tag
	if wh.SecretHash != "" {
		t.Errorf("SecretHash = %q, want empty (json:'-' tag)", wh.SecretHash)
	}
}
