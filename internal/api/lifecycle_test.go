package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// TestFullAPILifecycle tests the complete user journey:
// register → login → create app → add domain → list apps → delete app → cleanup
func TestFullAPILifecycle(t *testing.T) {
	// Setup: real SQLite + BBolt in temp dir
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	boltPath := filepath.Join(tmpDir, "test.bolt")

	sqlite, err := db.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("sqlite init: %v", err)
	}
	defer sqlite.Close()

	bolt, err := db.NewBoltStore(boltPath)
	if err != nil {
		t.Fatalf("bolt init: %v", err)
	}
	defer bolt.Close()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	events := core.NewEventBus(logger)
	registry := core.NewRegistry()
	services := core.NewServices()

	// Create marketplace registry
	mpRegistry := marketplace.NewTemplateRegistry()
	mpRegistry.LoadBuiltins()

	// Create auth module
	authMod := newTestAuthServices(t, "test-secret-key-for-integration-tests-32b")

	cfg, _ := core.LoadConfig("")
	c := &core.Core{
		Config:   cfg,
		Store:    sqlite,
		Events:   events,
		Logger:   logger,
		Registry: registry,
		Services: services,
		DB:       &core.Database{Bolt: bolt},
	}

	// Register modules
	registry.Register(authMod)

	// Register marketplace module (needs Init to load templates)
	mpMod := marketplace.New()
	mpMod.Init(context.Background(), c)
	registry.Register(mpMod)

	// Create router with real store
	router := NewRouter(c, authMod, sqlite)
	handler := router.Handler()
	server := httptest.NewServer(handler)
	defer server.Close()

	// Helper: make request
	doJSON := func(method, path string, body any, token string) *http.Response {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req, _ := http.NewRequest(method, server.URL+path, &buf)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}

	parseJSON := func(resp *http.Response) map[string]any {
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		return result
	}

	// ================================================================
	// Step 1: Health check
	// ================================================================
	t.Run("01_health", func(t *testing.T) {
		resp := doJSON("GET", "/health", nil, "")
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("health: %d", resp.StatusCode)
		}
		data := parseJSON(resp)
		if data["status"] != "ok" {
			t.Errorf("health status = %v", data["status"])
		}
	})

	// ================================================================
	// Step 2: Register a new user
	// ================================================================
	var accessToken string

	t.Run("02_register", func(t *testing.T) {
		resp := doJSON("POST", "/api/v1/auth/register", map[string]string{
			"email":    "test@deploy.monster",
			"password": "SecurePass123!",
			"name":     "Test User",
		}, "")
		data := parseJSON(resp)

		if resp.StatusCode != 201 {
			t.Fatalf("register: %d — %v", resp.StatusCode, data)
		}

		token, _ := data["access_token"].(string)
		if token == "" {
			t.Fatal("no access_token in register response")
		}
		accessToken = token
	})

	// ================================================================
	// Step 3: Login with registered credentials
	// ================================================================
	t.Run("03_login", func(t *testing.T) {
		resp := doJSON("POST", "/api/v1/auth/login", map[string]string{
			"email":    "test@deploy.monster",
			"password": "SecurePass123!",
		}, "")
		data := parseJSON(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("login: %d — %v", resp.StatusCode, data)
		}

		token, _ := data["access_token"].(string)
		if token == "" {
			t.Fatal("no access_token in login response")
		}
		accessToken = token
	})

	// ================================================================
	// Step 4: Get current user (me)
	// ================================================================
	t.Run("04_me", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/auth/me", nil, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("me: %d — %v", resp.StatusCode, data)
		}

		user, _ := data["user"].(map[string]any)
		if user == nil {
			t.Fatal("no user in me response")
		}
		if user["email"] != "test@deploy.monster" {
			t.Errorf("email = %v", user["email"])
		}
	})

	// ================================================================
	// Step 5: Create an application
	// ================================================================
	var appID string

	t.Run("05_create_app", func(t *testing.T) {
		resp := doJSON("POST", "/api/v1/apps", map[string]string{
			"name":        "my-test-app",
			"source_type": "image",
			"source_url":  "nginx:latest",
		}, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 201 {
			t.Fatalf("create app: %d — %v", resp.StatusCode, data)
		}

		id, _ := data["id"].(string)
		if id == "" {
			t.Fatal("no id in create app response")
		}
		appID = id
	})

	// ================================================================
	// Step 6: Get the created app
	// ================================================================
	t.Run("06_get_app", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/apps/"+appID, nil, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("get app: %d — %v", resp.StatusCode, data)
		}
		if data["name"] != "my-test-app" {
			t.Errorf("name = %v", data["name"])
		}
		if data["source_url"] != "nginx:latest" {
			t.Errorf("source_url = %v", data["source_url"])
		}
	})

	// ================================================================
	// Step 7: List apps (should have 1)
	// ================================================================
	t.Run("07_list_apps", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/apps", nil, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("list apps: %d — %v", resp.StatusCode, data)
		}

		apps, _ := data["data"].([]any)
		if len(apps) != 1 {
			t.Errorf("expected 1 app, got %d", len(apps))
		}
	})

	// ================================================================
	// Step 8: Add a domain
	// ================================================================
	t.Run("08_add_domain", func(t *testing.T) {
		resp := doJSON("POST", "/api/v1/domains", map[string]string{
			"fqdn":   "myapp.deploy.monster",
			"app_id": appID,
		}, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 201 {
			t.Fatalf("add domain: %d — %v", resp.StatusCode, data)
		}
	})

	// ================================================================
	// Step 9: List domains (should have 1)
	// ================================================================
	t.Run("09_list_domains", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/domains", nil, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("list domains: %d — %v", resp.StatusCode, data)
		}

		domains, _ := data["data"].([]any)
		if len(domains) != 1 {
			t.Errorf("expected 1 domain, got %d", len(domains))
		}
	})

	// ================================================================
	// Step 10: Marketplace templates
	// ================================================================
	t.Run("10_marketplace", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/marketplace", nil, accessToken)
		data := parseJSON(resp)

		if resp.StatusCode != 200 {
			t.Fatalf("marketplace: %d — %v", resp.StatusCode, data)
		}

		total, _ := data["total"].(float64)
		if total < 20 {
			t.Errorf("expected 20+ templates, got %v", total)
		}
	})

	// ================================================================
	// Step 11: Dashboard stats
	// ================================================================
	t.Run("11_dashboard", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/dashboard/stats", nil, accessToken)
		if resp.StatusCode != 200 {
			t.Fatalf("dashboard: %d", resp.StatusCode)
		}
		data := parseJSON(resp)
		apps, _ := data["apps"].(map[string]any)
		if apps != nil {
			total, _ := apps["total"].(float64)
			if total != 1 {
				t.Errorf("dashboard apps total = %v, want 1", total)
			}
		}
	})

	// ================================================================
	// Step 12: Delete the app
	// ================================================================
	t.Run("12_delete_app", func(t *testing.T) {
		resp := doJSON("DELETE", "/api/v1/apps/"+appID, nil, accessToken)
		defer resp.Body.Close()

		if resp.StatusCode != 200 && resp.StatusCode != 204 {
			t.Fatalf("delete app: %d", resp.StatusCode)
		}
	})

	// ================================================================
	// Step 13: Verify app is deleted
	// ================================================================
	t.Run("13_verify_deleted", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/apps/"+appID, nil, accessToken)
		defer resp.Body.Close()

		if resp.StatusCode != 404 {
			t.Errorf("expected 404 for deleted app, got %d", resp.StatusCode)
		}
	})

	// ================================================================
	// Step 14: Unauthorized access (no token)
	// ================================================================
	t.Run("14_unauthorized", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/apps", nil, "")
		defer resp.Body.Close()

		if resp.StatusCode != 401 {
			t.Errorf("expected 401 without token, got %d", resp.StatusCode)
		}
	})

	// ================================================================
	// Step 15: Invalid token
	// ================================================================
	t.Run("15_invalid_token", func(t *testing.T) {
		resp := doJSON("GET", "/api/v1/apps", nil, "invalid.jwt.token")
		defer resp.Body.Close()

		if resp.StatusCode != 401 {
			t.Errorf("expected 401 with invalid token, got %d", resp.StatusCode)
		}
	})

	_ = c // keep core reference alive
	_ = context.Background()
}
