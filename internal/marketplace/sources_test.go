package marketplace

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ───────────────────────────── test helpers ─────────────────────────────

// fakeSource is a TemplateSource that returns a pre-canned slice (or error)
// and counts how many times Fetch was called. Covers every code path the
// marketplace's remote-source loop cares about without a real HTTP server.
type fakeSource struct {
	name      string
	templates []*Template
	err       error
	delay     time.Duration
	calls     atomic.Int32
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Fetch(ctx context.Context) ([]*Template, error) {
	f.calls.Add(1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	// Return a defensive copy so the caller cannot mutate our fixture.
	out := make([]*Template, len(f.templates))
	copy(out, f.templates)
	return out, nil
}

func validTpl(slug string) *Template {
	return &Template{
		Slug:        slug,
		Name:        "Test " + slug,
		Description: "desc",
		Category:    "testing",
		Author:      "tester",
		Version:     "1.0.0",
		// Minimal docker-compose with one service containing an image.
		ComposeYAML: "services:\n  web:\n    image: nginx:alpine\n",
	}
}

func newInitedModule(t *testing.T) *Module {
	t.Helper()
	m := New()
	if err := m.Init(context.Background(), &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{},
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return m
}

// ───────────────────────────── AddSource ─────────────────────────────

func TestAddSource_IgnoresNil(t *testing.T) {
	m := New()
	m.AddSource(nil)
	if len(m.sources) != 0 {
		t.Fatalf("AddSource(nil) should not grow sources, got %d", len(m.sources))
	}
}

func TestAddSource_AppendsInOrder(t *testing.T) {
	m := New()
	a := &fakeSource{name: "a"}
	b := &fakeSource{name: "b"}
	c := &fakeSource{name: "c"}
	m.AddSource(a)
	m.AddSource(b)
	m.AddSource(c)

	if len(m.sources) != 3 {
		t.Fatalf("got %d sources, want 3", len(m.sources))
	}
	got := []string{m.sources[0].Name(), m.sources[1].Name(), m.sources[2].Name()}
	want := []string{"a", "b", "c"}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("sources[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAddSource_ConcurrentSafe(t *testing.T) {
	m := New()
	const workers = 16
	const perWorker = 50

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				m.AddSource(&fakeSource{name: "s"})
			}
		}(w)
	}
	wg.Wait()

	if got := len(m.sources); got != workers*perWorker {
		t.Errorf("race lost: got %d sources, want %d", got, workers*perWorker)
	}
}

// ───────────────────────────── SetUpdateInterval ─────────────────────────────

func TestSetUpdateInterval_StoresValue(t *testing.T) {
	m := New()
	m.SetUpdateInterval(42 * time.Second)
	if m.updateInterval != 42*time.Second {
		t.Errorf("updateInterval = %v, want 42s", m.updateInterval)
	}
}

func TestSetUpdateInterval_AcceptsZero(t *testing.T) {
	m := New()
	m.SetUpdateInterval(1 * time.Hour) // set something first
	m.SetUpdateInterval(0)             // then disable
	if m.updateInterval != 0 {
		t.Errorf("updateInterval = %v, want 0 (loop disabled)", m.updateInterval)
	}
}

// ───────────────────────────── UpdateTemplates ─────────────────────────────

func TestUpdateTemplates_NoSources_ReturnsEmpty(t *testing.T) {
	m := newInitedModule(t)
	res := m.UpdateTemplates(context.Background())
	if len(res) != 0 {
		t.Errorf("expected 0 results with no sources, got %d", len(res))
	}
}

func TestUpdateTemplates_HappyPath_AddsAndUpdates(t *testing.T) {
	m := newInitedModule(t)
	preCount := m.registry.Count()

	// One brand-new slug + one that already exists (wordpress is a builtin).
	src := &fakeSource{
		name: "upstream",
		templates: []*Template{
			validTpl("custom-new-app"),
			// Override an existing builtin slug to exercise the "updated" path.
			func() *Template {
				t := validTpl("wordpress")
				t.Description = "upstream override"
				return t
			}(),
		},
	}
	m.AddSource(src)

	results := m.UpdateTemplates(context.Background())
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	r := results[0]
	if r.Source != "upstream" {
		t.Errorf("source = %q, want upstream", r.Source)
	}
	if r.Added != 1 {
		t.Errorf("Added = %d, want 1", r.Added)
	}
	if r.Updated != 1 {
		t.Errorf("Updated = %d, want 1", r.Updated)
	}
	if m.registry.Count() != preCount+1 {
		t.Errorf("registry count = %d, want %d (one new add)", m.registry.Count(), preCount+1)
	}
	if wp := m.registry.Get("wordpress"); wp == nil || wp.Description != "upstream override" {
		t.Errorf("wordpress not updated with override description")
	}
}

func TestUpdateTemplates_SourceError_ContinuesOtherSources(t *testing.T) {
	m := newInitedModule(t)

	bad := &fakeSource{name: "bad", err: errors.New("503 from upstream")}
	good := &fakeSource{name: "good", templates: []*Template{validTpl("good-app")}}
	m.AddSource(bad)
	m.AddSource(good)

	results := m.UpdateTemplates(context.Background())
	// Only the healthy source produces a result. The failing one is logged
	// and skipped, the loop does not abort.
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1 (bad source skipped)", len(results))
	}
	if results[0].Source != "good" {
		t.Errorf("surviving source = %q, want good", results[0].Source)
	}
	// Both sources were still called.
	if bad.calls.Load() != 1 || good.calls.Load() != 1 {
		t.Errorf("fetch counts: bad=%d good=%d, want 1/1", bad.calls.Load(), good.calls.Load())
	}
}

func TestUpdateTemplates_EmptyResponse_TreatedAsError(t *testing.T) {
	m := newInitedModule(t)
	pre := m.registry.Count()

	m.AddSource(&fakeSource{name: "empty", templates: nil})
	results := m.UpdateTemplates(context.Background())

	if len(results) != 0 {
		t.Errorf("empty upstream should produce no successful result, got %d", len(results))
	}
	// Critical: registry must NOT be emptied by a zero-template response.
	if m.registry.Count() != pre {
		t.Errorf("registry emptied by zero-template response: before=%d after=%d", pre, m.registry.Count())
	}
}

// ───────────────────────── TemplateRegistry.Update ─────────────────────────

func TestRegistryUpdate_NilSource(t *testing.T) {
	r := NewTemplateRegistry()
	if _, err := r.Update(context.Background(), nil); err == nil {
		t.Error("expected error for nil source")
	}
}

func TestRegistryUpdate_RejectsInvalid_ContinuesValid(t *testing.T) {
	r := NewTemplateRegistry()
	src := &fakeSource{
		name: "mixed",
		templates: []*Template{
			validTpl("valid-one"),
			{Slug: "broken"}, // missing required fields → rejected
			nil,              // also rejected
			validTpl("valid-two"),
		},
	}

	res, err := r.Update(context.Background(), src)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Added != 2 {
		t.Errorf("Added = %d, want 2", res.Added)
	}
	if res.Rejected != 2 {
		t.Errorf("Rejected = %d, want 2", res.Rejected)
	}
	if res.Total != 4 {
		t.Errorf("Total = %d, want 4", res.Total)
	}
	if len(res.Errors) != 2 {
		t.Errorf("expected 2 validation error messages, got %d", len(res.Errors))
	}
}

func TestRegistryUpdate_AllInvalid_ReturnsError(t *testing.T) {
	r := NewTemplateRegistry()
	src := &fakeSource{
		name:      "allbad",
		templates: []*Template{{Slug: "x"}, {Slug: "y"}},
	}

	res, err := r.Update(context.Background(), src)
	if err == nil {
		t.Error("expected error when every template fails validation")
	}
	if res == nil || res.Rejected != 2 {
		t.Errorf("Rejected = %v, want 2", res)
	}
}

func TestRegistryUpdate_FetchError_Propagates(t *testing.T) {
	r := NewTemplateRegistry()
	sentinel := errors.New("network down")
	src := &fakeSource{name: "offline", err: sentinel}

	_, err := r.Update(context.Background(), src)
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want wrapped sentinel", err)
	}
}

// ───────────────────────────── updateLoop ─────────────────────────────

// TestStart_BackgroundLoopRunsAndStops exercises the full goroutine path:
// Start spawns updateLoop, a tick fires UpdateTemplates, Stop closes stopCh
// and wg.Wait returns. A failure here means either the loop didn't start,
// didn't run, or didn't shut down cleanly.
func TestStart_BackgroundLoopRunsAndStops(t *testing.T) {
	m := newInitedModule(t)
	src := &fakeSource{name: "ticker", templates: []*Template{validTpl("ticker-app")}}
	m.AddSource(src)
	m.SetUpdateInterval(10 * time.Millisecond) // fast enough to see a tick

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for at least one tick to land.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && src.calls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if src.calls.Load() == 0 {
		t.Error("updateLoop never called the source within 2s")
	}

	// Stop should return promptly (goroutine exits on stopCh close).
	stopped := make(chan struct{})
	go func() {
		_ = m.Stop(context.Background())
		close(stopped)
	}()
	select {
	case <-stopped:
		// Confirm the template was registered by the tick.
		if m.registry.Get("ticker-app") == nil {
			t.Error("tick ran but did not register ticker-app")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop hung — updateLoop did not exit on stopCh close")
	}
}

// TestStart_NoSources_SkipsLoop verifies Start does NOT spawn the goroutine
// when sources are empty or interval is zero — the common single-binary
// deployment never pays for a background ticker it doesn't need.
func TestStart_NoSources_SkipsLoop(t *testing.T) {
	m := newInitedModule(t)
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.stopCh != nil {
		t.Error("stopCh should be nil when no sources are configured — loop was spawned anyway")
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestStart_SourcesButZeroInterval_SkipsLoop(t *testing.T) {
	m := newInitedModule(t)
	m.AddSource(&fakeSource{name: "src"})
	// interval left at 0 → loop disabled
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if m.stopCh != nil {
		t.Error("stopCh should be nil when interval is 0 — loop should be disabled")
	}
}

// TestStop_IsIdempotent — Stop must be safe to call twice (sync.Once guard).
func TestStop_IsIdempotent(t *testing.T) {
	m := newInitedModule(t)
	m.AddSource(&fakeSource{name: "s", templates: []*Template{validTpl("stop-idem")}})
	m.SetUpdateInterval(20 * time.Millisecond)

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second Stop must not panic (close of closed channel) and must return nil.
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
