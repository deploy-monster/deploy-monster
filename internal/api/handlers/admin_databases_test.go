package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	internalAuth "github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// jwtAuth wraps a real *auth.JWTService so AdminHandler can call into
// it during the success path of RevokeAllKeys without needing the
// full auth Module.
type jwtAuth struct{ jwt *internalAuth.JWTService }

func (j jwtAuth) JWT() *internalAuth.JWTService   { return j.jwt }
func (j jwtAuth) TOTP() *internalAuth.TOTPService { return nil }

// ---------------------------------------------------------------------------
// AdminHandler.RevokeAllKeys
// ---------------------------------------------------------------------------

func TestAdminHandler_RevokeAllKeys_AuthServiceUnavailable(t *testing.T) {
	c := monitoringTestCore()
	h := NewAdminHandler(c, newMockStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/revoke-all", nil)
	rr := httptest.NewRecorder()
	h.RevokeAllKeys(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminHandler_RevokeAllKeys_NilJWTReturns503(t *testing.T) {
	// Pass an AuthServices implementation that hands back nil for JWT().
	c := monitoringTestCore()
	h := NewAdminHandler(c, newMockStore(), jwtAuth{jwt: nil})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/revoke-all", nil)
	rr := httptest.NewRecorder()
	h.RevokeAllKeys(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminHandler_RevokeAllKeys_Success(t *testing.T) {
	jwt, err := internalAuth.NewJWTService("primary-test-key-32-bytes-long-abc")
	if err != nil {
		t.Fatalf("NewJWTService: %v", err)
	}
	jwt.AddPreviousKey("rotated-key-32-bytes-long-1234567")
	jwt.AddPreviousKey("rotated-key-32-bytes-long-7654321")

	c := monitoringTestCore()
	h := NewAdminHandler(c, newMockStore(), jwtAuth{jwt: jwt})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys/revoke-all", nil)
	rr := httptest.NewRecorder()
	h.RevokeAllKeys(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Status      string `json:"status"`
		RevokedKeys int    `json:"revoked_keys"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.RevokedKeys != 2 {
		t.Errorf("revoked_keys = %d, want 2", resp.RevokedKeys)
	}
}

// ---------------------------------------------------------------------------
// DatabaseHandler.List
// ---------------------------------------------------------------------------

func TestDatabaseHandler_List_Unauthorized(t *testing.T) {
	h := NewDatabaseHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestDatabaseHandler_List_NoRuntimeReturnsEmpty(t *testing.T) {
	// runtime=nil short-circuits to an empty list with total=0, so the
	// handler stays usable on agent-less control planes.
	h := NewDatabaseHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/databases", nil),
		"u1", "tenant-1", "r1", "u@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data  []databaseInstanceView `json:"data"`
		Total int                    `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Total != 0 || len(resp.Data) != 0 {
		t.Fatalf("expected empty list when runtime is nil, got total=%d data=%d", resp.Total, len(resp.Data))
	}
}

func TestDatabaseHandler_List_WithRuntime(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "c1",
				Name:  "fallback-name",
				Image: "postgres:16",
				State: "running",
				Labels: map[string]string{
					"monster.db.name":   "primary",
					"monster.db.engine": "postgres",
				},
			},
			{
				ID:     "c2",
				Image:  "redis:7-alpine",
				State:  "exited",
				Labels: map[string]string{"monster.db.engine": "redis"},
			},
		},
	}
	h := NewDatabaseHandler(newMockStore(), runtime, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/databases", nil),
		"u1", "tenant-1", "r1", "u@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data  []databaseInstanceView `json:"data"`
		Total int                    `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Total != 2 || len(resp.Data) != 2 {
		t.Fatalf("total = %d, data len = %d, want 2/2", resp.Total, len(resp.Data))
	}

	// First container has monster.db.name set, so the view name should
	// pull from there rather than c.Name.
	if got := resp.Data[0].Name; got != "primary" {
		t.Errorf("first name = %q, want primary (from monster.db.name label)", got)
	}
	if got := resp.Data[0].Engine; got != "postgres" {
		t.Errorf("first engine = %q, want postgres", got)
	}
	if got := resp.Data[0].Version; got != "16" {
		t.Errorf("first version = %q, want 16 (from imageTag of postgres:16)", got)
	}
	if got := resp.Data[0].Status; got != "running" {
		t.Errorf("first status = %q, want running", got)
	}

	// Second container has no monster.db.name label and an empty Name;
	// the view name falls back to c.Name (also empty here, which is
	// the contract for unlabelled containers).
	if got := resp.Data[1].Status; got != "exited" {
		t.Errorf("second status = %q, want exited", got)
	}
	if got := resp.Data[1].Version; got != "7-alpine" {
		t.Errorf("second version = %q, want 7-alpine", got)
	}
}

func TestDatabaseHandler_List_RuntimeError(t *testing.T) {
	runtime := &mockContainerRuntime{listErr: errBoom}
	h := NewDatabaseHandler(newMockStore(), runtime, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/databases", nil),
		"u1", "tenant-1", "r1", "u@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when ListByLabels errors; body=%s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// imageTag helper
// ---------------------------------------------------------------------------

func TestImageTag(t *testing.T) {
	cases := []struct {
		name, image, want string
	}{
		{"basic version", "postgres:16", "16"},
		{"semver tag", "redis:7.2.4", "7.2.4"},
		{"tag with letters", "redis:7-alpine", "7-alpine"},
		{"no colon returns empty", "postgres", ""},
		{"colon followed by registry path", "registry.io:5000/postgres", ""},
		{"empty input", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := imageTag(tc.image); got != tc.want {
				t.Fatalf("imageTag(%q) = %q, want %q", tc.image, got, tc.want)
			}
		})
	}
}
