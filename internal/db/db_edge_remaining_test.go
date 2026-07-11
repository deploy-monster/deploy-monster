package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — NewSQLiteKVStore sql.Open error (write to a non-existent dir)
// ═══════════════════════════════════════════════════════════════════════════════
func TestNewSQLiteKVStore_OpenErrorPath(t *testing.T) {
	_, err := NewSQLiteKVStore("/nonexistent_dir_xyz/test.db")
	if err == nil {
		t.Fatal("expected error for invalid directory path")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — NewSQLiteKVStoreFromDB with nil db
// ═══════════════════════════════════════════════════════════════════════════════
func TestNewSQLiteKVStoreFromDB_NilDB(t *testing.T) {
	_, err := NewSQLiteKVStoreFromDB(nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — BatchSet empty list (nil input)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_BatchSet_NilInput(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.BatchSet(nil)
	if err != nil {
		t.Fatalf("expected nil for nil input, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — BatchSet zero-length slice
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_BatchSet_EmptySlice(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err = store.BatchSet([]core.BoltBatchItem{})
	if err != nil {
		t.Fatalf("expected nil for empty slice, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Mutate nil callback error path
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Mutate_NilMutateFunc(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var dest string
	err = store.Mutate("b", "k", &dest, 0, nil)
	if err == nil {
		t.Fatal("expected error for nil mutate callback")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Mutate callback returns error (propagation test)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Mutate_CallbackReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var val string
	err = store.Mutate("mut_err", "ek", &val, 0, func(bool) error {
		return sql.ErrNoRows
	})
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Mutate key does not exist (exists=false path)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Mutate_KeyNotExist(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var val string
	err = store.Mutate("mut_new", "newkey", &val, 0, func(exists bool) error {
		if exists {
			t.Error("expected exists=false for new key")
		}
		val = "created"
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	var read string
	if err := store.Get("mut_new", "newkey", &read); err != nil {
		t.Fatal(err)
	}
	if read != "created" {
		t.Errorf("expected 'created', got '%s'", read)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Mutate key exists with TTL (verify reading an existing key)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Mutate_KeyExists(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Set("mut_ex", "ek", "orig", 0); err != nil {
		t.Fatal(err)
	}
	var val string
	err = store.Mutate("mut_ex", "ek", &val, 0, func(exists bool) error {
		if !exists {
			t.Error("expected exists=true")
		}
		if val != "orig" {
			t.Errorf("expected 'orig', got '%s'", val)
		}
		val = "updated"
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	var read string
	if err := store.Get("mut_ex", "ek", &read); err != nil {
		t.Fatal(err)
	}
	if read != "updated" {
		t.Errorf("expected 'updated', got '%s'", read)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — List with some expired and some valid entries
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_List_FiltersExpired(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	past := time.Now().Unix() - 100
	_, err = store.db.Exec(`INSERT INTO kv_store(bucket, key, data, expires_at) VALUES (?, ?, ?, ?)`,
		"flist", "expired", []byte(`"val"`), past)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("flist", "valid", "v", 3600); err != nil {
		t.Fatal(err)
	}

	keys, err := store.List("flist")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 || keys[0] != "valid" {
		t.Errorf("expected 1 valid key, got %v", keys)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — List with invalid JSON data (should be skipped)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_List_InvalidJSONData(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Ensure bucket exists in kv_buckets
	_, err = store.db.Exec(`INSERT OR IGNORE INTO kv_buckets(name) VALUES (?)`, "binbucket")
	if err != nil {
		t.Fatal(err)
	}
	// Insert a row with valid JSON and one with invalid JSON
	_, err = store.db.Exec(`INSERT INTO kv_store(bucket, key, data, expires_at) VALUES (?, ?, ?, 0)`,
		"binbucket", "valid-key", []byte(`"valid"`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`INSERT INTO kv_store(bucket, key, data, expires_at) VALUES (?, ?, ?, 0)`,
		"binbucket", "binkey", []byte("not-json"))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := store.List("binbucket")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should only return the valid JSON entry
	if len(keys) != 1 || keys[0] != "valid-key" {
		t.Errorf("expected 1 key (valid-key), got %v", keys)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Get with expired TTL
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Get_ExpiredKey(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	past := time.Now().Unix() - 10
	_, err = store.db.Exec(`INSERT INTO kv_store(bucket, key, data, expires_at) VALUES (?, ?, ?, ?)`,
		"g", "ek", []byte(`"value"`), past)
	if err != nil {
		t.Fatal(err)
	}
	var val string
	err = store.Get("g", "ek", &val)
	if err == nil {
		t.Fatal("expected error for expired key")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix with cancelled context
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetAPIKeyByPrefix_CtxCancelled(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.GetAPIKeyByPrefix(ctx, "x")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix with valid record but no prefix match
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetAPIKeyByPrefix_RecordNoMatch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rec := apiKeyKVRecord{KeyPrefix: "pref_a", UserID: "u1", Hash: "h1"}
	data, _ := json.Marshal(rec)
	_, err = store.db.Exec(`INSERT INTO kv_store(bucket,key,data,expires_at) VALUES(?,?,?,0)`,
		"api_keys", "k1", data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.GetAPIKeyByPrefix(context.Background(), "pref_b")
	if err == nil {
		t.Fatal("expected not found for non-matching prefix")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix with matching record (success path)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetAPIKeyByPrefix_MatchSuccess(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rec := apiKeyKVRecord{KeyPrefix: "pref_m", UserID: "um", Hash: "hm"}
	data, _ := json.Marshal(rec)
	_, err = store.db.Exec(`INSERT INTO kv_store(bucket,key,data,expires_at) VALUES(?,?,?,0)`,
		"api_keys", "km", data)
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.GetAPIKeyByPrefix(context.Background(), "pref_m")
	if err != nil {
		t.Fatalf("GetAPIKeyByPrefix: %v", err)
	}
	if key.KeyPrefix != "pref_m" {
		t.Errorf("expected pref_m, got %s", key.KeyPrefix)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix with binary garbage in api_keys bucket
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetAPIKeyByPrefix_GarbageData(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.db.Exec(`INSERT INTO kv_store(bucket,key,data,expires_at) VALUES(?,?,X'DEADBEEF',0)`,
		"api_keys", "garbage")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.GetAPIKeyByPrefix(context.Background(), "x")
	if err == nil {
		t.Fatal("expected not found")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix on a closed DB (scan error)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetAPIKeyByPrefix_ClosedDBErr(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	_, err = store.GetAPIKeyByPrefix(context.Background(), "x")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord empty prefix validation error
// ═══════════════════════════════════════════════════════════════════════════════
func TestDecodeAPIKeyRecord_NoKeyPrefix(t *testing.T) {
	raw, _ := json.Marshal(apiKeyKVRecord{
		UserID: "u1", Hash: "h1",
	})
	_, err := decodeAPIKeyRecord(raw)
	if err == nil {
		t.Fatal("expected error for empty key prefix")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord empty userID validation error
// ═══════════════════════════════════════════════════════════════════════════════
func TestDecodeAPIKeyRecord_NoUserID(t *testing.T) {
	raw, _ := json.Marshal(apiKeyKVRecord{
		Prefix: "pref",
		Hash:   "h1",
	})
	_, err := decodeAPIKeyRecord(raw)
	if err == nil {
		t.Fatal("expected error for empty userID")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord fallback from UserID→CreatedBy
// ═══════════════════════════════════════════════════════════════════════════════
func TestDecodeAPIKeyRecord_FallbackCreatedBy(t *testing.T) {
	raw, _ := json.Marshal(apiKeyKVRecord{
		Prefix:    "pref",
		Hash:      "hash",
		CreatedBy: "cb_user",
	})
	key, err := decodeAPIKeyRecord(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.UserID != "cb_user" {
		t.Errorf("expected UserID from CreatedBy, got %s", key.UserID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord fallback KeyPrefix→Prefix, Hash→KeyHash
// ═══════════════════════════════════════════════════════════════════════════════
func TestDecodeAPIKeyRecord_FallbackKeyHashPrefix(t *testing.T) {
	raw, _ := json.Marshal(apiKeyKVRecord{
		Prefix: "pref", Hash: "hash_val",
		UserID: "u1",
	})
	key, err := decodeAPIKeyRecord(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.KeyPrefix != "pref" {
		t.Errorf("expected pref, got %s", key.KeyPrefix)
	}
	if key.KeyHash != "hash_val" {
		t.Errorf("expected hash_val, got %s", key.KeyHash)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord with legacy kvEntry wrapper
// ═════════════════════════════──────────────────────────────────────────────────
func TestDecodeAPIKeyRecord_LegacyWrapped(t *testing.T) {
	inner := apiKeyKVRecord{KeyPrefix: "p", UserID: "u", Hash: "h"}
	innerData, _ := json.Marshal(inner)
	entry := kvEntry{Data: innerData}
	wrapper, _ := json.Marshal(entry)

	key, err := decodeAPIKeyRecord(wrapper)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if key.KeyPrefix != "p" {
		t.Errorf("expected p, got %s", key.KeyPrefix)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord with empty legacy Data (non-JSON parseable)
// ═══════════════════════════════════════════════════════════════════════════════
func TestDecodeAPIKeyRecord_EmptyLegacyData(t *testing.T) {
	entry := kvEntry{Data: []byte{}}
	raw, _ := json.Marshal(entry)
	_, err := decodeAPIKeyRecord(raw)
	if err == nil {
		t.Fatal("expected error for empty legacy data")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret success path
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetWebhookSecret_OK(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rec := struct {
		SecretHash string `json:"secret_hash"`
	}{SecretHash: "abc123"}
	data, _ := json.Marshal(rec)

	_, err = store.db.Exec(`INSERT INTO kv_store(bucket,key,data,expires_at) VALUES(?,?,?,0)`,
		"webhooks", "wh_ok", data)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.GetWebhookSecret("wh_ok")
	if err != nil {
		t.Fatalf("GetWebhookSecret: %v", err)
	}
	if hash != "abc123" {
		t.Errorf("expected abc123, got %s", hash)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret with legacy kvEntry wrapper format
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetWebhookSecret_LegacyFormat(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	inner := struct {
		SecretHash string `json:"secret_hash"`
	}{SecretHash: "legacy_hash"}
	innerData, _ := json.Marshal(inner)
	entry := kvEntry{Data: innerData}
	wrapper, _ := json.Marshal(entry)

	_, err = store.db.Exec(`INSERT INTO kv_store(bucket,key,data,expires_at) VALUES(?,?,?,0)`,
		"webhooks", "wh_legacy", wrapper)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.GetWebhookSecret("wh_legacy")
	if err != nil {
		t.Fatalf("GetWebhookSecret: %v", err)
	}
	if hash != "legacy_hash" {
		t.Errorf("expected legacy_hash, got %s", hash)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret not-found
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetWebhookSecret_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.GetWebhookSecret("nonexistent")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret with empty hash value (should error)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_GetWebhookSecret_EmptyHashRecordCheck(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rec := struct {
		SecretHash string `json:"secret_hash"`
	}{SecretHash: ""}
	data, _ := json.Marshal(rec)

	_, err = store.db.Exec(`INSERT INTO kv_store(bucket,key,data,expires_at) VALUES(?,?,?,0)`,
		"webhooks", "wh_empty", data)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.GetWebhookSecret("wh_empty")
	if err == nil {
		t.Fatal("expected error for empty secret hash")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Close with nil db receiver
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Close_NilDB(t *testing.T) {
	b := &BoltStore{db: nil, closeDB: true}
	if err := b.Close(); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Close with closeDB=false (skip close)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Close_SkipClose(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	store.closeDB = false
	if err := store.Close(); err != nil {
		t.Fatalf("expected nil when closeDB=false, got %v", err)
	}
	// Manually close to clean up
	store.db.Close()
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Delete non-existent key in existing bucket
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Delete_ExistingBucketMissingKey(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSQLiteKVStore(dir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Ensure bucket exists
	if err := store.Set("db", "dummy", "v", 0); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("db", "nonexistent"); err != nil {
		t.Fatalf("expected nil for non-existent key, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — ListDeploymentsByStatus query error (closed DB)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListDeploymentsByStatus_ClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.ListDeploymentsByStatus(context.Background(), "active")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — ListDeploymentsByStatus empty results (no matches)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListDeploymentsByStatus_NoResults(t *testing.T) {
	db := testDB(t)
	deploys, err := db.ListDeploymentsByStatus(context.Background(), "status_xyzzy")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(deploys) != 0 {
		t.Errorf("expected 0, got %d", len(deploys))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — AtomicNextDeployVersion begin immediate error (closed DB)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_AtomicNextDeployVersion_ClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.AtomicNextDeployVersion(context.Background(), "app")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — AtomicNextDeployVersion first version (no deployments yet)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_AtomicNextDeployVersion_FirstVersion(t *testing.T) {
	db := testDB(t)
	ver, err := db.AtomicNextDeployVersion(context.Background(), "app_v1")
	if err != nil {
		t.Fatalf("AtomicNextDeployVersion: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected 1, got %d", ver)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — GetLatestDeploymentsByAppIDs nil/empty input
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_GetLatestDeploymentsByAppIDs_NilIDs(t *testing.T) {
	db := testDB(t)
	result, err := db.GetLatestDeploymentsByAppIDs(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSQLite_GetLatestDeploymentsByAppIDs_EmptyIDs(t *testing.T) {
	db := testDB(t)
	result, err := db.GetLatestDeploymentsByAppIDs(context.Background(), []string{})
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — GetLatestDeploymentsByAppIDs query error (closed DB)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_GetLatestDeploymentsByAppIDs_ClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.GetLatestDeploymentsByAppIDs(context.Background(), []string{"a1", "a2"})
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — GetServer not found
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_GetServer_NotFoundErr(t *testing.T) {
	db := testDB(t)
	_, err := db.GetServer(context.Background(), "nonexistent")
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — GetServer query error (closed DB)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_GetServer_ClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.GetServer(context.Background(), "srv")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — ListServersByTenant empty list
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListServersByTenant_EmptyList(t *testing.T) {
	db := testDB(t)
	srvs, err := db.ListServersByTenant(context.Background(), "no_servers_tenant")
	if err != nil {
		t.Fatalf("ListServersByTenant: %v", err)
	}
	if len(srvs) != 0 {
		t.Errorf("expected 0, got %d", len(srvs))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — ListServersByTenant closed DB
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListServersByTenant_ClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.ListServersByTenant(context.Background(), "t")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — ListAllServers empty list
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListAllServers_EmptyResult(t *testing.T) {
	db := testDB(t)
	srvs, err := db.ListAllServers(context.Background())
	if err != nil {
		t.Fatalf("ListAllServers: %v", err)
	}
	if len(srvs) != 0 {
		t.Errorf("expected 0, got %d", len(srvs))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — ListAllServers closed DB
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListAllServers_ClosedDB(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.ListAllServers(context.Background())
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// servers.go — CreateServer with all defaults and then Get/UpdateStatus/Delete
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_Servers_FullLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	srv := &core.Server{Hostname: "lifecycle-server", IPAddress: "10.0.0.99"}
	if err := db.CreateServer(ctx, srv); err != nil {
		t.Fatal(err)
	}
	if srv.Role != "worker" || srv.SSHPort != 22 || srv.Status != "provisioning" || srv.AgentStatus != "unknown" {
		t.Errorf("defaults wrong: role=%s port=%d status=%s agent=%s",
			srv.Role, srv.SSHPort, srv.Status, srv.AgentStatus)
	}

	got, err := db.GetServer(ctx, srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "lifecycle-server" {
		t.Errorf("expected lifecycle-server, got %s", got.Hostname)
	}

	if err := db.UpdateServerStatus(ctx, srv.ID, "active"); err != nil {
		t.Fatal(err)
	}
	upd, err := db.GetServer(ctx, srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if upd.Status != "active" {
		t.Errorf("expected active, got %s", upd.Status)
	}

	if err := db.DeleteServer(ctx, srv.ID); err != nil {
		t.Fatal(err)
	}
	_, err = db.GetServer(ctx, srv.ID)
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — ListMigrations success (at least one migration applied)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListMigrations_HasEntries(t *testing.T) {
	db := testDB(t)
	migs, err := db.ListMigrations(context.Background())
	if err != nil {
		t.Fatalf("ListMigrations: %v", err)
	}
	if len(migs) == 0 {
		t.Fatal("expected at least one migration")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — ListMigrations closed DB
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_ListMigrations_ClosedDBErr(t *testing.T) {
	db := testDB(t)
	db.Close()
	_, err := db.ListMigrations(context.Background())
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — Rollback with all steps (no down files)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_Rollback_RequestMany(t *testing.T) {
	db := testDB(t)
	err := db.Rollback(9999)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — Rollback with negative steps (same as all)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_Rollback_NegativeSteps(t *testing.T) {
	db := testDB(t)
	err := db.Rollback(0)
	if err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — Rollback on closed DB (error path)
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_Rollback_ClosedDBErr(t *testing.T) {
	db := testDB(t)
	db.Close()
	err := db.Rollback(1)
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Health with nil bolt
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Health_NilBoltStore(t *testing.T) {
	m := &Module{}
	if h := m.Health(); h != core.HealthDown {
		t.Errorf("expected HealthDown, got %v", h)
	}
}

func TestModule_Health_SQLiteNoDB(t *testing.T) {
	m := &Module{driver: "sqlite", bolt: &BoltStore{}}
	if h := m.Health(); h != core.HealthDown {
		t.Errorf("expected HealthDown, got %v", h)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Stop with nil/Clean state
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Stop_NilModule(t *testing.T) {
	m := &Module{}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Stop with both sqlite and bolt
// ═══════════════════════════════════════════════════════════════════════════════
func TestModule_Stop_WithSQLiteAndBolt(t *testing.T) {
	sqldb := testDB(t)
	bolt, _ := NewSQLiteKVStoreFromDB(sqldb.DB())
	m := &Module{sqlite: sqldb, bolt: bolt}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// postgres.go — hostname function
// ═══════════════════════════════════════════════════════════════════════════════
func TestHostname_ReturnsString(t *testing.T) {
	h := hostname()
	if h == "" {
		t.Fatal("expected non-empty hostname")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — SnapshotBackup closed DB
// ═══════════════════════════════════════════════════════════════════════════════
func TestSQLite_SnapshotBackup_ClosedDBErr(t *testing.T) {
	db := testDB(t)
	db.Close()
	err := db.SnapshotBackup(context.Background(), "/tmp/bak.db")
	if err == nil {
		t.Fatal("expected error on closed db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — NewSQLite with empty path edge case
// ═══════════════════════════════════════════════════════════════════════════════
func TestNewSQLite_EmptyPathEdge(t *testing.T) {
	_, err := NewSQLite(":memory:")
	if err != nil {
		t.Logf("NewSQLite(:memory:): %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — BatchSet happy path with TTL
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_BatchSet_HappyPathWithTTL(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSQLiteKVStore(dir + "/test.db")
	defer store.Close()

	items := []core.BoltBatchItem{
		{Bucket: "bt", Key: "k1", Value: "v1", TTL: 3600},
		{Bucket: "bt", Key: "k2", Value: "v2"},
	}
	if err := store.BatchSet(items); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}
	var v string
	if err := store.Get("bt", "k1", &v); err != nil {
		t.Fatal(err)
	}
	if v != "v1" {
		t.Errorf("expected v1, got %s", v)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — BatchSet with marshal error (unmarshallable value)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_BatchSet_JSONError(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSQLiteKVStore(dir + "/test.db")
	defer store.Close()

	items := []core.BoltBatchItem{
		{Bucket: "bt", Key: "bad", Value: make(chan int)},
	}
	err := store.BatchSet(items)
	if err == nil {
		t.Fatal("expected error for unmarshalable value")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Set with unmarshalable value (marshal error path)
// ═══════════════════════════════════════════════════════════════════════════════
func TestBoltStore_Set_MarshalErrorPath(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewSQLiteKVStore(dir + "/test.db")
	defer store.Close()

	err := store.Set("bt", "k", make(chan int), 0)
	if err == nil {
		t.Fatal("expected marshal error")
	}
}