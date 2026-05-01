package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockStoreUpdateApp supports UpdateApp for scaleApp tests.
type mockStoreUpdateApp struct {
	core.Store
	app       *core.Application
	appErr    error
	updateErr error
}

func (m *mockStoreUpdateApp) GetApp(_ context.Context, id string) (*core.Application, error) {
	if m.appErr != nil {
		return nil, m.appErr
	}
	if m.app != nil && m.app.ID == id {
		return m.app, nil
	}
	return nil, fmt.Errorf("app %q not found", id)
}

func (m *mockStoreUpdateApp) UpdateApp(_ context.Context, app *core.Application) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.app = app
	return nil
}

func TestScaleApp_Success(t *testing.T) {
	store := &mockStoreUpdateApp{app: &core.Application{ID: "app-1", Name: "web", Replicas: 1}}
	eb := core.NewEventBus(discardLogger())
	h := NewHandler(store, nil, eb, discardLogger())

	input, _ := json.Marshal(map[string]any{
		"app_id":   "app-1",
		"replicas": 3,
	})
	resp, err := h.HandleToolCall(context.Background(), "scale_app", input)
	if err != nil {
		t.Fatalf("HandleToolCall: %v", err)
	}
	if resp.IsError {
		t.Fatalf("unexpected error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "scaled") {
		t.Errorf("expected 'scaled' in response, got %q", resp.Content[0].Text)
	}
	if store.app.Replicas != 3 {
		t.Errorf("replicas = %d, want 3", store.app.Replicas)
	}
}

func TestScaleApp_InvalidJSON(t *testing.T) {
	h := NewHandler(&mockStoreUpdateApp{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	resp, err := h.HandleToolCall(context.Background(), "scale_app", json.RawMessage(`{bad`))
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Error("expected error for invalid JSON")
	}
}

func TestScaleApp_MissingAppID(t *testing.T) {
	h := NewHandler(&mockStoreUpdateApp{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]any{"replicas": 2})
	resp, err := h.HandleToolCall(context.Background(), "scale_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "app_id") {
		t.Errorf("expected app_id required error, got %v", resp.Content)
	}
}

func TestScaleApp_NegativeReplicas(t *testing.T) {
	h := NewHandler(&mockStoreUpdateApp{}, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]any{"app_id": "a1", "replicas": -1})
	resp, err := h.HandleToolCall(context.Background(), "scale_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "replicas") {
		t.Errorf("expected replicas validation error, got %v", resp.Content)
	}
}

func TestScaleApp_AppNotFound(t *testing.T) {
	store := &mockStoreUpdateApp{appErr: fmt.Errorf("not found")}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]any{"app_id": "missing", "replicas": 2})
	resp, err := h.HandleToolCall(context.Background(), "scale_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "not found") {
		t.Errorf("expected not found error, got %v", resp.Content)
	}
}

func TestScaleApp_UpdateError(t *testing.T) {
	store := &mockStoreUpdateApp{
		app:       &core.Application{ID: "app-1", Name: "web", Replicas: 1},
		updateErr: fmt.Errorf("db locked"),
	}
	h := NewHandler(store, nil, core.NewEventBus(discardLogger()), discardLogger())
	input, _ := json.Marshal(map[string]any{"app_id": "app-1", "replicas": 2})
	resp, err := h.HandleToolCall(context.Background(), "scale_app", input)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsError || !strings.Contains(resp.Content[0].Text, "failed to update") {
		t.Errorf("expected update error, got %v", resp.Content)
	}
}
