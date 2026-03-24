package db

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "core.db" {
		t.Errorf("expected ID 'core.db', got %q", m.ID())
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "Database" {
		t.Errorf("expected Name 'Database', got %q", m.Name())
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", m.Version())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if deps != nil {
		t.Errorf("expected nil dependencies, got %v", deps)
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	routes := m.Routes()
	if routes != nil {
		t.Errorf("expected nil routes, got %v", routes)
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	events := m.Events()
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}
}

func TestModule_Health_Uninitialized(t *testing.T) {
	m := New()
	// With no sqlite or bolt initialized, Health should return HealthDown
	if m.Health() != core.HealthDown {
		t.Errorf("expected HealthDown for uninitialized module, got %v", m.Health())
	}
}

func TestModule_Health_WithStores(t *testing.T) {
	m := New()

	// Initialize both stores to get HealthOK
	db := testDB(t)
	bolt := testBolt(t)

	m.sqlite = db
	m.bolt = bolt

	if m.Health() != core.HealthOK {
		t.Errorf("expected HealthOK with both stores, got %v", m.Health())
	}
}

func TestModule_Health_MissingSQLite(t *testing.T) {
	m := New()
	m.bolt = testBolt(t)

	if m.Health() != core.HealthDown {
		t.Errorf("expected HealthDown without sqlite, got %v", m.Health())
	}
}

func TestModule_Health_MissingBolt(t *testing.T) {
	m := New()
	m.sqlite = testDB(t)

	if m.Health() != core.HealthDown {
		t.Errorf("expected HealthDown without bolt, got %v", m.Health())
	}
}

func TestModule_Store_ReturnsStoreInterface(t *testing.T) {
	m := New()
	db := testDB(t)
	m.sqlite = db

	store := m.Store()
	if store == nil {
		t.Fatal("Store() should not return nil when sqlite is set")
	}

	// Verify it's the same underlying instance
	if store != core.Store(db) {
		t.Error("Store() should return the same SQLiteDB as core.Store")
	}
}

func TestModule_SQLite_ReturnsInstance(t *testing.T) {
	m := New()
	db := testDB(t)
	m.sqlite = db

	if m.SQLite() != db {
		t.Error("SQLite() should return the set SQLiteDB instance")
	}
}

func TestModule_Bolt_ReturnsInstance(t *testing.T) {
	m := New()
	bolt := testBolt(t)
	m.bolt = bolt

	if m.Bolt() != bolt {
		t.Error("Bolt() should return the set BoltStore instance")
	}
}

func TestModule_StopIdempotent(t *testing.T) {
	m := New()
	// Should not panic or error when nothing is initialized
	if err := m.Stop(nil); err != nil {
		t.Errorf("Stop on uninitialized module should not error, got: %v", err)
	}
}

func TestModule_StopClosesStores(t *testing.T) {
	dir := t.TempDir()

	sqliteDB, err := NewSQLite(dir + "/module-stop.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}

	boltStore, err := NewBoltStore(dir + "/module-stop.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.sqlite = sqliteDB
	m.bolt = boltStore

	if err := m.Stop(nil); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestModule_StartNoError(t *testing.T) {
	m := New()
	if err := m.Start(nil); err != nil {
		t.Errorf("Start should not error, got: %v", err)
	}
}
