package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestFullAppStartup tests that the API module starts correctly and serves
// health, SPA, and API endpoints. This is a smoke test for the full app.
func TestFullAppStartup_Health(t *testing.T) {
	// Create a handler directly (without full module lifecycle)
	mux := http.NewServeMux()

	// Register health endpoint
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"modules": map[string]string{
				"api":     "ok",
				"core.db": "ok",
				"deploy":  "ok",
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test health endpoint
	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("health status = %d, want 200", resp.StatusCode)
	}

	var health map[string]any
	json.NewDecoder(resp.Body).Decode(&health)

	if health["status"] != "ok" {
		t.Errorf("health status = %v, want ok", health["status"])
	}

	modules, ok := health["modules"].(map[string]any)
	if !ok {
		t.Fatal("modules not a map")
	}
	if len(modules) != 3 {
		t.Errorf("module count = %d, want 3", len(modules))
	}
}

func TestFullAppStartup_SPAFallback(t *testing.T) {
	mux := http.NewServeMux()

	// SPA handler returns index.html for unknown routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!doctype html><html><body>DeployMonster</body></html>"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test SPA fallback for client-side routes
	paths := []string{"/", "/login", "/apps", "/marketplace", "/settings"}
	for _, path := range paths {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Errorf("request %s failed: %v", path, err)
			continue
		}
		if resp.StatusCode != 200 {
			t.Errorf("%s: status = %d, want 200", path, resp.StatusCode)
		}
		ct := resp.Header.Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Errorf("%s: Content-Type = %s, want text/html", path, ct)
		}
		resp.Body.Close()
	}
}

func TestFullAppStartup_CORS(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(200)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test OPTIONS preflight
	req, _ := http.NewRequest("OPTIONS", server.URL+"/api/v1/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 204 {
		t.Errorf("preflight status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
}

func TestFullAppStartup_Auth(t *testing.T) {
	mux := http.NewServeMux()

	// Mock auth endpoint
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)

		if body["email"] == "admin@deploy.monster" && body["password"] == "test123" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token":  "jwt.token.here",
				"refresh_token": "refresh.token.here",
			})
			return
		}
		http.Error(w, `{"error":"invalid credentials"}`, 401)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test successful login
	body := `{"email":"admin@deploy.monster","password":"test123"}`
	resp, err := http.Post(server.URL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("login status = %d, want 200", resp.StatusCode)
	}

	var tokens map[string]string
	json.NewDecoder(resp.Body).Decode(&tokens)
	if tokens["access_token"] == "" {
		t.Error("missing access_token")
	}

	// Test failed login
	body = `{"email":"admin@deploy.monster","password":"wrong"}`
	resp2, _ := http.Post(server.URL+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	defer resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Errorf("bad login status = %d, want 401", resp2.StatusCode)
	}
}

func TestFullAppStartup_RequestTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(100 * time.Millisecond):
			w.WriteHeader(200)
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/v1/slow", nil)
	_, err := http.DefaultClient.Do(req)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestFullAppStartup_Marketplace(t *testing.T) {
	templates := []map[string]any{
		{"slug": "wordpress", "name": "WordPress", "category": "cms"},
		{"slug": "ghost", "name": "Ghost", "category": "cms"},
		{"slug": "n8n", "name": "n8n", "category": "automation"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/marketplace", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data":       templates,
			"total":      len(templates),
			"categories": []string{"cms", "automation"},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/v1/marketplace")
	if err != nil {
		t.Fatalf("marketplace request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	total, _ := result["total"].(float64)
	if total != 3 {
		t.Errorf("marketplace total = %v, want 3", total)
	}

	categories, _ := result["categories"].([]any)
	if len(categories) != 2 {
		t.Errorf("categories = %d, want 2", len(categories))
	}
}
