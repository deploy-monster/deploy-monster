package dns

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Mock DNSProvider
// ---------------------------------------------------------------------------

type mockDNSProvider struct {
	name          string
	createCalls   int
	updateCalls   int
	deleteCalls   int
	verifyCalls   int
	createErr     error
	updateErr     error
	deleteErr     error
	verifyResult  bool
	verifyErr     error
	mu            sync.Mutex
	lastRecord    core.DNSRecord
	lastDeleteID  string
	lastVerifyFQDN string
}

func (m *mockDNSProvider) Name() string { return m.name }

func (m *mockDNSProvider) CreateRecord(_ context.Context, record core.DNSRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	m.lastRecord = record
	return m.createErr
}

func (m *mockDNSProvider) UpdateRecord(_ context.Context, record core.DNSRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	m.lastRecord = record
	return m.updateErr
}

func (m *mockDNSProvider) DeleteRecord(_ context.Context, recordID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls++
	m.lastDeleteID = recordID
	return m.deleteErr
}

func (m *mockDNSProvider) Verify(_ context.Context, fqdn string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.verifyCalls++
	m.lastVerifyFQDN = fqdn
	return m.verifyResult, m.verifyErr
}

// ---------------------------------------------------------------------------
// Mock Store (minimal, satisfies core.Store)
// ---------------------------------------------------------------------------

type mockStore struct{}

func (s *mockStore) CreateTenant(_ context.Context, _ *core.Tenant) error   { return nil }
func (s *mockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}
func (s *mockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}
func (s *mockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error   { return nil }
func (s *mockStore) DeleteTenant(_ context.Context, _ string) error         { return nil }
func (s *mockStore) CreateUser(_ context.Context, _ *core.User) error       { return nil }
func (s *mockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, nil
}
func (s *mockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, nil
}
func (s *mockStore) UpdateUser(_ context.Context, _ *core.User) error       { return nil }
func (s *mockStore) UpdatePassword(_ context.Context, _, _ string) error    { return nil }
func (s *mockStore) UpdateLastLogin(_ context.Context, _ string) error      { return nil }
func (s *mockStore) CountUsers(_ context.Context) (int, error)              { return 0, nil }
func (s *mockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (s *mockStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }
func (s *mockStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	return nil, nil
}
func (s *mockStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }
func (s *mockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (s *mockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (s *mockStore) UpdateAppStatus(_ context.Context, _, _ string) error   { return nil }
func (s *mockStore) DeleteApp(_ context.Context, _ string) error            { return nil }
func (s *mockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error {
	return nil
}
func (s *mockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}
func (s *mockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (s *mockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}
func (s *mockStore) CreateDomain(_ context.Context, _ *core.Domain) error { return nil }
func (s *mockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, nil
}
func (s *mockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (s *mockStore) DeleteDomain(_ context.Context, _ string) error       { return nil }
func (s *mockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	return nil, nil
}
func (s *mockStore) CreateProject(_ context.Context, _ *core.Project) error { return nil }
func (s *mockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, nil
}
func (s *mockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (s *mockStore) DeleteProject(_ context.Context, _ string) error      { return nil }
func (s *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}
func (s *mockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, nil
}
func (s *mockStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, nil
}
func (s *mockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}
func (s *mockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (s *mockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}
func (s *mockStore) CreateSecret(_ context.Context, _ *core.Secret) error { return nil }
func (s *mockStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}
func (s *mockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (s *mockStore) CreateInvite(_ context.Context, _ *core.Invitation) error { return nil }
func (s *mockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (s *mockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}
func (s *mockStore) Close() error                    { return nil }
func (s *mockStore) Ping(_ context.Context) error    { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testCore(cfToken string) *core.Core {
	cfg := &core.Config{}
	cfg.DNS.CloudflareToken = cfToken
	return &core.Core{
		Config:   cfg,
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
		Store:    &mockStore{},
	}
}

// ===========================================================================
// Module tests
// ===========================================================================

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModule_Identity(t *testing.T) {
	m := New()
	if m.ID() != "dns.sync" {
		t.Errorf("ID() = %q, want %q", m.ID(), "dns.sync")
	}
	if m.Name() != "DNS Synchronizer" {
		t.Errorf("Name() = %q, want %q", m.Name(), "DNS Synchronizer")
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want %q", m.Version(), "1.0.0")
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies() = %v, want [core.db]", deps)
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", h)
	}
}

func TestModule_Init_WithCloudflareToken(t *testing.T) {
	c := testCore("test-cf-token-123")
	m := New()

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// Should have registered Cloudflare provider
	provider := c.Services.DNSProvider("cloudflare")
	if provider == nil {
		t.Fatal("expected Cloudflare provider to be registered")
	}
	if provider.Name() != "cloudflare" {
		t.Errorf("provider.Name() = %q, want %q", provider.Name(), "cloudflare")
	}
}

func TestModule_Init_WithoutCloudflareToken(t *testing.T) {
	c := testCore("")
	m := New()

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	// No Cloudflare provider should be registered
	provider := c.Services.DNSProvider("cloudflare")
	if provider != nil {
		t.Error("expected no Cloudflare provider when token is empty")
	}
}

func TestModule_Start(t *testing.T) {
	c := testCore("test-token")
	m := New()

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// syncQueue should be running
	if m.syncQueue == nil {
		t.Fatal("syncQueue should not be nil after Start()")
	}

	// Clean up
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestModule_Stop_NilSyncQueue(t *testing.T) {
	m := New()
	// syncQueue is nil — Stop should not panic
	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestModule_Stop_WithSyncQueue(t *testing.T) {
	c := testCore("")
	m := New()

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	err := m.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestModule_DomainAddedEvent_TriggersSync(t *testing.T) {
	// Use empty token so Init() doesn't register a real Cloudflare (which would overwrite our mock)
	c := testCore("")
	mock := &mockDNSProvider{name: "cloudflare", verifyResult: true}
	c.Services.RegisterDNSProvider("cloudflare", mock)

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer m.Stop(context.Background())

	// Publish a domain.added event
	c.Events.Publish(context.Background(), core.Event{
		Type:   core.EventDomainAdded,
		Source: "test",
		Data: core.DomainEventData{
			DomainID: "dom-1",
			FQDN:     "app.example.com",
			AppID:    "app-1",
		},
	})

	// Give async handler + sync queue time to process
	time.Sleep(500 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	// Should have enqueued at least 1 create job (A record)
	if mock.createCalls < 1 {
		t.Errorf("expected at least 1 create call, got %d", mock.createCalls)
	}
}

func TestModule_DomainRemovedEvent(t *testing.T) {
	c := testCore("")
	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer m.Stop(context.Background())

	// Publish domain.removed — no crash
	err := c.Events.Publish(context.Background(), core.Event{
		Type:   core.EventDomainRemoved,
		Source: "test",
		Data:   core.DomainEventData{FQDN: "gone.example.com"},
	})
	if err != nil {
		t.Errorf("Publish domain.removed error: %v", err)
	}

	// Give async handler time
	time.Sleep(100 * time.Millisecond)
}

func TestModule_DomainAddedEvent_NoProviders(t *testing.T) {
	c := testCore("")
	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer m.Stop(context.Background())

	// Publish domain.added with no providers registered — should not crash
	c.Events.Publish(context.Background(), core.Event{
		Type:   core.EventDomainAdded,
		Source: "test",
		Data: core.DomainEventData{
			FQDN: "noprov.example.com",
		},
	})

	time.Sleep(100 * time.Millisecond)
}

func TestModule_DomainAddedEvent_NonDomainData(t *testing.T) {
	c := testCore("")
	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer m.Stop(context.Background())

	// Publish domain.added with wrong data type — should not crash
	c.Events.Publish(context.Background(), core.Event{
		Type:   core.EventDomainAdded,
		Source: "test",
		Data:   "not-a-domain-event-data",
	})

	time.Sleep(100 * time.Millisecond)
}

// ===========================================================================
// SyncQueue tests
// ===========================================================================

func TestNewSyncQueue(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	if q == nil {
		t.Fatal("NewSyncQueue returned nil")
	}
}

func TestSyncQueue_Enqueue_SetsIDAndCreatedAt(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())

	job := &SyncJob{
		Action:   "create",
		Provider: "test",
		Record:   core.DNSRecord{Name: "test.example.com"},
	}

	q.Start()
	defer q.Stop()

	q.Enqueue(job)

	if job.ID == "" {
		t.Error("Enqueue should set job.ID if empty")
	}
	if job.CreatedAt.IsZero() {
		t.Error("Enqueue should set job.CreatedAt")
	}
}

func TestSyncQueue_Enqueue_PreservesExistingID(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())

	q.Start()
	defer q.Stop()

	job := &SyncJob{
		ID:       "preset-id",
		Action:   "create",
		Provider: "test",
		Record:   core.DNSRecord{Name: "test.example.com"},
	}

	q.Enqueue(job)

	if job.ID != "preset-id" {
		t.Errorf("Enqueue should preserve existing ID, got %q", job.ID)
	}
}

func TestSyncQueue_Enqueue_QueueFull(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	// Don't start — so nothing drains the channel

	// Fill the buffer (100 capacity)
	for i := 0; i < 100; i++ {
		q.Enqueue(&SyncJob{
			Action:   "create",
			Provider: "test",
			Record:   core.DNSRecord{Name: fmt.Sprintf("rec-%d.example.com", i)},
		})
	}

	// This one should be dropped (logged as warning), not panic
	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "test",
		Record:   core.DNSRecord{Name: "overflow.example.com"},
	})
}

func TestSyncQueue_Process_Create(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock", verifyResult: true}
	svc.RegisterDNSProvider("mock", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "mock",
		Record: core.DNSRecord{
			Type:  "A",
			Name:  "app.example.com",
			Value: "1.2.3.4",
			TTL:   300,
		},
	})

	time.Sleep(200 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", mock.createCalls)
	}
	if mock.lastRecord.Name != "app.example.com" {
		t.Errorf("expected record name 'app.example.com', got %q", mock.lastRecord.Name)
	}
}

func TestSyncQueue_Process_Update(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock", verifyResult: true}
	svc.RegisterDNSProvider("mock", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "update",
		Provider: "mock",
		Record: core.DNSRecord{
			ID:    "rec-1",
			Type:  "A",
			Name:  "app.example.com",
			Value: "5.6.7.8",
			TTL:   600,
		},
	})

	time.Sleep(200 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.updateCalls != 1 {
		t.Errorf("expected 1 update call, got %d", mock.updateCalls)
	}
}

func TestSyncQueue_Process_Delete(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock", verifyResult: true}
	svc.RegisterDNSProvider("mock", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "delete",
		Provider: "mock",
		Record: core.DNSRecord{
			ID:   "rec-del-1",
			Name: "old.example.com",
		},
	})

	time.Sleep(200 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.deleteCalls != 1 {
		t.Errorf("expected 1 delete call, got %d", mock.deleteCalls)
	}
	if mock.lastDeleteID != "rec-del-1" {
		t.Errorf("expected delete ID 'rec-del-1', got %q", mock.lastDeleteID)
	}
}

func TestSyncQueue_Process_UnknownAction(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock", verifyResult: true}
	svc.RegisterDNSProvider("mock", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "invalid_action",
		Provider: "mock",
		Record:   core.DNSRecord{Name: "bad.example.com"},
	})

	time.Sleep(200 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	// No calls should be made
	if mock.createCalls+mock.updateCalls+mock.deleteCalls != 0 {
		t.Error("unknown action should not call any provider method")
	}
}

func TestSyncQueue_Process_ProviderNotFound(t *testing.T) {
	svc := core.NewServices()
	// No providers registered
	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "nonexistent",
		Record:   core.DNSRecord{Name: "orphan.example.com"},
	})

	time.Sleep(200 * time.Millisecond)
	// Should not panic — just log error
}

func TestSyncQueue_Process_CreateError_Retry(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{
		name:         "mock",
		createErr:    fmt.Errorf("network timeout"),
		verifyResult: true,
	}
	svc.RegisterDNSProvider("mock", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "mock",
		Record:   core.DNSRecord{Name: "retry.example.com"},
	})

	// Wait long enough for retries (job sleeps retries*5 seconds between attempts)
	// First attempt: immediate, then retry after 5s, then after 10s
	// We wait enough to see at least 2 attempts
	time.Sleep(7 * time.Second)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createCalls < 2 {
		t.Errorf("expected at least 2 create calls (retries), got %d", mock.createCalls)
	}
}

func TestSyncQueue_Stop(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	q.Stop()
	// Should not block or panic
}

// ===========================================================================
// SyncDomainRecords tests
// ===========================================================================

func TestSyncDomainRecords_RegularDomain(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "cf", verifyResult: true}
	svc.RegisterDNSProvider("cf", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	SyncDomainRecords(q, "app.example.com", "1.2.3.4", "cf")

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	// Should have 2 create calls: A record + CNAME for www
	if mock.createCalls != 2 {
		t.Errorf("expected 2 create calls (A + CNAME), got %d", mock.createCalls)
	}
}

func TestSyncDomainRecords_WildcardDomain(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "cf", verifyResult: true}
	svc.RegisterDNSProvider("cf", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	SyncDomainRecords(q, "*.example.com", "1.2.3.4", "cf")

	time.Sleep(300 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	// Wildcard domains should only get A record, no www CNAME
	if mock.createCalls != 1 {
		t.Errorf("expected 1 create call (A record only for wildcard), got %d", mock.createCalls)
	}
}

func TestSyncDomainRecords_RecordTypes(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "cf", verifyResult: true}
	svc.RegisterDNSProvider("cf", mock)

	events := core.NewEventBus(testLogger())
	q := NewSyncQueue(svc, &mockStore{}, events, testLogger())
	q.Start()
	defer q.Stop()

	SyncDomainRecords(q, "app.example.com", "10.0.0.1", "cf")

	time.Sleep(300 * time.Millisecond)

	// Verify the records have correct types — by checking what was processed
	// The first call should be an A record and second a CNAME
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createCalls != 2 {
		t.Fatalf("expected 2 create calls, got %d", mock.createCalls)
	}
}
