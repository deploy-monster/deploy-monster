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

func TestMutatingTools_NilEventBus(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		store core.Store
		input map[string]any
	}{
		{
			name:  "deploy app",
			tool:  "deploy_app",
			store: &mockStore{},
			input: map[string]any{
				"name":        "web",
				"source_type": "git",
				"source_url":  "https://example.com/repo.git",
			},
		},
		{
			name:  "scale app",
			tool:  "scale_app",
			store: &mockStoreUpdateApp{app: &core.Application{ID: "app-1", Name: "web", TenantID: "tenant-1", Replicas: 1}},
			input: map[string]any{"app_id": "app-1", "replicas": 2},
		},
		{
			name:  "create database",
			tool:  "create_database",
			store: &mockStore{},
			input: map[string]any{"engine": "postgres", "name": "appdb"},
		},
		{
			name:  "marketplace deploy",
			tool:  "marketplace_deploy",
			store: &mockStore{},
			input: map[string]any{"template_slug": "postgres", "name": "db"},
		},
		{
			name:  "provision server",
			tool:  "provision_server",
			store: &mockStore{},
			input: map[string]any{"provider": "custom", "name": "node-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(tt.store, nil, nil, discardLogger())
			input, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := h.HandleToolCall(context.Background(), tt.tool, input)
			if err != nil {
				t.Fatalf("HandleToolCall: %v", err)
			}
			if resp.IsError {
				t.Fatalf("unexpected error response: %v", resp.Content)
			}
		})
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
