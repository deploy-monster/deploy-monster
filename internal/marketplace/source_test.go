package marketplace

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// makeValidTemplateJSON returns a minimal template serializable to JSON
// that will pass validation when round-tripped through a source.
func makeValidTemplateJSON(slug string) *Template {
	return &Template{
		Slug:        slug,
		Name:        "T-" + slug,
		Description: "desc",
		Category:    "test",
		Author:      "tester",
		Version:     "1.0.0",
		MinResources: ResourceReq{
			MemoryMB: 128,
			DiskMB:   256,
			CPUMB:    100,
		},
		ComposeYAML: "services:\n  web:\n    image: nginx:latest\n",
	}
}

func TestHTTPTemplateSource_Fetch_BareArray(t *testing.T) {
	want := []*Template{
		makeValidTemplateJSON("a"),
		makeValidTemplateJSON("b"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(want)
	}))
	defer srv.Close()

	src := &HTTPTemplateSource{URL: srv.URL, Label: "test"}
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 2 || got[0].Slug != "a" || got[1].Slug != "b" {
		t.Errorf("unexpected fetch result: %+v", got)
	}
	if src.Name() != "test" {
		t.Errorf("expected Name=test, got %s", src.Name())
	}
}

func TestHTTPTemplateSource_Fetch_Envelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]any{
			"templates": []*Template{makeValidTemplateJSON("a")},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	src := &HTTPTemplateSource{URL: srv.URL}
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 1 || got[0].Slug != "a" {
		t.Errorf("unexpected fetch result: %+v", got)
	}
	// Name() falls back to URL when Label is empty.
	if src.Name() != srv.URL {
		t.Errorf("expected Name=URL, got %s", src.Name())
	}
}

func TestHTTPTemplateSource_Fetch_EmptyURL(t *testing.T) {
	src := &HTTPTemplateSource{}
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestHTTPTemplateSource_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := &HTTPTemplateSource{URL: srv.URL}
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status code, got %v", err)
	}
}

func TestHTTPTemplateSource_Fetch_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	src := &HTTPTemplateSource{URL: srv.URL}
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
}

func TestHTTPTemplateSource_Fetch_ContextCanceled(t *testing.T) {
	// A server that hangs forever; the context cancellation should
	// unblock Fetch without waiting for the default 30s timeout.
	blocker := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
	}))
	defer func() {
		close(blocker)
		srv.Close()
	}()

	src := &HTTPTemplateSource{URL: srv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := src.Fetch(ctx)
	if err == nil {
		t.Fatal("expected context-cancellation error")
	}
}

// fakeSource lets tests hand-craft the returned templates and error.
type fakeSource struct {
	name  string
	data  []*Template
	err   error
	calls int
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Fetch(ctx context.Context) ([]*Template, error) {
	f.calls++
	return f.data, f.err
}

func TestRegistry_Update_NilSource(t *testing.T) {
	r := NewTemplateRegistry()
	_, err := r.Update(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil source")
	}
}

func TestRegistry_Update_AddsAndUpdates(t *testing.T) {
	r := NewTemplateRegistry()
	// Pre-populate with one that will be updated.
	existing := makeValidTemplateJSON("a")
	existing.Name = "old"
	r.Add(existing)

	src := &fakeSource{
		name: "fake",
		data: []*Template{
			makeValidTemplateJSON("a"), // update
			makeValidTemplateJSON("b"), // add
		},
	}
	res, err := r.Update(context.Background(), src)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Source != "fake" {
		t.Errorf("expected Source=fake, got %s", res.Source)
	}
	if res.Added != 1 || res.Updated != 1 {
		t.Errorf("expected added=1 updated=1, got added=%d updated=%d", res.Added, res.Updated)
	}
	if res.Total != 2 || res.Rejected != 0 {
		t.Errorf("expected total=2 rejected=0, got total=%d rejected=%d", res.Total, res.Rejected)
	}
	if r.Get("a").Name == "old" {
		t.Error("expected template a to be updated")
	}
	if r.Get("b") == nil {
		t.Error("expected template b to be added")
	}
}

func TestRegistry_Update_RejectsInvalidButKeepsValid(t *testing.T) {
	r := NewTemplateRegistry()
	good := makeValidTemplateJSON("good")
	bad := makeValidTemplateJSON("bad")
	bad.Name = "" // fails validation

	src := &fakeSource{name: "mixed", data: []*Template{good, bad, nil}}
	res, err := r.Update(context.Background(), src)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if res.Added != 1 {
		t.Errorf("expected added=1, got %d", res.Added)
	}
	if res.Rejected != 2 {
		t.Errorf("expected rejected=2 (bad + nil), got %d", res.Rejected)
	}
	if r.Get("good") == nil {
		t.Error("good template should have been added")
	}
	if r.Get("bad") != nil {
		t.Error("bad template should have been rejected")
	}
}

func TestRegistry_Update_EmptyFeedIsError(t *testing.T) {
	r := NewTemplateRegistry()
	// Pre-populate; ensure empty feed does NOT wipe the registry.
	r.Add(makeValidTemplateJSON("kept"))

	src := &fakeSource{name: "empty", data: []*Template{}}
	_, err := r.Update(context.Background(), src)
	if err == nil {
		t.Fatal("expected error for empty feed")
	}
	if r.Get("kept") == nil {
		t.Error("empty feed must not wipe existing templates")
	}
}

func TestRegistry_Update_AllInvalidIsError(t *testing.T) {
	r := NewTemplateRegistry()
	bad := makeValidTemplateJSON("bad")
	bad.Name = ""
	src := &fakeSource{name: "allbad", data: []*Template{bad}}

	res, err := r.Update(context.Background(), src)
	if err == nil {
		t.Fatal("expected error when every template is invalid")
	}
	if res.Rejected != 1 {
		t.Errorf("expected rejected=1, got %d", res.Rejected)
	}
}

func TestRegistry_Update_SourceError(t *testing.T) {
	r := NewTemplateRegistry()
	src := &fakeSource{name: "broken", err: http.ErrHandlerTimeout}
	_, err := r.Update(context.Background(), src)
	if err == nil {
		t.Fatal("expected error from source")
	}
}

func TestModule_UpdateTemplates_MultiSource(t *testing.T) {
	m := New()
	m.registry = NewTemplateRegistry()
	// Use a discard logger so the test doesn't depend on core.Core.
	m.logger = newTestLogger()

	srcA := &fakeSource{name: "a", data: []*Template{makeValidTemplateJSON("from-a")}}
	srcB := &fakeSource{name: "b", data: []*Template{makeValidTemplateJSON("from-b")}}
	srcC := &fakeSource{name: "c", err: http.ErrHandlerTimeout}

	m.AddSource(srcA)
	m.AddSource(srcB)
	m.AddSource(srcC)
	m.AddSource(nil) // ignored

	results := m.UpdateTemplates(context.Background())
	if len(results) != 2 {
		t.Errorf("expected 2 successful results, got %d", len(results))
	}
	if m.registry.Get("from-a") == nil || m.registry.Get("from-b") == nil {
		t.Error("both successful sources should have added templates")
	}
	if srcA.calls != 1 || srcB.calls != 1 || srcC.calls != 1 {
		t.Errorf("expected each source fetched once, got %d/%d/%d",
			srcA.calls, srcB.calls, srcC.calls)
	}
}

func TestModule_StartStop_WithoutSources(t *testing.T) {
	m := New()
	m.registry = NewTemplateRegistry()
	m.logger = newTestLogger()

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	// Idempotent Stop.
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestModule_StartStop_WithSources(t *testing.T) {
	m := New()
	m.registry = NewTemplateRegistry()
	m.logger = newTestLogger()

	src := &fakeSource{name: "tick", data: []*Template{makeValidTemplateJSON("ticked")}}
	m.AddSource(src)
	m.SetUpdateInterval(5 * time.Millisecond)

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Wait long enough for at least one tick.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.registry.Get("ticked") != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m.registry.Get("ticked") == nil {
		t.Error("expected update loop to run at least once")
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
