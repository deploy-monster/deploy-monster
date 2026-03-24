package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Dependencies ────────────────────────────────────────────────────────────

func TestDependencies_Graph_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:     "app1",
		Name:   "Web App",
		Status: "running",
	})

	handler := NewDependencyHandler(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/dependencies", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Graph(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}

	nodes, ok := resp["nodes"].([]any)
	if !ok {
		t.Fatal("expected nodes array in response")
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node (the app itself), got %d", len(nodes))
	}

	node := nodes[0].(map[string]any)
	if node["id"] != "app1" {
		t.Errorf("expected node id=app1, got %v", node["id"])
	}
	if node["name"] != "Web App" {
		t.Errorf("expected node name='Web App', got %v", node["name"])
	}
	if node["type"] != "app" {
		t.Errorf("expected node type=app, got %v", node["type"])
	}
	if node["status"] != "running" {
		t.Errorf("expected node status=running, got %v", node["status"])
	}
}

func TestDependencies_Graph_WithRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:     "app1",
		Name:   "Web App",
		Status: "running",
	})

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "abcdef123456abcdef",
				Image: "postgres:15",
				State: "running",
				Labels: map[string]string{
					"monster.stack":         "Web App",
					"monster.stack.service": "db",
				},
			},
			{
				ID:    "fedcba654321fedcba",
				Image: "redis:7",
				State: "running",
				Labels: map[string]string{
					"monster.stack":         "Web App",
					"monster.stack.service": "cache",
				},
			},
		},
	}

	handler := NewDependencyHandler(store, runtime)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/dependencies", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Graph(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	nodes := resp["nodes"].([]any)
	// 1 app + 2 linked services = 3 nodes.
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}

	// Verify that the linked nodes have the correct type guesses.
	dbNode := nodes[1].(map[string]any)
	if dbNode["type"] != "database" {
		t.Errorf("expected postgres node type=database, got %v", dbNode["type"])
	}
	cacheNode := nodes[2].(map[string]any)
	if cacheNode["type"] != "cache" {
		t.Errorf("expected redis node type=cache, got %v", cacheNode["type"])
	}
}

func TestDependencies_Graph_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewDependencyHandler(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent/dependencies", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Graph(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

func TestDependencies_GuessNodeType(t *testing.T) {
	tests := []struct {
		image    string
		expected string
	}{
		{"postgres:15", "database"},
		{"mysql:8", "database"},
		{"mariadb:10", "database"},
		{"redis:7", "cache"},
		{"memcached:latest", "cache"},
		{"mongo:6", "database"},
		{"nginx:latest", "service"},
		{"myapp:v1", "service"},
	}

	for _, tt := range tests {
		got := guessNodeType(tt.image)
		if got != tt.expected {
			t.Errorf("guessNodeType(%q) = %q, want %q", tt.image, got, tt.expected)
		}
	}
}
