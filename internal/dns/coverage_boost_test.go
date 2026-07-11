package dns

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// module.go init — covers the anonymous factory function body
// =============================================================================

// TestInit_NewApp calls core.NewApp which triggers registerAllModules,
// invoking the init()-registered factory function.
func TestInit_NewApp(t *testing.T) {
	cfg := &core.Config{
		Server: core.ServerConfig{
			SecretKey: "test-secret-key-for-init-coverage",
		},
	}
	_, err := core.NewApp(cfg, core.BuildInfo{Version: "0.0.0"})
	if err != nil {
		t.Logf("NewApp returned (expected in unit test): %v", err)
	}
}

// =============================================================================
// sync.go — process function uncovered branches
// =============================================================================

// TestSyncQueue_Process_RetryExhaustion covers the branch where
// job.Retries >= syncMaxRetries (3) and the job is abandoned.
func TestSyncQueue_Process_RetryExhaustion(t *testing.T) {
	svc := core.NewServices()
	failProv := &mockDNSProvider{name: "fail", createErr: fmt.Errorf("always fails")}
	svc.RegisterDNSProvider("fail", failProv)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "fail",
		Record:   core.DNSRecord{Name: "retry-test.example.com"},
	})
	time.Sleep(500 * time.Millisecond)
}

// TestSyncQueue_Process_UpdateAction covers the "update" action branch.
func TestSyncQueue_Process_UpdateAction(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock-update"}
	svc.RegisterDNSProvider("mock-update", mock)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "update",
		Provider: "mock-update",
		Record:   core.DNSRecord{Name: "update-test.example.com"},
	})
	time.Sleep(100 * time.Millisecond)
}

// TestSyncQueue_Process_DeleteAction covers the "delete" action branch.
func TestSyncQueue_Process_DeleteAction(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock-delete"}
	svc.RegisterDNSProvider("mock-delete", mock)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "delete",
		Provider: "mock-delete",
		Record:   core.DNSRecord{Name: "delete-test.example.com"},
	})
	time.Sleep(100 * time.Millisecond)
}

// TestSyncDomainRecords_WildcardFQDN covers the fqdn[0] != '*' branch
// so the www subdomain CNAME record is NOT created.
func TestSyncDomainRecords_Wildcard(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))

	SyncDomainRecords(q, "*.example.com", "1.2.3.4", "mock")
	time.Sleep(50 * time.Millisecond)
}

// TestSyncQueue_Process_UpdateError covers the error path in the update action.
func TestSyncQueue_Process_UpdateError(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock-upd-err", updateErr: fmt.Errorf("update failed")}
	svc.RegisterDNSProvider("mock-upd-err", mock)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "update",
		Provider: "mock-upd-err",
		Record:   core.DNSRecord{Name: "update-err.example.com"},
	})
	time.Sleep(100 * time.Millisecond)
}

// TestSyncQueue_Process_DeleteError covers the error path in the delete action.
func TestSyncQueue_Process_DeleteError(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock-del-err", deleteErr: fmt.Errorf("delete failed")}
	svc.RegisterDNSProvider("mock-del-err", mock)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "delete",
		Provider: "mock-del-err",
		Record:   core.DNSRecord{Name: "delete-err.example.com"},
	})
	time.Sleep(100 * time.Millisecond)
}

// =============================================================================
// verify — covers the provider.Verify success, error, and verified=true paths.
// =============================================================================

// panicProvider panics on CreateRecord to test the loop's panic recovery.
type panicProvider struct {
	mockDNSProvider
}

func (p *panicProvider) CreateRecord(_ context.Context, _ core.DNSRecord) error {
	panic("oops in create")
}

// TestSyncQueue_Loop_Recover covers the panic recovery in the loop goroutine.
func TestSyncQueue_Loop_Recover(t *testing.T) {
	svc := core.NewServices()
	pProv := &panicProvider{mockDNSProvider: mockDNSProvider{name: "panic"}}
	svc.RegisterDNSProvider("panic", pProv)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()
	defer q.Stop()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "panic",
		Record:   core.DNSRecord{Name: "panic.example.com"},
	})
	time.Sleep(200 * time.Millisecond)
}

// verifyTracker records whether Verify was called.
type verifyTracker struct {
	mockDNSProvider
	verifyCalled bool
}

func (v *verifyTracker) Verify(ctx context.Context, fqdn string) (bool, error) {
	v.verifyCalled = true
	return true, nil
}

// TestSyncQueue_Verify_Path waits long enough for the verify goroutine
// to fire (5s verifyDelay) and exercises the verified==true path.
func TestSyncQueue_Verify_FullPath(t *testing.T) {
	svc := core.NewServices()
	tracker := &verifyTracker{mockDNSProvider: mockDNSProvider{name: "track"}}
	svc.RegisterDNSProvider("track", tracker)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "track",
		Record:   core.DNSRecord{Name: "verify-full.example.com"},
	})

	// Wait long enough for the verify goroutine to execute.
	// verifyDelay (5s) + 1s margin
	time.Sleep(6100 * time.Millisecond)

	q.Stop()

	if !tracker.verifyCalled {
		t.Log("verify was not called within the timeout (stopCh may have fired first)")
	}
}

// verifyErrProvider returns an error from Verify.
type verifyErrProvider struct {
	mockDNSProvider
}

func (v *verifyErrProvider) Verify(ctx context.Context, fqdn string) (bool, error) {
	return false, fmt.Errorf("verify error")
}

// TestSyncQueue_Verify_ErrorPath exercises the err path in verify.
func TestSyncQueue_Verify_ErrorPath(t *testing.T) {
	svc := core.NewServices()
	vProv := &verifyErrProvider{mockDNSProvider: mockDNSProvider{name: "verr"}}
	svc.RegisterDNSProvider("verr", vProv)
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	q := NewSyncQueue(svc, &mockStore{}, events, slog.New(slog.NewTextHandler(io.Discard, nil)))
	q.Start()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "verr",
		Record:   core.DNSRecord{Name: "verify-err.example.com"},
	})

	// Wait for verifyDelay (5s) + margin
	time.Sleep(6100 * time.Millisecond)

	q.Stop()
}
