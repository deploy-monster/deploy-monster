package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/db"
)

func TestRequireAuth_RealBoltAdminAPIKeyRecord(t *testing.T) {
	bolt, err := db.NewBoltStore(t.TempDir() + "/api-keys.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	t.Cleanup(func() { _ = bolt.Close() })

	pair, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	rec := struct {
		Prefix    string    `json:"prefix"`
		Hash      string    `json:"hash"`
		Type      string    `json:"type"`
		CreatedBy string    `json:"created_by"`
		CreatedAt time.Time `json:"created_at"`
	}{
		Prefix:    pair.Prefix,
		Hash:      pair.Hash,
		Type:      "platform",
		CreatedBy: "user-1",
		CreatedAt: time.Now(),
	}
	if err := bolt.Set("api_keys", pair.Prefix, rec, 0); err != nil {
		t.Fatalf("Set api key: %v", err)
	}

	jwtSvc := testJWT()
	handler := RequireAuth(jwtSvc, bolt, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := auth.ClaimsFromContext(r.Context())
		if claims == nil {
			t.Fatal("expected claims in context")
		}
		if claims.UserID != "user-1" {
			t.Fatalf("claims.UserID = %q, want user-1", claims.UserID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("X-API-Key", pair.Key)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
}
