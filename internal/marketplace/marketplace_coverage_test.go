package marketplace

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init with Core
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Init_WithCore(t *testing.T) {
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{},
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.registry == nil {
		t.Error("registry should be initialized after Init")
	}
	if m.logger == nil {
		t.Error("logger should be set after Init")
	}

	// Registry should have builtins loaded
	if m.registry.Count() < 10 {
		t.Errorf("expected at least 10 builtin templates after Init, got %d", m.registry.Count())
	}
}

func TestModule_Start_WithCore(t *testing.T) {
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{},
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestModule_Registry_ReturnsTemplateRegistry(t *testing.T) {
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{},
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	reg := m.Registry()
	if reg == nil {
		t.Fatal("Registry() should not return nil after Init")
	}
	if reg.Count() < 10 {
		t.Errorf("Registry should have builtins loaded, got %d", reg.Count())
	}
}

func TestModule_Registry_BeforeInit(t *testing.T) {
	m := New()
	reg := m.Registry()
	if reg != nil {
		t.Error("Registry() should return nil before Init")
	}
}

func TestModule_FullLifecycle(t *testing.T) {
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{},
	}

	m := New()

	// Init
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Start
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Verify registry works
	reg := m.Registry()
	wp := reg.Get("wordpress")
	if wp == nil {
		t.Error("wordpress template should be available after Init")
	}

	// Stop
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Health
	if m.Health() != core.HealthOK {
		t.Errorf("Health should be OK, got %v", m.Health())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TemplateRegistry — more edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestTemplateRegistry_Search_ByDescription(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(&Template{
		Slug:        "test-app",
		Name:        "TestApp",
		Description: "A unique description about widgets",
		Category:    "testing",
	})

	results := r.Search("widgets")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'widgets', got %d", len(results))
	}
}

func TestTemplateRegistry_Search_CaseInsensitiveTags(t *testing.T) {
	r := NewTemplateRegistry()
	r.Add(&Template{
		Slug:     "tagged",
		Name:     "Tagged",
		Category: "test",
		Tags:     []string{"Database", "SQL"},
	})

	results := r.Search("database")
	if len(results) != 1 {
		t.Errorf("expected 1 result for case-insensitive tag search, got %d", len(results))
	}

	results = r.Search("sql")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'sql' tag search, got %d", len(results))
	}
}

func TestTemplateRegistry_List_AllCategories(t *testing.T) {
	r := NewTemplateRegistry()
	r.LoadBuiltins()

	cats := r.Categories()
	for _, cat := range cats {
		catTemplates := r.List(cat)
		if len(catTemplates) == 0 {
			t.Errorf("category %q should have at least one template", cat)
		}
	}
}

func TestContainsTag_NoMatch(t *testing.T) {
	if containsTag([]string{"a", "b", "c"}, "z") {
		t.Error("should not match")
	}
}

func TestContainsTag_EmptyTags(t *testing.T) {
	if containsTag(nil, "test") {
		t.Error("nil tags should not match anything")
	}
}

func TestContainsTag_PartialMatch(t *testing.T) {
	if !containsTag([]string{"database"}, "data") {
		t.Error("partial match should work (Contains)")
	}
}

func TestTemplateRegistry_ConcurrentAccess(t *testing.T) {
	r := NewTemplateRegistry()

	// Concurrent writes
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			r.Add(&Template{
				Slug:     "concurrent-" + string(rune('a'+i%26)),
				Name:     "Concurrent",
				Category: "test",
			})
		}
		close(done)
	}()

	// Concurrent reads while writing
	for i := 0; i < 50; i++ {
		_ = r.Count()
		_ = r.List("")
		_ = r.Search("concurrent")
		_ = r.Categories()
	}

	<-done
}
