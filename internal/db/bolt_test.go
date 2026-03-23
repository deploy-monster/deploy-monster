package db

import (
	"path/filepath"
	"testing"
	"time"
)

func testBolt(t *testing.T) *BoltStore {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bolt")

	store, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestBolt_SetAndGet(t *testing.T) {
	store := testBolt(t)

	type data struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	// Set
	err := store.Set("sessions", "key1", data{Name: "test", Value: 42}, 0)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Get
	var got data
	err = store.Get("sessions", "key1", &got)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.Name != "test" || got.Value != 42 {
		t.Errorf("expected {test, 42}, got {%s, %d}", got.Name, got.Value)
	}
}

func TestBolt_Get_NotFound(t *testing.T) {
	store := testBolt(t)

	var dest string
	err := store.Get("sessions", "nonexistent", &dest)
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestBolt_TTL_Expired(t *testing.T) {
	store := testBolt(t)

	// Set with 2 second TTL
	store.Set("sessions", "expiring", "value", 2)

	// Should work immediately
	var got string
	if err := store.Get("sessions", "expiring", &got); err != nil {
		t.Fatalf("Get before expiry: %v", err)
	}

	// Wait for expiry
	time.Sleep(2100 * time.Millisecond)

	err := store.Get("sessions", "expiring", &got)
	if err == nil {
		t.Error("expected error for expired key")
	}
}

func TestBolt_TTL_NoExpiry(t *testing.T) {
	store := testBolt(t)

	// TTL 0 = no expiry
	store.Set("sessions", "forever", "value", 0)

	var got string
	if err := store.Get("sessions", "forever", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
}

func TestBolt_Delete(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "todelete", "value", 0)

	if err := store.Delete("sessions", "todelete"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var got string
	err := store.Get("sessions", "todelete", &got)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestBolt_InvalidBucket(t *testing.T) {
	store := testBolt(t)

	err := store.Get("nonexistent_bucket", "key", nil)
	if err == nil {
		t.Error("expected error for nonexistent bucket")
	}
}
