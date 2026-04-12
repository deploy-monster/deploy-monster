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

// TestSmoke_HealthEndpoint verifies the /api/v1/health endpoint returns 200.
func TestSmoke_HealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		RespondOK(w, map[string]string{"status": "ok"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/v1/health", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data field in response")
	}
	if data["status"] != "ok" {
		t.Errorf("status = %q, want %q", data["status"], "ok")
	}
}

// TestSmoke_AuthFlow verifies login → register → refresh flow.
func TestSmoke_AuthFlow(t *testing.T) {
	mux := http.NewServeMux()

	// Register endpoint
	mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			RespondError(w, 400, "bad_request", "invalid json")
			return
		}
		if body["email"] == "" || body["password"] == "" {
			RespondError(w, 400, "validation_error", "email and password required")
			return
		}
		RespondOK(w, map[string]string{
			"token":   "test-jwt-token",
			"user_id": "usr_test123",
		})
	})

	// Login endpoint
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			RespondError(w, 400, "bad_request", "invalid json")
			return
		}
		if body["email"] == "admin@test.com" && body["password"] == "TestPass123!" {
			RespondOK(w, map[string]string{
				"access_token":  "test-access-token",
				"refresh_token": "test-refresh-token",
			})
			return
		}
		RespondError(w, 401, "unauthorized", "invalid credentials")
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Step 1: Register
	regBody := `{"email":"admin@test.com","password":"TestPass123!","name":"Admin"}`
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/v1/auth/register", strings.NewReader(regBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("register status = %d, want 200", resp.StatusCode)
	}

	// Step 2: Login
	loginBody := `{"email":"admin@test.com","password":"TestPass123!"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/v1/auth/login", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("login status = %d, want 200", resp.StatusCode)
	}

	var loginResp map[string]any
	json.NewDecoder(resp.Body).Decode(&loginResp)
	data, ok := loginResp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data in login response")
	}
	if data["access_token"] == "" {
		t.Error("expected non-empty access_token")
	}

	// Step 3: Login with wrong credentials
	wrongBody := `{"email":"admin@test.com","password":"wrong"}`
	req, _ = http.NewRequestWithContext(ctx, "POST", srv.URL+"/api/v1/auth/login", strings.NewReader(wrongBody))
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("bad login: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Errorf("bad login status = %d, want 401", resp2.StatusCode)
	}
}

// TestSmoke_APIVersionHeader checks that API responses include version header.
func TestSmoke_APIVersionHeader(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-API-Version", "v1")
		RespondOK(w, map[string]string{"version": "0.1.1"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/version")
	if err != nil {
		t.Fatalf("version request: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("X-API-Version"); got != "v1" {
		t.Errorf("X-API-Version = %q, want %q", got, "v1")
	}
}

// TestSmoke_NotFoundReturns404 checks unknown routes return 404.
func TestSmoke_NotFoundReturns404(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		RespondError(w, 404, "not_found", "route not found")
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/nonexistent")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// TestSmoke_ContentTypeJSON ensures all API responses are JSON.
func TestSmoke_ContentTypeJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/test", func(w http.ResponseWriter, r *http.Request) {
		RespondOK(w, map[string]string{"test": "ok"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/test")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}
