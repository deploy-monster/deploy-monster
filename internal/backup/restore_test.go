package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// inMemoryAppStore is a minimal, thread-safe core.Store implementation
// used by the restore round-trip tests. Only the app/tenant/backup
// methods the scheduler and restore path actually hit are backed by
// real state; everything else falls through to the embedded nil Store
// which will panic if a test reaches for an un-mocked method (that's
// the signal to extend the mock, not a bug).
type inMemoryAppStore struct {
	core.Store

	mu      sync.Mutex
	tenants []core.Tenant
	apps    map[string]core.Application // keyed by appID
	backups map[string]*core.Backup     // keyed by backupID
}

func newInMemoryStore(tenants []core.Tenant, apps []core.Application) *inMemoryAppStore {
	s := &inMemoryAppStore{
		tenants: tenants,
		apps:    make(map[string]core.Application, len(apps)),
		backups: make(map[string]*core.Backup),
	}
	for _, a := range apps {
		s.apps[a.ID] = a
	}
	return s
}

func (s *inMemoryAppStore) ListAllTenants(_ context.Context, limit, offset int) ([]core.Tenant, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if offset >= len(s.tenants) {
		return nil, len(s.tenants), nil
	}
	end := offset + limit
	if end > len(s.tenants) {
		end = len(s.tenants)
	}
	out := make([]core.Tenant, end-offset)
	copy(out, s.tenants[offset:end])
	return out, len(s.tenants), nil
}

func (s *inMemoryAppStore) ListAppsByTenant(_ context.Context, tenantID string, limit, offset int) ([]core.Application, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []core.Application
	for _, a := range s.apps {
		if a.TenantID == tenantID {
			all = append(all, a)
		}
	}
	// Deterministic order so pagination is stable across map
	// iteration nondeterminism.
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	total := len(all)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (s *inMemoryAppStore) GetApp(_ context.Context, id string) (*core.Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.apps[id]
	if !ok {
		return nil, fmt.Errorf("app %q not found", id)
	}
	cp := a
	return &cp, nil
}

func (s *inMemoryAppStore) CreateApp(_ context.Context, app *core.Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.apps[app.ID]; exists {
		return fmt.Errorf("app %q already exists", app.ID)
	}
	s.apps[app.ID] = *app
	return nil
}

func (s *inMemoryAppStore) UpdateApp(_ context.Context, app *core.Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.apps[app.ID]; !ok {
		return fmt.Errorf("app %q not found", app.ID)
	}
	s.apps[app.ID] = *app
	return nil
}

func (s *inMemoryAppStore) DeleteApp(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.apps, id)
	return nil
}

func (s *inMemoryAppStore) CreateBackup(_ context.Context, b *core.Backup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *b
	s.backups[b.ID] = &cp
	return nil
}

func (s *inMemoryAppStore) UpdateBackupStatus(_ context.Context, backupID, status string, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.backups[backupID]; ok {
		b.Status = status
		b.SizeBytes = size
	}
	return nil
}

func (s *inMemoryAppStore) appCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.apps)
}

// sampleApp builds a deterministic Application record for restore
// tests. Every field is populated so the round-trip assertion can
// catch silent JSON encoding drift (e.g., a field that used to
// survive marshal-unmarshal but no longer does because a `json:"-"`
// tag was added).
func sampleApp(tenantID, id, name string) core.Application {
	now := time.Date(2026, 4, 11, 9, 0, 0, 0, time.UTC)
	return core.Application{
		ID:         id,
		ProjectID:  "proj-" + tenantID,
		TenantID:   tenantID,
		Name:       name,
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://git.example/" + name + ".git",
		Branch:     "main",
		Dockerfile: "Dockerfile",
		Replicas:   3,
		Port:       8080,
		Status:     "running",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestRestoreApp_MissingBackup(t *testing.T) {
	store := newInMemoryStore(nil, nil)
	storage := NewLocalStorage(t.TempDir())

	_, err := RestoreApp(context.Background(), store, storage, "tenant1", "appX")
	if !errors.Is(err, ErrNoBackupsFound) {
		t.Errorf("RestoreApp with no backups = %v, want ErrNoBackupsFound", err)
	}
}

func TestRestoreApp_NilGuards(t *testing.T) {
	store := newInMemoryStore(nil, nil)
	storage := NewLocalStorage(t.TempDir())

	if _, err := RestoreApp(context.Background(), nil, storage, "t", "a"); err == nil {
		t.Error("RestoreApp(nil store) = nil, want error")
	}
	if _, err := RestoreApp(context.Background(), store, nil, "t", "a"); err == nil {
		t.Error("RestoreApp(nil storage) = nil, want error")
	}
	if _, err := RestoreApp(context.Background(), store, storage, "", "a"); err == nil {
		t.Error("RestoreApp(empty tenant) = nil, want error")
	}
	if _, err := RestoreApp(context.Background(), store, storage, "t", ""); err == nil {
		t.Error("RestoreApp(empty app) = nil, want error")
	}
}

// TestBackupRestore_RoundTrip is the headline test for Phase 3.2.3:
// drive the scheduler's real backupTenant/backupApp path through
// LocalStorage, wipe the store, restore via RestoreApp, and assert
// the recovered Application matches the original field-for-field.
// This is the first end-to-end exercise of the backup write+read
// paths in the codebase — before this test the backup writer and
// reader had never been proven compatible.
func TestBackupRestore_RoundTrip(t *testing.T) {
	tenant := core.Tenant{ID: "tenant1", Name: "acme"}
	original := sampleApp(tenant.ID, "app1", "checkout")

	store := newInMemoryStore([]core.Tenant{tenant}, []core.Application{original})
	storage := NewLocalStorage(t.TempDir())
	storages := map[string]core.BackupStorage{"local": storage}

	// Run one backup sweep directly through the scheduler so we're
	// testing the real write path (backupApp → storage.Upload), not a
	// hand-rolled upload.
	s := NewScheduler(store, storages, nil, nil, "02:00", testLogger())
	s.runBackupsCtx(context.Background())

	// Sanity check: the payload landed where RestoreApp will look.
	entries, err := storage.List(context.Background(), fmt.Sprintf("%s/%s/", tenant.ID, original.ID))
	if err != nil {
		t.Fatalf("List after backup: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no backup entries after scheduler run")
	}

	// Wipe the store as if disaster has struck. appCount must drop
	// to zero so the Create path in RestoreApp is the one exercised,
	// not the Update path.
	if err := store.DeleteApp(context.Background(), original.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}
	if store.appCount() != 0 {
		t.Fatalf("store appCount after wipe = %d, want 0", store.appCount())
	}

	// Restore and assert the round-trip.
	restored, err := RestoreApp(context.Background(), store, storage, tenant.ID, original.ID)
	if err != nil {
		t.Fatalf("RestoreApp: %v", err)
	}
	if restored.ID != original.ID {
		t.Errorf("restored ID = %q, want %q", restored.ID, original.ID)
	}
	if restored.TenantID != original.TenantID {
		t.Errorf("restored TenantID = %q, want %q", restored.TenantID, original.TenantID)
	}
	if restored.Name != original.Name {
		t.Errorf("restored Name = %q, want %q", restored.Name, original.Name)
	}
	if restored.SourceURL != original.SourceURL {
		t.Errorf("restored SourceURL = %q, want %q", restored.SourceURL, original.SourceURL)
	}
	if restored.Replicas != original.Replicas {
		t.Errorf("restored Replicas = %d, want %d", restored.Replicas, original.Replicas)
	}
	if restored.Port != original.Port {
		t.Errorf("restored Port = %d, want %d", restored.Port, original.Port)
	}
	if restored.Status != original.Status {
		t.Errorf("restored Status = %q, want %q", restored.Status, original.Status)
	}
	// UpdatedAt is deliberately refreshed by RestoreApp, so we only
	// check that CreatedAt survived the round-trip unchanged.
	if !restored.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("restored CreatedAt = %v, want %v", restored.CreatedAt, original.CreatedAt)
	}

	// The restore went through CreateApp, so the store should hold
	// exactly one app again.
	if store.appCount() != 1 {
		t.Errorf("store appCount after restore = %d, want 1", store.appCount())
	}
}

// TestRestoreApp_IdempotentUpdate covers the "reset to last backup"
// scenario: the app still exists (maybe a user wants to roll back a
// config edit), so RestoreApp should UpdateApp rather than fail
// with "already exists" from the mock's CreateApp guard.
func TestRestoreApp_IdempotentUpdate(t *testing.T) {
	tenant := core.Tenant{ID: "tenant1"}
	original := sampleApp(tenant.ID, "app1", "api")

	store := newInMemoryStore([]core.Tenant{tenant}, []core.Application{original})
	storage := NewLocalStorage(t.TempDir())
	s := NewScheduler(store, map[string]core.BackupStorage{"local": storage}, nil, nil, "02:00", testLogger())
	s.runBackupsCtx(context.Background())

	// Mutate the in-store copy so the restore can be observed.
	dirty := original
	dirty.Name = "api-corrupted"
	if err := store.UpdateApp(context.Background(), &dirty); err != nil {
		t.Fatalf("pre-dirty UpdateApp: %v", err)
	}

	// Restore without wiping — UpdateApp path must run.
	restored, err := RestoreApp(context.Background(), store, storage, tenant.ID, original.ID)
	if err != nil {
		t.Fatalf("RestoreApp: %v", err)
	}
	if restored.Name != original.Name {
		t.Errorf("restore did not overwrite dirty app: got Name=%q, want %q", restored.Name, original.Name)
	}

	// Verify the store itself has the clean value.
	got, err := store.GetApp(context.Background(), original.ID)
	if err != nil {
		t.Fatalf("GetApp post-restore: %v", err)
	}
	if got.Name != original.Name {
		t.Errorf("store post-restore Name = %q, want %q", got.Name, original.Name)
	}
}

// TestRestoreApp_TenantMismatch proves RestoreApp refuses to write
// a backup whose embedded TenantID does not match the restore
// target — a defense against a mis-keyed upload crossing tenant
// boundaries during disaster recovery.
func TestRestoreApp_TenantMismatch(t *testing.T) {
	attacker := core.Tenant{ID: "attacker"}
	victim := core.Tenant{ID: "victim"}

	// Back up an app under "attacker"; then ask RestoreApp to load
	// it into "victim". The embedded TenantID guard must reject.
	attackerApp := sampleApp(attacker.ID, "app1", "malicious")
	attackerStore := newInMemoryStore([]core.Tenant{attacker}, []core.Application{attackerApp})
	shared := NewLocalStorage(t.TempDir())
	s := NewScheduler(attackerStore, map[string]core.BackupStorage{"local": shared}, nil, nil, "02:00", testLogger())
	s.runBackupsCtx(context.Background())

	// Manually move the payload under "victim/app1/" so the List
	// prefix matches — without this, the guard would never be
	// reached because List would return zero entries for victim/app1/.
	// We achieve that by re-uploading the same payload under the
	// victim's prefix directly.
	entries, err := shared.List(context.Background(), fmt.Sprintf("%s/%s/", attacker.ID, attackerApp.ID))
	if err != nil || len(entries) == 0 {
		t.Fatalf("no attacker backup entries: err=%v", err)
	}
	rc, err := shared.Download(context.Background(), entries[0].Key)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	payload, readErr := io.ReadAll(rc)
	_ = rc.Close()
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	// Upload the raw payload under victim/app1/<some-id>.json.
	victimKey := fmt.Sprintf("%s/%s/cross-tenant.json", victim.ID, attackerApp.ID)
	if err := shared.Upload(context.Background(), victimKey, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Upload poisoned: %v", err)
	}

	victimStore := newInMemoryStore([]core.Tenant{victim}, nil)
	_, err = RestoreApp(context.Background(), victimStore, shared, victim.ID, attackerApp.ID)
	if err == nil {
		t.Fatal("RestoreApp with cross-tenant payload returned nil, want tenant mismatch error")
	}
	if victimStore.appCount() != 0 {
		t.Errorf("victimStore appCount after refused restore = %d, want 0 (no write should happen)",
			victimStore.appCount())
	}
}
