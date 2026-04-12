package db

import (
	"testing"
)

// ---------- KV store operations ----------

func TestBoltExtra_SetAndGet_StringValue(t *testing.T) {
	store := testBolt(t)

	if err := store.Set("sessions", "greeting", "hello world", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got string
	if err := store.Get("sessions", "greeting", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestBoltExtra_SetAndGet_IntValue(t *testing.T) {
	store := testBolt(t)

	if err := store.Set("sessions", "counter", 42, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got int
	if err := store.Get("sessions", "counter", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestBoltExtra_SetAndGet_StructValue(t *testing.T) {
	store := testBolt(t)

	type Config struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		TLS  bool   `json:"tls"`
	}

	input := Config{Host: "localhost", Port: 8080, TLS: true}
	if err := store.Set("sessions", "config", input, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got Config
	if err := store.Get("sessions", "config", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", got.Host)
	}
	if got.Port != 8080 {
		t.Errorf("expected port 8080, got %d", got.Port)
	}
	if !got.TLS {
		t.Error("expected TLS true")
	}
}

func TestBoltExtra_SetAndGet_SliceValue(t *testing.T) {
	store := testBolt(t)

	input := []string{"alpha", "bravo", "charlie"}
	if err := store.Set("sessions", "tags", input, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got []string
	if err := store.Get("sessions", "tags", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0] != "alpha" || got[1] != "bravo" || got[2] != "charlie" {
		t.Errorf("unexpected values: %v", got)
	}
}

func TestBoltExtra_SetAndGet_MapValue(t *testing.T) {
	store := testBolt(t)

	input := map[string]int{"a": 1, "b": 2, "c": 3}
	if err := store.Set("sessions", "counts", input, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got map[string]int
	if err := store.Get("sessions", "counts", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got["a"] != 1 || got["b"] != 2 || got["c"] != 3 {
		t.Errorf("unexpected values: %v", got)
	}
}

func TestBoltExtra_Overwrite(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "key", "first", 0)
	store.Set("sessions", "key", "second", 0)

	var got string
	if err := store.Get("sessions", "key", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

// ---------- Bucket tests ----------

func TestBoltExtra_DefaultBuckets(t *testing.T) {
	store := testBolt(t)

	buckets := []string{"sessions", "ratelimit", "buildcache", "metrics_ring"}
	for _, bucket := range buckets {
		t.Run(bucket, func(t *testing.T) {
			// Should be able to write to each default bucket without error
			if err := store.Set(bucket, "test-key", "test-value", 0); err != nil {
				t.Errorf("Set to bucket %q failed: %v", bucket, err)
			}
			var got string
			if err := store.Get(bucket, "test-key", &got); err != nil {
				t.Errorf("Get from bucket %q failed: %v", bucket, err)
			}
			if got != "test-value" {
				t.Errorf("expected 'test-value' from bucket %q, got %q", bucket, got)
			}
		})
	}
}

func TestBoltExtra_NonexistentBucket_Set(t *testing.T) {
	store := testBolt(t)

	err := store.Set("nonexistent_bucket", "key", "value", 0)
	if err == nil {
		t.Error("expected error when setting to nonexistent bucket")
	}
}

func TestBoltExtra_NonexistentBucket_Get(t *testing.T) {
	store := testBolt(t)

	var dest string
	err := store.Get("nonexistent_bucket", "key", &dest)
	if err == nil {
		t.Error("expected error when getting from nonexistent bucket")
	}
}

func TestBoltExtra_NonexistentBucket_Delete(t *testing.T) {
	store := testBolt(t)

	err := store.Delete("nonexistent_bucket", "key")
	if err == nil {
		t.Error("expected error when deleting from nonexistent bucket")
	}
}

// ---------- Delete tests ----------

func TestBoltExtra_Delete_NonexistentKey(t *testing.T) {
	store := testBolt(t)

	// Deleting a key that does not exist should not error (bbolt behavior)
	err := store.Delete("sessions", "never-existed")
	if err != nil {
		t.Errorf("Delete of nonexistent key should not error, got: %v", err)
	}
}

func TestBoltExtra_Delete_ThenGet(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "ephemeral", "data", 0)
	store.Delete("sessions", "ephemeral")

	var got string
	err := store.Get("sessions", "ephemeral", &got)
	if err == nil {
		t.Error("expected error after deleting key")
	}
}

// ---------- Persistence across operations ----------

func TestBoltExtra_PersistenceAcrossOperations(t *testing.T) {
	store := testBolt(t)

	// Write multiple keys across multiple buckets
	store.Set("sessions", "s1", "session-data", 0)
	store.Set("ratelimit", "r1", 100, 0)
	store.Set("buildcache", "b1", map[string]string{"hash": "abc123"}, 0)

	// Read them back — all should persist
	var s1 string
	if err := store.Get("sessions", "s1", &s1); err != nil {
		t.Fatalf("Get sessions/s1: %v", err)
	}
	if s1 != "session-data" {
		t.Errorf("expected 'session-data', got %q", s1)
	}

	var r1 int
	if err := store.Get("ratelimit", "r1", &r1); err != nil {
		t.Fatalf("Get ratelimit/r1: %v", err)
	}
	if r1 != 100 {
		t.Errorf("expected 100, got %d", r1)
	}

	var b1 map[string]string
	if err := store.Get("buildcache", "b1", &b1); err != nil {
		t.Fatalf("Get buildcache/b1: %v", err)
	}
	if b1["hash"] != "abc123" {
		t.Errorf("expected hash 'abc123', got %q", b1["hash"])
	}
}

func TestBoltExtra_PersistenceAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/persist.bolt"

	// Write
	store1, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	store1.Set("sessions", "persistent", "survives-close", 0)
	store1.Close()

	// Reopen and read
	store2, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer store2.Close()

	var got string
	if err := store2.Get("sessions", "persistent", &got); err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if got != "survives-close" {
		t.Errorf("expected 'survives-close', got %q", got)
	}
}

// ---------- Multiple keys in same bucket ----------

func TestBoltExtra_MultipleKeysInSameBucket(t *testing.T) {
	store := testBolt(t)

	keys := map[string]string{
		"key-1": "value-1",
		"key-2": "value-2",
		"key-3": "value-3",
		"key-4": "value-4",
		"key-5": "value-5",
	}

	for k, v := range keys {
		if err := store.Set("sessions", k, v, 0); err != nil {
			t.Fatalf("Set %q: %v", k, err)
		}
	}

	for k, expected := range keys {
		var got string
		if err := store.Get("sessions", k, &got); err != nil {
			t.Fatalf("Get %q: %v", k, err)
		}
		if got != expected {
			t.Errorf("key %q: expected %q, got %q", k, expected, got)
		}
	}
}

// ---------- TTL edge cases ----------

func TestBoltExtra_TTL_ZeroMeansNoExpiry(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "no-ttl", "forever", 0)

	var got string
	if err := store.Get("sessions", "no-ttl", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "forever" {
		t.Errorf("expected 'forever', got %q", got)
	}
}

func TestBoltExtra_TTL_LargeTTL(t *testing.T) {
	store := testBolt(t)

	// Set with a very large TTL (1 year in seconds)
	if err := store.Set("sessions", "long-lived", "data", 365*24*3600); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var got string
	if err := store.Get("sessions", "long-lived", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "data" {
		t.Errorf("expected 'data', got %q", got)
	}
}
