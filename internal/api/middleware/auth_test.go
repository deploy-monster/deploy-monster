package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
)

const testJWTSecret = "test-secret-key-for-jwt-minimum-32-bytes"

func testJWT() *auth.JWTService {
	return auth.NewJWTService(testJWTSecret)
}

func generateTestToken(userID, tenantID, roleID, email string) string {
	jwt := testJWT()
	pair, err := jwt.GenerateTokenPair(userID, tenantID, roleID, email)
	if err != nil {
		panic("generateTestToken: " + err.Error())
	}
	return pair.AccessToken
}

func TestRequireAuth_ValidBearerToken(t *testing.T) {
	jwtSvc := testJWT()
	token := generateTestToken("user-1", "tenant-1", "role_admin", "admin@test.com")

	handler := RequireAuth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("expected claims in context")
		}
		if claims.UserID != "user-1" {
			t.Errorf("expected user-1, got %q", claims.UserID)
		}
		if claims.TenantID != "tenant-1" {
			t.Errorf("expected tenant-1, got %q", claims.TenantID)
		}
		if claims.Email != "admin@test.com" {
			t.Errorf("expected admin@test.com, got %q", claims.Email)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", rr.Code)
	}
}

func TestRequireAuth_InvalidBearerToken(t *testing.T) {
	jwtSvc := testJWT()

	handler := RequireAuth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached with invalid token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", rr.Code)
	}
}

func TestRequireAuth_MissingAuthorization(t *testing.T) {
	jwtSvc := testJWT()

	handler := RequireAuth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached without authorization")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing auth, got %d", rr.Code)
	}
}

func TestRequireAuth_ValidAPIKey(t *testing.T) {
	jwtSvc := testJWT()

	handler := RequireAuth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("expected claims in context for API key auth")
		}
		if claims.UserID != "api-key-user" {
			t.Errorf("expected api-key-user, got %q", claims.UserID)
		}
		if claims.TenantID != "api-key-tenant" {
			t.Errorf("expected api-key-tenant, got %q", claims.TenantID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("X-API-Key", "dm_test_api_key_12345")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid API key, got %d", rr.Code)
	}
}

func TestRequireAuth_InvalidAPIKey(t *testing.T) {
	jwtSvc := testJWT()

	handler := RequireAuth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached with invalid API key")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("X-API-Key", "invalid_key_no_prefix")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid API key, got %d", rr.Code)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	// Use a different secret to create a token that won't validate
	wrongJWT := auth.NewJWTService("wrong-secret-key-at-least-32-bytes!!")
	pair, err := wrongJWT.GenerateTokenPair("user-1", "tenant-1", "role_admin", "admin@test.com")
	if err != nil {
		t.Fatalf("generating token: %v", err)
	}

	jwtSvc := testJWT() // validates with correct secret
	handler := RequireAuth(jwtSvc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be reached with wrong-secret token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong-secret token, got %d", rr.Code)
	}
}
