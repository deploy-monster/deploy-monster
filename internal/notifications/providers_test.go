package notifications

import (
	"context"
	"testing"
)

func TestDispatcher_RegisterAndGet(t *testing.T) {
	d := NewDispatcher()

	mock := &mockProvider{name: "test"}
	d.RegisterProvider(mock)

	got, ok := d.GetProvider("test")
	if !ok {
		t.Fatal("expected provider to be found")
	}
	if got.Name() != "test" {
		t.Errorf("expected name 'test', got %q", got.Name())
	}
}

func TestDispatcher_NotFound(t *testing.T) {
	d := NewDispatcher()
	_, ok := d.GetProvider("nonexistent")
	if ok {
		t.Error("expected false for missing provider")
	}
}

func TestDispatcher_Providers(t *testing.T) {
	d := NewDispatcher()
	d.RegisterProvider(&mockProvider{name: "slack"})
	d.RegisterProvider(&mockProvider{name: "discord"})

	names := d.Providers()
	if len(names) != 2 {
		t.Errorf("expected 2 providers, got %d", len(names))
	}
}

type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Send(_ context.Context, _, _, _, _ string) error { return nil }
func (m *mockProvider) Validate() error { return nil }
