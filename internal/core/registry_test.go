package core

import (
	"context"
	"testing"
)

// stubModule is a minimal Module implementation for testing.
type stubModule struct {
	id          string
	deps        []string
	initCalled  bool
	startCalled bool
	stopCalled  bool
	health      HealthStatus
}

func newStub(id string, deps ...string) *stubModule {
	return &stubModule{id: id, deps: deps, health: HealthOK}
}

func (s *stubModule) ID() string             { return s.id }
func (s *stubModule) Name() string           { return s.id }
func (s *stubModule) Version() string        { return "1.0.0" }
func (s *stubModule) Dependencies() []string { return s.deps }
func (s *stubModule) Health() HealthStatus   { return s.health }
func (s *stubModule) Routes() []Route        { return nil }
func (s *stubModule) Events() []EventHandler { return nil }

func (s *stubModule) Init(_ context.Context, _ *Core) error {
	s.initCalled = true
	return nil
}
func (s *stubModule) Start(_ context.Context) error {
	s.startCalled = true
	return nil
}
func (s *stubModule) Stop(_ context.Context) error {
	s.stopCalled = true
	return nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	m := newStub("test.module")

	if err := r.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got := r.Get("test.module")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.ID() != "test.module" {
		t.Fatalf("expected ID 'test.module', got %q", got.ID())
	}
}

func TestRegistry_DuplicateRegister(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub("dup"))

	err := r.Register(newStub("dup"))
	if err == nil {
		t.Fatal("expected error on duplicate register")
	}
}

func TestRegistry_Resolve_DependencyOrder(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub("c", "b"))
	r.Register(newStub("b", "a"))
	r.Register(newStub("a"))

	if err := r.Resolve(); err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	order := r.All()
	indexOf := func(id string) int {
		for i, v := range order {
			if v == id {
				return i
			}
		}
		return -1
	}

	if indexOf("a") > indexOf("b") {
		t.Error("a should come before b")
	}
	if indexOf("b") > indexOf("c") {
		t.Error("b should come before c")
	}
}

func TestRegistry_Resolve_CircularDependency(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub("x", "y"))
	r.Register(newStub("y", "x"))

	err := r.Resolve()
	if err == nil {
		t.Fatal("expected circular dependency error")
	}
}

func TestRegistry_Resolve_UnknownDependency(t *testing.T) {
	r := NewRegistry()
	r.Register(newStub("orphan", "nonexistent"))

	err := r.Resolve()
	if err == nil {
		t.Fatal("expected unknown dependency error")
	}
}

func TestRegistry_Lifecycle(t *testing.T) {
	r := NewRegistry()
	a := newStub("a")
	b := newStub("b", "a")
	r.Register(a)
	r.Register(b)
	r.Resolve()

	ctx := context.Background()
	core := &Core{}

	if err := r.InitAll(ctx, core); err != nil {
		t.Fatalf("InitAll: %v", err)
	}
	if !a.initCalled || !b.initCalled {
		t.Error("Init not called on all modules")
	}

	if err := r.StartAll(ctx); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	if !a.startCalled || !b.startCalled {
		t.Error("Start not called on all modules")
	}

	if err := r.StopAll(ctx); err != nil {
		t.Fatalf("StopAll: %v", err)
	}
	if !a.stopCalled || !b.stopCalled {
		t.Error("Stop not called on all modules")
	}
}

func TestRegistry_HealthAll(t *testing.T) {
	r := NewRegistry()
	ok := newStub("ok")
	down := newStub("down")
	down.health = HealthDown

	r.Register(ok)
	r.Register(down)

	health := r.HealthAll()
	if health["ok"] != HealthOK {
		t.Error("expected ok module to be healthy")
	}
	if health["down"] != HealthDown {
		t.Error("expected down module to be down")
	}
}
