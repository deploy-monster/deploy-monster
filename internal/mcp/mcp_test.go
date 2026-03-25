package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =========================================================================
// Mocks
// =========================================================================

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockStore implements core.Store with only the methods the MCP handler uses.
type mockStore struct {
	core.Store // embed to satisfy the rest with nil panics if called

	apps      []core.Application
	appsTotal int
	appsErr   error
	app       *core.Application
	appErr    error
}

func (m *mockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return m.apps, m.appsTotal, m.appsErr
}

func (m *mockStore) GetApp(_ context.Context, id string) (*core.Application, error) {
	if m.appErr != nil {
		return nil, m.appErr
	}
	if m.app != nil && m.app.ID == id {
		return m.app, nil
	}
	return nil, fmt.Errorf("app %q not found", id)
}

// mockRuntime implements core.ContainerRuntime with only what's needed.
type mockRuntime struct {
	containers []core.ContainerInfo
	listErr    error
	logsData   string
	logsErr    error
	execOutput string
	execErr    error
}

func (m *mockRuntime) Ping() error { return nil }
func (m *mockRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (m *mockRuntime) Stop(_ context.Context, _ string, _ int) error   { return nil }
func (m *mockRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	return io.NopCloser(strings.NewReader(m.logsData)), nil
}
func (m *mockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return m.containers, m.listErr
}
func (m *mockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return m.execOutput, m.execErr
}
func (m *mockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{CPUPercent: 5.0}, nil
}
func (m *mockRuntime) ImagePull(_ context.Context, _ string) error                   { return nil }
func (m *mockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error)         { return nil, nil }
func (m *mockRuntime) ImageRemove(_ context.Context, _ string) error                 { return nil }
func (m *mockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error)      { return nil, nil }
func (m *mockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error)        { return nil, nil }

// =========================================================================
// Module tests
// =========================================================================

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModuleID(t *testing.T) {
	m := New()
	if got := m.ID(); got != "mcp" {
		t.Errorf("ID() = %q, want %q", got, "mcp")
	}
}

func TestModuleName(t *testing.T) {
	m := New()
	if got := m.Name(); got != "MCP Server" {
		t.Errorf("Name() = %q, want %q", got, "MCP Server")
	}
}

func TestModuleVersion(t *testing.T) {
	m := New()
	if got := m.Version(); got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}
}

func TestModuleDependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("Dependencies() = %v, want 2 elements", deps)
	}
	if deps[0] != "core.db" || deps[1] != "deploy" {
		t.Errorf("Dependencies() = %v, want [core.db deploy]", deps)
	}
}

func TestModuleRoutes(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModuleEvents(t *testing.T) {
	m := New()
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModuleHealth(t *testing.T) {
	m := New()
	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() = %v, want %v", got, core.HealthOK)
	}
}

func TestModuleInit(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:    &mockStore{},
		Services: core.NewServices(),
		Logger:   discardLogger(),
		Events:   core.NewEventBus(discardLogger()),
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if m.core != c {
		t.Error("Init() did not set core reference")
	}
	if m.store == nil {
		t.Error("Init() did not set store reference")
	}
	if m.logger == nil {
		t.Error("Init() did not set logger")
	}
}

func TestModuleStart(t *testing.T) {
	m := New()
	m.logger = discardLogger()
	if err := m.Start(context.Background()); err != nil {
		t.Errorf("Start() error = %v", err)
	}
}

func TestModuleStop(t *testing.T) {
	m := New()
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestModuleImplementsCoreModule(t *testing.T) {
	var _ core.Module = (*Module)(nil)
}

// =========================================================================
// Handler tests
// =========================================================================

func TestNewHandler(t *testing.T) {
	store := &mockStore{}
	runtime := &mockRuntime{}
	events := core.NewEventBus(discardLogger())
	logger := discardLogger()

	h := NewHandler(store, runtime, events, logger)
	if h == nil {
		t.Fatal("NewHandler() returned nil")
	}
	if h.store != store {
		t.Error("store not set")
	}
	if h.runtime != runtime {
		t.Error("runtime not set")
	}
	if h.events != events {
		t.Error("events not set")
	}
	if h.logger != logger {
		t.Error("logger not set")
	}
}

func TestHandleToolCall_ListApps_Success(t *testing.T) {
	store := &mockStore{
		apps: []core.Application{
			{ID: "app-1", Name: "web", Status: "running"},
			{ID: "app-2", Name: "api", Status: "stopped"},
		},
		appsTotal: 2,
	}
	h := NewHandler(store, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	resp, err := h.HandleToolCall(context.Background(), "list_apps", nil)
	if err != nil {
		t.Fatalf("HandleToolCall(list_apps) error = %v", err)
	}
	if resp.IsError {
		t.Errorf("response is error: %v", resp.Content)
	}
	if len(resp.Content) == 0 {
		t.Fatal("empty content")
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("content type = %q", resp.Content[0].Type)
	}
	// Verify the JSON contains both apps
	if !strings.Contains(resp.Content[0].Text, "app-1") {
		t.Error("response missing app-1")
	}
	if !strings.Contains(resp.Content[0].Text, "app-2") {
		t.Error("response missing app-2")
	}
}

func TestHandleToolCall_ListApps_Error(t *testing.T) {
	store := &mockStore{
		appsErr: fmt.Errorf("database connection failed"),
	}
	h := NewHandler(store, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	resp, err := h.HandleToolCall(context.Background(), "list_apps", nil)
	if err != nil {
		t.Fatalf("HandleToolCall(list_apps) error = %v", err)
	}
	if !resp.IsError {
		t.Error("expected error response")
	}
	if !strings.Contains(resp.Content[0].Text, "database connection failed") {
		t.Errorf("error text = %q", resp.Content[0].Text)
	}
}

func TestHandleToolCall_GetAppStatus_Success(t *testing.T) {
	store := &mockStore{
		app: &core.Application{
			ID:     "app-1",
			Name:   "web",
			Status: "running",
			Type:   "git",
		},
	}
	h := NewHandler(store, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{"app_id": "app-1"})
	resp, err := h.HandleToolCall(context.Background(), "get_app_status", input)
	if err != nil {
		t.Fatalf("HandleToolCall(get_app_status) error = %v", err)
	}
	if resp.IsError {
		t.Errorf("response is error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "app-1") {
		t.Error("response missing app ID")
	}
	if !strings.Contains(resp.Content[0].Text, "running") {
		t.Error("response missing status")
	}
}

func TestHandleToolCall_GetAppStatus_NotFound(t *testing.T) {
	store := &mockStore{
		appErr: fmt.Errorf("not found"),
	}
	h := NewHandler(store, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{"app_id": "nonexistent"})
	resp, err := h.HandleToolCall(context.Background(), "get_app_status", input)
	if err != nil {
		t.Fatalf("HandleToolCall(get_app_status) error = %v", err)
	}
	if !resp.IsError {
		t.Error("expected error response for missing app")
	}
	if !strings.Contains(resp.Content[0].Text, "App not found") {
		t.Errorf("error text = %q", resp.Content[0].Text)
	}
}

func TestHandleToolCall_DeployApp(t *testing.T) {
	h := NewHandler(&mockStore{}, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{
		"name":        "my-app",
		"source_type": "git",
		"source_url":  "https://github.com/user/repo.git",
	})
	resp, err := h.HandleToolCall(context.Background(), "deploy_app", input)
	if err != nil {
		t.Fatalf("HandleToolCall(deploy_app) error = %v", err)
	}
	if resp.IsError {
		t.Errorf("response is error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "my-app") {
		t.Error("response missing app name")
	}
	if !strings.Contains(resp.Content[0].Text, "pipeline initiated") {
		t.Error("response missing pipeline initiated message")
	}
	if !strings.Contains(resp.Content[0].Text, "git") {
		t.Error("response missing source type")
	}
}

func TestHandleToolCall_ViewLogs_Success(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456", Name: "app-container", Status: "running"},
		},
		logsData: "2024-01-01 12:00:00 Server started\n2024-01-01 12:01:00 Request received",
	}
	h := NewHandler(&mockStore{}, runtime, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]any{"app_id": "app-1", "lines": 50})
	resp, err := h.HandleToolCall(context.Background(), "view_logs", input)
	if err != nil {
		t.Fatalf("HandleToolCall(view_logs) error = %v", err)
	}
	if resp.IsError {
		t.Errorf("response is error: %v", resp.Content)
	}
	if !strings.Contains(resp.Content[0].Text, "Server started") {
		t.Error("response missing log content")
	}
}

func TestHandleToolCall_ViewLogs_DefaultLines(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456", Name: "app-container"},
		},
		logsData: "log line",
	}
	h := NewHandler(&mockStore{}, runtime, core.NewEventBus(discardLogger()), discardLogger())

	// lines <= 0 should default to 50
	input, _ := json.Marshal(map[string]any{"app_id": "app-1", "lines": 0})
	resp, err := h.HandleToolCall(context.Background(), "view_logs", input)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if resp.IsError {
		t.Error("unexpected error response")
	}
}

func TestHandleToolCall_ViewLogs_NoRuntime(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{"app_id": "app-1"})
	resp, err := h.HandleToolCall(context.Background(), "view_logs", input)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !resp.IsError {
		t.Error("expected error when runtime is nil")
	}
	if !strings.Contains(resp.Content[0].Text, "runtime not available") {
		t.Errorf("error text = %q", resp.Content[0].Text)
	}
}

func TestHandleToolCall_ViewLogs_NoContainers(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{}, // empty
	}
	h := NewHandler(&mockStore{}, runtime, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{"app_id": "app-1"})
	resp, err := h.HandleToolCall(context.Background(), "view_logs", input)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !resp.IsError {
		t.Error("expected error when no containers")
	}
	if !strings.Contains(resp.Content[0].Text, "No running container") {
		t.Errorf("error text = %q", resp.Content[0].Text)
	}
}

func TestHandleToolCall_ViewLogs_ListByLabelsError(t *testing.T) {
	runtime := &mockRuntime{
		listErr: fmt.Errorf("docker daemon not reachable"),
	}
	h := NewHandler(&mockStore{}, runtime, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{"app_id": "app-1"})
	resp, err := h.HandleToolCall(context.Background(), "view_logs", input)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !resp.IsError {
		t.Error("expected error when ListByLabels fails")
	}
}

func TestHandleToolCall_ViewLogs_LogsError(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456"},
		},
		logsErr: fmt.Errorf("container stopped"),
	}
	h := NewHandler(&mockStore{}, runtime, core.NewEventBus(discardLogger()), discardLogger())

	input, _ := json.Marshal(map[string]string{"app_id": "app-1"})
	resp, err := h.HandleToolCall(context.Background(), "view_logs", input)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !resp.IsError {
		t.Error("expected error when Logs() fails")
	}
	if !strings.Contains(resp.Content[0].Text, "container stopped") {
		t.Errorf("error text = %q", resp.Content[0].Text)
	}
}

// =========================================================================
// "Coming soon" tool tests
// =========================================================================

func TestHandleToolCall_ComingSoonTools(t *testing.T) {
	h := NewHandler(&mockStore{}, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	tests := []struct {
		tool    string
		keyword string
	}{
		{"create_database", "coming soon"},
		{"add_domain", "coming soon"},
		{"marketplace_deploy", "coming soon"},
		{"provision_server", "coming soon"},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			resp, err := h.HandleToolCall(context.Background(), tt.tool, nil)
			if err != nil {
				t.Fatalf("HandleToolCall(%s) error = %v", tt.tool, err)
			}
			if resp.IsError {
				t.Error("coming soon tools should not be error responses")
			}
			if !strings.Contains(strings.ToLower(resp.Content[0].Text), tt.keyword) {
				t.Errorf("response = %q, want %q", resp.Content[0].Text, tt.keyword)
			}
		})
	}
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	h := NewHandler(&mockStore{}, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())

	_, err := h.HandleToolCall(context.Background(), "nonexistent_tool", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error = %q, want 'unknown tool' mention", err.Error())
	}
}

// =========================================================================
// ListTools tests
// =========================================================================

func TestHandlerListTools(t *testing.T) {
	h := NewHandler(&mockStore{}, &mockRuntime{}, core.NewEventBus(discardLogger()), discardLogger())
	tools := h.ListTools()
	if len(tools) == 0 {
		t.Fatal("ListTools() returned empty")
	}
	// Should match BuiltinTools
	builtin := BuiltinTools()
	if len(tools) != len(builtin) {
		t.Errorf("ListTools() returned %d tools, BuiltinTools() returns %d", len(tools), len(builtin))
	}
}

// =========================================================================
// Tool definitions tests
// =========================================================================

func TestBuiltinTools_Count(t *testing.T) {
	tools := BuiltinTools()
	if len(tools) < 8 {
		t.Errorf("BuiltinTools() returned %d tools, want at least 8", len(tools))
	}
}

func TestBuiltinTools_AllHaveNames(t *testing.T) {
	for _, tool := range BuiltinTools() {
		if tool.Name == "" {
			t.Error("found tool with empty name")
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
	}
}

func TestBuiltinTools_AllHaveSchemas(t *testing.T) {
	for _, tool := range BuiltinTools() {
		if tool.InputSchema.Type != "object" {
			t.Errorf("tool %q schema type = %q, want %q", tool.Name, tool.InputSchema.Type, "object")
		}
		if tool.InputSchema.Properties == nil {
			t.Errorf("tool %q has nil properties", tool.Name)
		}
	}
}

func TestBuiltinTools_SpecificTools(t *testing.T) {
	tools := BuiltinTools()
	toolMap := make(map[string]Tool)
	for _, tool := range tools {
		toolMap[tool.Name] = tool
	}

	expected := []string{
		"deploy_app", "list_apps", "get_app_status", "scale_app",
		"view_logs", "create_database", "add_domain", "marketplace_deploy",
		"provision_server",
	}
	for _, name := range expected {
		if _, ok := toolMap[name]; !ok {
			t.Errorf("missing tool %q in BuiltinTools()", name)
		}
	}
}

func TestBuiltinTools_DeployAppRequired(t *testing.T) {
	tools := BuiltinTools()
	var deployTool Tool
	for _, tool := range tools {
		if tool.Name == "deploy_app" {
			deployTool = tool
			break
		}
	}
	if len(deployTool.InputSchema.Required) != 3 {
		t.Errorf("deploy_app required = %v, want 3 required fields", deployTool.InputSchema.Required)
	}
}

func TestBuiltinTools_DeployAppEnums(t *testing.T) {
	tools := BuiltinTools()
	for _, tool := range tools {
		if tool.Name == "deploy_app" {
			prop, ok := tool.InputSchema.Properties["source_type"]
			if !ok {
				t.Fatal("deploy_app missing source_type property")
			}
			if len(prop.Enum) != 3 {
				t.Errorf("source_type enum = %v, want 3 values", prop.Enum)
			}
		}
	}
}

func TestBuiltinTools_JSONSerializable(t *testing.T) {
	tools := BuiltinTools()
	data, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("failed to marshal BuiltinTools: %v", err)
	}
	if len(data) == 0 {
		t.Error("marshaled tools is empty")
	}
	// Round-trip
	var decoded []Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(decoded) != len(tools) {
		t.Errorf("round-trip lost tools: %d -> %d", len(tools), len(decoded))
	}
}

// =========================================================================
// textResponse / errorResponse helper tests
// =========================================================================

func TestTextResponse(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, nil, discardLogger())
	resp, err := h.textResponse("hello")
	if err != nil {
		t.Fatalf("textResponse error = %v", err)
	}
	if resp.IsError {
		t.Error("textResponse should not set IsError")
	}
	if len(resp.Content) != 1 {
		t.Fatalf("content length = %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" {
		t.Errorf("type = %q", resp.Content[0].Type)
	}
	if resp.Content[0].Text != "hello" {
		t.Errorf("text = %q", resp.Content[0].Text)
	}
}

func TestErrorResponse(t *testing.T) {
	h := NewHandler(&mockStore{}, nil, nil, discardLogger())
	resp, err := h.errorResponse("something broke")
	if err != nil {
		t.Fatalf("errorResponse error = %v", err)
	}
	if !resp.IsError {
		t.Error("errorResponse should set IsError = true")
	}
	if !strings.HasPrefix(resp.Content[0].Text, "Error: ") {
		t.Errorf("text = %q, want 'Error: ' prefix", resp.Content[0].Text)
	}
}

// =========================================================================
// Full lifecycle test
// =========================================================================

func TestModuleFullLifecycle(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:    &mockStore{},
		Services: core.NewServices(),
		Logger:   discardLogger(),
		Events:   core.NewEventBus(discardLogger()),
	}
	ctx := context.Background()

	if err := m.Init(ctx, c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v", h)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// =========================================================================
// Input schema type assertions
// =========================================================================

func TestInputSchemaStruct(t *testing.T) {
	s := InputSchema{
		Type: "object",
		Properties: map[string]Property{
			"name": {Type: "string", Description: "Name"},
		},
		Required: []string{"name"},
	}
	if s.Type != "object" {
		t.Error("Type mismatch")
	}
	if len(s.Properties) != 1 {
		t.Error("Properties count mismatch")
	}
	if len(s.Required) != 1 {
		t.Error("Required count mismatch")
	}
}

func TestPropertyWithEnum(t *testing.T) {
	p := Property{
		Type:        "string",
		Description: "Engine",
		Enum:        []string{"postgres", "mysql"},
	}
	if len(p.Enum) != 2 {
		t.Errorf("Enum length = %d", len(p.Enum))
	}
}

func TestContentBlock(t *testing.T) {
	b := ContentBlock{Type: "text", Text: "hello"}
	data, _ := json.Marshal(b)
	if !strings.Contains(string(data), `"type":"text"`) {
		t.Errorf("marshaled = %s", data)
	}
}

func TestMCPRequestJSON(t *testing.T) {
	raw := `{"method":"tools/call","params":{"name":"test"}}`
	var req MCPRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if req.Method != "tools/call" {
		t.Errorf("Method = %q", req.Method)
	}
	if req.Params == nil {
		t.Error("Params is nil")
	}
}

func TestMCPResponseJSON(t *testing.T) {
	resp := MCPResponse{
		Content: []ContentBlock{{Type: "text", Text: "ok"}},
		IsError: false,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), `"text":"ok"`) {
		t.Errorf("marshaled = %s", data)
	}
	// IsError false should be omitted
	if strings.Contains(string(data), `"isError"`) {
		t.Errorf("expected isError to be omitted when false, got %s", data)
	}
}

func TestMCPResponseJSON_WithError(t *testing.T) {
	resp := MCPResponse{
		Content: []ContentBlock{{Type: "text", Text: "fail"}},
		IsError: true,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(data), `"isError":true`) {
		t.Errorf("expected isError:true, got %s", data)
	}
}
