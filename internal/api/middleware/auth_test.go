package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
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

// mockBoltStore implements core.BoltStorer for testing
type mockBoltStore struct {
	apiKeys map[string]*models.APIKey
}

func newMockBoltStore() *mockBoltStore {
	return &mockBoltStore{
		apiKeys: make(map[string]*models.APIKey),
	}
}

func (m *mockBoltStore) Set(bucket, key string, value any, ttlSeconds int64) error {
	return nil
}

func (m *mockBoltStore) Get(bucket, key string, dest any) error {
	return nil
}

func (m *mockBoltStore) Delete(bucket, key string) error {
	return nil
}

func (m *mockBoltStore) List(bucket string) ([]string, error) {
	return nil, nil
}

func (m *mockBoltStore) Close() error {
	return nil
}

func (m *mockBoltStore) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	if key, ok := m.apiKeys[prefix]; ok {
		return key, nil
	}
	return nil, ErrAPIKeyNotFound
}

func (m *mockBoltStore) GetWebhookSecret(webhookID string) (string, error) {
	return "", nil
}

// Ensure mockBoltStore implements core.BoltStorer
var _ core.BoltStorer = (*mockBoltStore)(nil)

// Test error for mock
var ErrAPIKeyNotFound = errors.New("api key not found")

func TestRequireAuth_ValidBearerToken(t *testing.T) {
	jwtSvc := testJWT()
	token := generateTestToken("user-1", "tenant-1", "role_admin", "admin@test.com")

	handler := RequireAuth(jwtSvc, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := RequireAuth(jwtSvc, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := RequireAuth(jwtSvc, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	bolt := newMockBoltStore()
	// Add a test API key to the mock store
	bolt.apiKeys["dm_test_"] = &models.APIKey{
		ID:        "key-1",
		UserID:    "api-key-user",
		TenantID:  "api-key-tenant",
		KeyPrefix: "dm_test_",
		KeyHash:   "dm_test_api_key_12345", // In real code this would be hashed
		CreatedAt: time.Now(),
	}

	handler := RequireAuth(jwtSvc, bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	handler := RequireAuth(jwtSvc, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	handler := RequireAuth(jwtSvc, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
