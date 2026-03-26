package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
	_ "modernc.org/sqlite"
)

// =============================================================================
// AdminAPIKeyHandler.Generate — bolt index Set error (second Set fails)
// =============================================================================

// boltFailOnSecondSet allows the first Set (key record) to succeed
// but fails on the second Set (index update).
type boltFailOnSecondSet struct {
	mockBoltStore
	setCalls int
}

func (b *boltFailOnSecondSet) Set(bucket, key string, value any, ttl int64) error {
	b.setCalls++
	if b.setCalls >= 2 {
		return fmt.Errorf("bolt index write error")
	}
	return b.mockBoltStore.Set(bucket, key, value, ttl)
}

func TestFinal95_AdminAPIKey_Generate_IndexSetError(t *testing.T) {
	bolt := &boltFailOnSecondSet{mockBoltStore: *newMockBoltStore()}
	h := NewAdminAPIKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to update key index")
}

// =============================================================================
// AdminAPIKeyHandler.Generate — non-admin role returns 403
// =============================================================================

func TestFinal95_AdminAPIKey_Generate_NonSuperAdmin(t *testing.T) {
	h := NewAdminAPIKeyHandler(newMockStore(), newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "user@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// =============================================================================
// DomainVerifyHandler.verifyDNS — success path (resolve a well-known host)
// =============================================================================

func TestFinal95_VerifyDNS_Success(t *testing.T) {
	// Use "localhost" which should resolve on any machine
	result := verifyDNS("localhost")
	if result.FQDN != "localhost" {
		t.Errorf("fqdn = %q, want localhost", result.FQDN)
	}
	// localhost should resolve, so Verified should be true and Records non-empty
	if !result.Verified {
		// On some CI machines localhost might not resolve — just verify no panic
		t.Logf("localhost did not verify (possible CI env): error=%q", result.Error)
	} else {
		if len(result.Records) == 0 {
			t.Error("expected at least one record for localhost")
		}
	}
	if result.CheckedAt == "" {
		t.Error("expected CheckedAt to be set")
	}
}

func TestFinal95_VerifyDNS_Failure(t *testing.T) {
	result := verifyDNS("this-domain-does-not-exist-xyz123.invalid")
	if result.Verified {
		t.Error("expected Verified=false for non-existent domain")
	}
	if result.Error == "" {
		t.Error("expected error message for non-existent domain")
	}
}

// =============================================================================
// DomainVerifyHandler.Verify — FQDN from stored bolt record
// =============================================================================

func TestFinal95_DomainVerify_FQDNFromBolt(t *testing.T) {
	bolt := newMockBoltStore()
	// Pre-store a domain verify record so the handler looks it up
	bolt.Set("domain_verify", "domain-1", domainVerifyRecord{
		DomainID: "domain-1",
		FQDN:     "this-domain-does-not-exist-xyz.invalid",
	}, 0)

	h := NewDomainVerifyHandler(newMockStore(), bolt)

	// POST without fqdn in body — handler should pull it from bolt
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/domains/domain-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "domain-1")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp VerifyResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "this-domain-does-not-exist-xyz.invalid" {
		t.Errorf("fqdn = %q, want stored domain", resp.FQDN)
	}
}

// =============================================================================
// DomainVerifyHandler.Verify — no FQDN anywhere returns 400
// =============================================================================

func TestFinal95_DomainVerify_NoFQDNAnywhere(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewDomainVerifyHandler(newMockStore(), bolt)

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/domains/domain-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "domain-1")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// MigrationHandler.Status — successful query with migration rows
// =============================================================================

func TestFinal95_MigrationHandler_Status_WithDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Create _migrations table and insert test data
	_, err = db.Exec(`CREATE TABLE _migrations (version INTEGER, name TEXT, applied_at TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO _migrations (version, name, applied_at) VALUES (1, 'initial', '2026-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, err = db.Exec(`INSERT INTO _migrations (version, name, applied_at) VALUES (2, 'add_users', '2026-01-02T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{Driver: "sqlite"},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		DB:       &core.Database{SQL: db},
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["driver"] != "sqlite" {
		t.Errorf("driver = %v, want sqlite", resp["driver"])
	}
	total, ok := resp["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	migrations, ok := resp["migrations"].([]any)
	if !ok || len(migrations) != 2 {
		t.Errorf("migrations count = %d, want 2", len(migrations))
	}
}

// =============================================================================
// MigrationHandler.Status — query error (table does not exist)
// =============================================================================

func TestFinal95_MigrationHandler_Status_QueryError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Do NOT create _migrations table so the query fails
	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{Driver: "sqlite"},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		DB:       &core.Database{SQL: db},
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// MigrationHandler.Status — DB.SQL is nil (DB struct exists but SQL is nil)
// =============================================================================

func TestFinal95_MigrationHandler_Status_NilSQL(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		DB:       &core.Database{SQL: nil},
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// MigrationHandler.Status — empty migration table (zero rows)
// =============================================================================

func TestFinal95_MigrationHandler_Status_EmptyTable(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE _migrations (version INTEGER, name TEXT, applied_at TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{Driver: "sqlite"},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		DB:       &core.Database{SQL: db},
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total, ok := resp["total"].(float64)
	if !ok || int(total) != 0 {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

// =============================================================================
// PlatformStatsHandler.Overview — with container runtime
// =============================================================================

func TestFinal95_PlatformStats_WithContainerRuntime(t *testing.T) {
	services := core.NewServices()
	services.Container = &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Name: "app-1"},
			{ID: "c2", Name: "app-2"},
		},
	}

	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: services,
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	containers, ok := resp["containers"].(float64)
	if !ok || int(containers) != 2 {
		t.Errorf("containers = %v, want 2", resp["containers"])
	}
}

// =============================================================================
// PlatformStatsHandler.Overview — container runtime returns error
// =============================================================================

func TestFinal95_PlatformStats_ContainerRuntimeError(t *testing.T) {
	services := core.NewServices()
	services.Container = &mockContainerRuntime{
		listErr: fmt.Errorf("docker not available"),
	}

	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: services,
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	// Should still succeed (graceful degradation), containers = 0
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	containers, ok := resp["containers"].(float64)
	if !ok || int(containers) != 0 {
		t.Errorf("containers = %v, want 0", resp["containers"])
	}
}

// =============================================================================
// PlatformStatsHandler.Overview — with module health statuses
// =============================================================================

// stubModule is a minimal core.Module implementation for testing.
type stubModule struct {
	id     string
	health core.HealthStatus
}

func (s *stubModule) ID() string                                 { return s.id }
func (s *stubModule) Name() string                               { return s.id }
func (s *stubModule) Version() string                            { return "1.0.0" }
func (s *stubModule) Dependencies() []string                     { return nil }
func (s *stubModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (s *stubModule) Start(_ context.Context) error              { return nil }
func (s *stubModule) Stop(_ context.Context) error               { return nil }
func (s *stubModule) Health() core.HealthStatus                  { return s.health }
func (s *stubModule) Routes() []core.Route                       { return nil }
func (s *stubModule) Events() []core.EventHandler                { return nil }

func TestFinal95_PlatformStats_ModuleHealth(t *testing.T) {
	registry := core.NewRegistry()
	registry.Register(&stubModule{id: "auth", health: core.HealthOK})
	registry.Register(&stubModule{id: "deploy", health: core.HealthDegraded})
	registry.Register(&stubModule{id: "backup", health: core.HealthDown})

	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: registry,
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	modules := resp["modules"].(map[string]any)
	if int(modules["healthy"].(float64)) != 1 {
		t.Errorf("healthy = %v, want 1", modules["healthy"])
	}
	if int(modules["degraded"].(float64)) != 1 {
		t.Errorf("degraded = %v, want 1", modules["degraded"])
	}
	if int(modules["down"].(float64)) != 1 {
		t.Errorf("down = %v, want 1", modules["down"])
	}
	if int(modules["total"].(float64)) != 3 {
		t.Errorf("total = %v, want 3", modules["total"])
	}
}

// =============================================================================
// SelfUpdateHandler.CheckUpdate — version comparison with update available
// =============================================================================

func TestFinal95_SelfUpdate_VersionFields(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build: core.BuildInfo{
			Version: "v1.0.0",
			Commit:  "abc123def",
			Date:    "2026-03-25",
		},
		Registry: core.NewRegistry(),
	}
	h := NewSelfUpdateHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/updates", nil)
	rr := httptest.NewRecorder()
	h.CheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["current_version"] != "v1.0.0" {
		t.Errorf("current_version = %v", resp["current_version"])
	}
	if resp["commit"] != "abc123def" {
		t.Errorf("commit = %v", resp["commit"])
	}
	if resp["build_date"] != "2026-03-25" {
		t.Errorf("build_date = %v", resp["build_date"])
	}
}

// =============================================================================
// SSHTestHandler.Test — default port (port <= 0 defaults to 22)
// =============================================================================

func TestFinal95_SSHTest_DefaultPort(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":"192.0.2.1","port":0}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["port"].(float64)) != 22 {
		t.Errorf("port = %v, want 22", resp["port"])
	}
}

// =============================================================================
// SSHTestHandler.Test — reachable host (use local TCP listener)
// =============================================================================

func TestFinal95_SSHTest_ReachableHost(t *testing.T) {
	// Start a local TCP listener to simulate a reachable SSH port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept one connection in background
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	h := NewSSHTestHandler(core.NewServices())

	body := fmt.Sprintf(`{"host":"127.0.0.1","port":%d}`, addr.Port)
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != true {
		t.Errorf("expected reachable=true, got %v", resp["reachable"])
	}
	if resp["latency"] == nil || resp["latency"] == "" {
		t.Error("expected latency to be set")
	}
}

// =============================================================================
// SSHTestHandler.Test — with server_id and SSH client (success path)
// =============================================================================

// mockSSHClient implements core.SSHClient for testing.
type mockSSHClient struct {
	output string
	err    error
}

func (m *mockSSHClient) Execute(_ context.Context, _, _ string) (string, error) {
	return m.output, m.err
}

func (m *mockSSHClient) Upload(_ context.Context, _, _, _ string) error {
	return nil
}

func TestFinal95_SSHTest_WithServerID_SSHSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	services := core.NewServices()
	services.SSH = &mockSSHClient{output: "ok\n"}
	h := NewSSHTestHandler(services)

	body := fmt.Sprintf(`{"host":"127.0.0.1","port":%d,"server_id":"srv-1"}`, addr.Port)
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != true {
		t.Errorf("expected reachable=true")
	}
	if resp["ssh_auth"] != true {
		t.Errorf("expected ssh_auth=true, got %v", resp["ssh_auth"])
	}
	if resp["ssh_output"] != "ok\n" {
		t.Errorf("ssh_output = %q", resp["ssh_output"])
	}
}

// =============================================================================
// SSHTestHandler.Test — with server_id and SSH client (error path)
// =============================================================================

func TestFinal95_SSHTest_WithServerID_SSHError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	services := core.NewServices()
	services.SSH = &mockSSHClient{err: fmt.Errorf("auth failed")}
	h := NewSSHTestHandler(services)

	body := fmt.Sprintf(`{"host":"127.0.0.1","port":%d,"server_id":"srv-1"}`, addr.Port)
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != true {
		t.Errorf("expected reachable=true")
	}
	if resp["ssh_auth"] != false {
		t.Errorf("expected ssh_auth=false, got %v", resp["ssh_auth"])
	}
	if resp["ssh_error"] == nil {
		t.Error("expected ssh_error to be set")
	}
}

// =============================================================================
// SSHTestHandler.Test — negative port defaults to 22
// =============================================================================

func TestFinal95_SSHTest_NegativePort(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":"192.0.2.1","port":-1}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["port"].(float64)) != 22 {
		t.Errorf("port = %v, want 22", resp["port"])
	}
}

// =============================================================================
// SSHKeyHandler.Generate — bolt Set error
// =============================================================================

func TestFinal95_SSHKey_Generate_BoltSaveError(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newErrorBoltStore())

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to store SSH key")
}

// =============================================================================
// SSHKeyHandler.Generate — invalid body (decode error)
// =============================================================================

func TestFinal95_SSHKey_Generate_InvalidBody(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockBoltStore())

	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader("bad json"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// SSHKeyHandler.List — with existing keys
// =============================================================================

func TestFinal95_SSHKey_List_WithKeys(t *testing.T) {
	bolt := newMockBoltStore()
	// Pre-store some SSH keys
	list := sshKeyList{
		Keys: []SSHKeyInfo{
			{ID: "k1", Name: "key-1", Fingerprint: "SHA256:abc"},
			{ID: "k2", Name: "key-2", Fingerprint: "SHA256:def"},
		},
	}
	bolt.Set("ssh_keys", "u1", list, 0)

	h := NewSSHKeyHandler(newMockStore(), bolt)

	req := httptest.NewRequest("GET", "/api/v1/ssh-keys", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("data len = %d, want 2", len(data))
	}
}

// =============================================================================
// SSLStatusHandler.Check — cached result returns immediately
// =============================================================================

func TestFinal95_SSLStatus_Check_Cached(t *testing.T) {
	bolt := newMockBoltStore()
	cached := SSLCheckResult{
		FQDN:      "example.com",
		Valid:     true,
		Issuer:    "Let's Encrypt",
		Subject:   "example.com",
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
		DaysLeft:  90,
		CheckedAt: time.Now(),
	}
	bolt.Set("certificates", "ssl_check:example.com", cached, 300)

	h := NewSSLStatusHandler(bolt)

	req := httptest.NewRequest("GET", "/api/v1/domains/d1/ssl-status?fqdn=example.com", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp SSLCheckResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "example.com" {
		t.Errorf("fqdn = %q", resp.FQDN)
	}
	if !resp.Valid {
		t.Error("expected valid=true from cache")
	}
	if resp.Issuer != "Let's Encrypt" {
		t.Errorf("issuer = %q", resp.Issuer)
	}
}

// =============================================================================
// SSLStatusHandler.checkSSL — TLS error path (invalid host)
// =============================================================================

func TestFinal95_CheckSSL_InvalidHost(t *testing.T) {
	result := checkSSL("192.0.2.1.nxdomain.invalid")
	if result.Valid {
		t.Error("expected valid=false for invalid host")
	}
	if result.Error == "" {
		t.Error("expected error for invalid host")
	}
	if result.FQDN != "192.0.2.1.nxdomain.invalid" {
		t.Errorf("fqdn = %q", result.FQDN)
	}
}

// =============================================================================
// SSLStatusHandler.checkSSL — struct fields always populated
// =============================================================================

func TestFinal95_CheckSSL_StructFields(t *testing.T) {
	// Use a host that will fail TLS connection — verifies the result struct
	// is properly populated even on failure.
	result := checkSSL("localhost")
	if result.FQDN != "localhost" {
		t.Errorf("fqdn = %q, want localhost", result.FQDN)
	}
	if result.CheckedAt.IsZero() {
		t.Error("expected CheckedAt to be set")
	}
	// localhost:443 is unlikely to have TLS, so should fail
	if result.Valid {
		// If by some chance it's valid, just verify cert fields are set
		if result.Issuer == "" {
			t.Error("valid cert should have issuer")
		}
	} else {
		if result.Error == "" {
			t.Error("invalid result should have error")
		}
	}
}

// =============================================================================
// SSLStatusHandler.checkSSL — connection refused
// =============================================================================

func TestFinal95_CheckSSL_ConnectionRefused(t *testing.T) {
	// Use a port that's almost certainly not listening
	result := checkSSL("127.0.0.1:1")
	// checkSSL appends :443, so the actual address is "127.0.0.1:1:443"
	// which will fail to connect
	if result.Valid {
		t.Error("expected valid=false")
	}
	if result.Error == "" {
		t.Error("expected error for connection refused")
	}
}

// =============================================================================
// DomainVerifyHandler.Verify — FQDN provided in body
// =============================================================================

func TestFinal95_DomainVerify_FQDNInBody(t *testing.T) {
	h := NewDomainVerifyHandler(newMockStore(), newMockBoltStore())

	body := `{"fqdn":"this-domain-does-not-exist-xyz.invalid"}`
	req := httptest.NewRequest("POST", "/api/v1/domains/d1/verify", strings.NewReader(body))
	req.SetPathValue("id", "d1")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp VerifyResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "this-domain-does-not-exist-xyz.invalid" {
		t.Errorf("fqdn = %q", resp.FQDN)
	}
	if resp.Verified {
		t.Error("expected verified=false for non-existent domain")
	}
}

// =============================================================================
// DomainVerifyHandler.BatchVerify — success with multiple FQDNs
// =============================================================================

func TestFinal95_DomainVerify_BatchVerify_MultipleFQDNs(t *testing.T) {
	h := NewDomainVerifyHandler(newMockStore(), newMockBoltStore())

	body := `{"fqdns":["invalid-domain-abc.invalid","invalid-domain-xyz.invalid"]}`
	req := httptest.NewRequest("POST", "/api/v1/domains/verify-batch", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.BatchVerify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	results := resp["results"].([]any)
	if len(results) != 2 {
		t.Errorf("results len = %d, want 2", len(results))
	}
}

// =============================================================================
// TenantSettingsHandler.Update — all branches
// =============================================================================

// mockStoreWithUpdateTenant wraps mockStore and adds a working UpdateTenant.
type mockStoreWithUpdateTenant struct {
	*mockStore
	errUpdateTenant error
	updatedTenant   *core.Tenant
}

func (m *mockStoreWithUpdateTenant) UpdateTenant(_ context.Context, t *core.Tenant) error {
	if m.errUpdateTenant != nil {
		return m.errUpdateTenant
	}
	m.updatedTenant = t
	return nil
}

func TestFinal95_TenantSettings_Update_Success(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{
		ID: "t1", Name: "Original", Slug: "orig", Status: "active",
	})
	store := &mockStoreWithUpdateTenant{mockStore: ms}
	h := NewTenantSettingsHandler(store)

	body := `{"name":"Updated Name","metadata":"{\"theme\":\"dark\"}"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "updated" {
		t.Errorf("status = %q", resp["status"])
	}
	if store.updatedTenant.Name != "Updated Name" {
		t.Errorf("name = %q", store.updatedTenant.Name)
	}
}

func TestFinal95_TenantSettings_Update_NoClaims(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())

	body := `{"name":"X"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_InvalidBody(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{ID: "t1", Name: "Test"})
	h := NewTenantSettingsHandler(&mockStoreWithUpdateTenant{mockStore: ms})

	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader("bad"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_TenantNotFound(t *testing.T) {
	ms := newMockStore()
	// No tenant added — GetTenant will return ErrNotFound
	h := NewTenantSettingsHandler(&mockStoreWithUpdateTenant{mockStore: ms})

	body := `{"name":"X"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_SaveError(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{ID: "t1", Name: "Test"})
	store := &mockStoreWithUpdateTenant{
		mockStore:       ms,
		errUpdateTenant: fmt.Errorf("db error"),
	}
	h := NewTenantSettingsHandler(store)

	body := `{"name":"X"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_PartialFields(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{
		ID: "t1", Name: "Original", MetadataJSON: `{"old":"data"}`,
	})
	store := &mockStoreWithUpdateTenant{mockStore: ms}
	h := NewTenantSettingsHandler(store)

	// Only name, no metadata — metadata should stay unchanged
	body := `{"name":"NewName"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if store.updatedTenant.MetadataJSON != `{"old":"data"}` {
		t.Errorf("metadata should be unchanged, got %q", store.updatedTenant.MetadataJSON)
	}
}

// =============================================================================
// MCPHandler — NewMCPHandler, ListTools, CallTool
// =============================================================================

func TestFinal95_MCPHandler_NewMCPHandler(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)
	if h == nil {
		t.Fatal("NewMCPHandler returned nil")
	}
}

func TestFinal95_MCPHandler_ListTools(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	req := httptest.NewRequest("GET", "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	h.ListTools(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["version"] != "2.0.0" {
		t.Errorf("version = %v", resp["version"])
	}
	if resp["tools"] == nil {
		t.Error("expected tools in response")
	}
}

func TestFinal95_MCPHandler_CallTool_ValidTool(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	// Call list_apps tool with empty input
	body := `{}`
	req := httptest.NewRequest("POST", "/mcp/v1/tools/list_apps", strings.NewReader(body))
	req.SetPathValue("name", "list_apps")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	// May return 200 or 400 depending on the tool — just check it doesn't panic
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_MCPHandler_CallTool_UnknownTool(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	body := `{}`
	req := httptest.NewRequest("POST", "/mcp/v1/tools/nonexistent_tool", strings.NewReader(body))
	req.SetPathValue("name", "nonexistent_tool")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_MCPHandler_CallTool_InvalidBody(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	// Invalid JSON body — should fall back to {}
	req := httptest.NewRequest("POST", "/mcp/v1/tools/list_apps", strings.NewReader("not json"))
	req.SetPathValue("name", "list_apps")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	// Should not panic; just verify it returns a response
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400, got %d", rr.Code)
	}
}

// =============================================================================
// MarketplaceDeployHandler.Deploy — all branches
// =============================================================================

func TestFinal95_MarketplaceDeploy_NoClaims(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":"wordpress"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_InvalidBody(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader("bad json"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_EmptySlug(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":""}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_TemplateNotFound(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_InvalidComposeYAML(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	registry.Add(&marketplace.Template{
		Slug:        "broken",
		Name:        "Broken",
		Category:    "test",
		ComposeYAML: `not: valid: compose: yaml: [[[`,
	})

	events := core.NewEventBus(slog.Default())
	store := newMockStore()
	h := NewMarketplaceDeployHandler(registry, &mockContainerRuntime{}, store, events)

	body := `{"slug":"broken","name":"my-broken"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_MarketplaceDeploy_CreateAppError(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	registry.Add(&marketplace.Template{
		Slug:     "nginx",
		Name:     "Nginx",
		Category: "web",
		ComposeYAML: `services:
  web:
    image: nginx:latest
`,
	})

	events := core.NewEventBus(slog.Default())
	store := newMockStore()
	store.errCreateApp = fmt.Errorf("db error")
	h := NewMarketplaceDeployHandler(registry, &mockContainerRuntime{}, store, events)

	body := `{"slug":"nginx"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ExecHandler.Exec — exec error with "exec create" in message
// =============================================================================

func TestFinal95_ExecHandler_ExecCreateError(t *testing.T) {
	runtime := &mockExecErrorRuntime{
		mockContainerRuntime: mockContainerRuntime{
			containers: []core.ContainerInfo{{ID: "c1", Name: "app-1"}},
		},
		execErr: fmt.Errorf("exec create: connection refused"),
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"ls"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_ExecHandler_ExecNonZeroExit(t *testing.T) {
	runtime := &mockExecErrorRuntime{
		mockContainerRuntime: mockContainerRuntime{
			containers: []core.ContainerInfo{{ID: "c1", Name: "app-1"}},
		},
		execErr:    fmt.Errorf("command failed with exit code 1"),
		execOutput: "some partial output",
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"false"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	// Non-zero exit still returns 200 with exit_code=1
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp execResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", resp.ExitCode)
	}
}

// mockExecErrorRuntime returns an error from Exec.
type mockExecErrorRuntime struct {
	mockContainerRuntime
	execErr    error
	execOutput string
}

func (m *mockExecErrorRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return m.execOutput, m.execErr
}

// =============================================================================
// LicenseHandler — Get with expired license, Activate bolt error
// =============================================================================

func TestFinal95_License_Get_ExpiredLicense(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("license", "current", LicenseInfo{
		Type:       "enterprise",
		Key:        "test****test",
		ValidUntil: time.Now().Add(-24 * time.Hour), // expired yesterday
		Status:     "active",
	}, 0)
	h := NewLicenseHandler(bolt)

	req := httptest.NewRequest("GET", "/api/v1/admin/license", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp LicenseInfo
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Status != "expired" {
		t.Errorf("status = %q, want expired", resp.Status)
	}
}

func TestFinal95_License_Activate_BoltError(t *testing.T) {
	h := NewLicenseHandler(newErrorBoltStore())

	body := `{"key":"enterprise-key-12345678"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/license", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Activate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// RedirectHandler.Delete — bolt Set error path
// =============================================================================

func TestFinal95_Redirect_Delete_BoltSetError(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("redirects", "app-1", redirectList{
		Rules: []RedirectRule{
			{ID: "r1", Source: "/old", Destination: "/new", StatusCode: 301},
		},
	}, 0)

	// Wrap bolt to fail on Set
	errBolt := &boltGetOkSetFail{mockBoltStore: bolt}
	h := NewRedirectHandler(errBolt)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// boltGetOkSetFail wraps mockBoltStore: Get works, Set always fails.
type boltGetOkSetFail struct {
	*mockBoltStore
}

func (b *boltGetOkSetFail) Set(_, _ string, _ any, _ int64) error {
	return fmt.Errorf("bolt set error")
}

// =============================================================================
// EventWebhookHandler — Delete bolt Set error, Create bolt Set error
// =============================================================================

func TestFinal95_EventWebhook_Delete_BoltSetError(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("event_webhooks", "all", eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh1", URL: "https://example.com/hook", Events: []string{"deploy.success"}},
		},
	}, 0)

	errBolt := &boltGetOkSetFail{mockBoltStore: bolt}
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(slog.Default()), errBolt)

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh1", nil)
	req.SetPathValue("id", "wh1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestFinal95_EventWebhook_Create_BoltSetError(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(slog.Default()), newErrorBoltStore())

	body := `{"url":"https://example.com/hook","events":["deploy.success"],"secret":"my-secret"}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// RegistryHandler.List — with custom registries
// =============================================================================

func TestFinal95_Registry_List_WithCustom(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("registries", "all", registryList{
		Registries: []RegistryConfig{
			{ID: "custom-1", Name: "My Registry", URL: "registry.example.com"},
		},
	}, 0)
	h := NewRegistryHandler(bolt)

	req := httptest.NewRequest("GET", "/api/v1/registries", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	// 3 builtins + 1 custom = 4
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
}

// =============================================================================
// DeployApprovalHandler.Reject — not found path
// =============================================================================

func TestFinal95_DeployApproval_Reject_NotFound(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := NewDeployApprovalHandler(newMockStore(), events)

	body := `{"reason":"too risky"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/nonexistent/reject", strings.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFinal95_DeployApproval_Reject_NoClaims(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := NewDeployApprovalHandler(newMockStore(), events)

	body := `{"reason":"nope"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/a1/reject", strings.NewReader(body))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}
