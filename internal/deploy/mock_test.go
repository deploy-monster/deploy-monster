package deploy

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockRuntime implements core.ContainerRuntime for testing.
type mockRuntime struct {
	createAndStartFn func(ctx context.Context, opts core.ContainerOpts) (string, error)
	stopFn           func(ctx context.Context, containerID string, timeoutSec int) error
	removeFn         func(ctx context.Context, containerID string, force bool) error
	restartFn        func(ctx context.Context, containerID string) error
	listByLabelsFn   func(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error)
	stopCalled       bool
	removeCalled     bool
	createCalled     bool
	restartCalled    bool
	lastOpts         core.ContainerOpts
	pingErr          error
}

func (m *mockRuntime) Ping() error { return m.pingErr }

func (m *mockRuntime) EnsureNetwork(_ context.Context, _ string) error { return nil }

func (m *mockRuntime) Close() error { return nil }

func (m *mockRuntime) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	m.createCalled = true
	m.lastOpts = opts
	if m.createAndStartFn != nil {
		return m.createAndStartFn(ctx, opts)
	}
	return "container-new-123", nil
}

func (m *mockRuntime) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	m.stopCalled = true
	if m.stopFn != nil {
		return m.stopFn(ctx, containerID, timeoutSec)
	}
	return nil
}

func (m *mockRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	m.removeCalled = true
	if m.removeFn != nil {
		return m.removeFn(ctx, containerID, force)
	}
	return nil
}

func (m *mockRuntime) Restart(ctx context.Context, containerID string) error {
	m.restartCalled = true
	if m.restartFn != nil {
		return m.restartFn(ctx, containerID)
	}
	return nil
}

func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockRuntime) ListByLabels(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
	if m.listByLabelsFn != nil {
		return m.listByLabelsFn(ctx, labels)
	}
	return nil, nil
}

func (m *mockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}

func (m *mockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}

func (m *mockRuntime) ImagePull(_ context.Context, _ string) error { return nil }

func (m *mockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }

func (m *mockRuntime) ImageRemove(_ context.Context, _ string) error { return nil }

func (m *mockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) { return nil, nil }

func (m *mockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) { return nil, nil }

// mockStore implements core.Store for testing.
// Only the methods needed by the deploy package are implemented;
// others panic if called, which helps catch unexpected usage.
type mockStore struct {
	// Deployment methods
	deployments        []core.Deployment
	listDeploymentsErr error
	nextVersion        int
	nextVersionErr     error
	atomicCalls        int // RACE-002: tracks AtomicNextDeployVersion call count for regression tests
	nonAtomicCalls     int // RACE-002: tracks legacy GetNextDeployVersion call count
	latestDeployment   *core.Deployment
	createDeployErr    error
	// Tier 100: persisted deployment rows, keyed by ID. The pre-100
	// mock only had `deployments []Deployment` for ListDeploymentsByApp
	// and ignored CreateDeployment entirely. The restart storm test
	// needs a real keyed map so UpdateDeployment / ListDeploymentsByStatus
	// can actually observe state transitions.
	deploymentsByID      map[string]*core.Deployment
	updateDeploymentErr  error
	listByStatusErr      error
	updateDeploymentCall int

	// App methods
	apps                map[string]*core.Application
	getAppErr           error
	listAppsByTenantErr error
	updateStatusFn      func(ctx context.Context, id, status string) error

	// Domain methods
	domains            map[string]*core.Domain
	createDomainErr    error
	getDomainByFQDNErr error

	// Tenant methods
	allTenantsList []core.Tenant
	listTenantsErr bool

	// Tracking
	appStatusUpdates []statusUpdate
}

type statusUpdate struct {
	ID     string
	Status string
}

func newMockStore() *mockStore {
	return &mockStore{
		apps:            make(map[string]*core.Application),
		domains:         make(map[string]*core.Domain),
		deploymentsByID: make(map[string]*core.Deployment),
	}
}

// DeploymentStore methods
func (s *mockStore) CreateDeployment(_ context.Context, dep *core.Deployment) error {
	if s.createDeployErr != nil {
		return s.createDeployErr
	}
	if dep.ID == "" {
		dep.ID = fmt.Sprintf("dep-%d-%d", time.Now().UnixNano(), len(s.deploymentsByID))
	}
	// Tier 100: store a copy so later UpdateDeployment calls can mutate
	// an owned pointer without the caller's in-memory struct drifting.
	copy := *dep
	s.deploymentsByID[dep.ID] = &copy
	s.deployments = append(s.deployments, copy)
	return nil
}

func (s *mockStore) UpdateDeployment(_ context.Context, dep *core.Deployment) error {
	s.updateDeploymentCall++
	if s.updateDeploymentErr != nil {
		return s.updateDeploymentErr
	}
	if dep == nil || dep.ID == "" {
		return fmt.Errorf("update deployment: missing id")
	}
	existing, ok := s.deploymentsByID[dep.ID]
	if !ok {
		// Accept updates for deployments the test didn't pre-seed so
		// tests that construct rows via CreateDeployment and then mutate
		// through UpdateDeployment can still observe the transition.
		copy := *dep
		s.deploymentsByID[dep.ID] = &copy
		s.deployments = append(s.deployments, copy)
		return nil
	}
	existing.Status = dep.Status
	existing.ContainerID = dep.ContainerID
	existing.BuildLog = dep.BuildLog
	existing.FinishedAt = dep.FinishedAt
	// Keep the slice view in sync so ListDeploymentsByApp sees the
	// update without rebuilding from the map on every read.
	for i := range s.deployments {
		if s.deployments[i].ID == dep.ID {
			s.deployments[i] = *existing
			break
		}
	}
	return nil
}

func (s *mockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return s.latestDeployment, nil
}

func (s *mockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	if s.listDeploymentsErr != nil {
		return nil, s.listDeploymentsErr
	}
	return s.deployments, nil
}

func (s *mockStore) ListDeploymentsByStatus(_ context.Context, status string) ([]core.Deployment, error) {
	if s.listByStatusErr != nil {
		return nil, s.listByStatusErr
	}
	var out []core.Deployment
	for _, d := range s.deploymentsByID {
		if d.Status == status {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (s *mockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	s.nonAtomicCalls++
	if s.nextVersionErr != nil {
		return 0, s.nextVersionErr
	}
	if s.nextVersion == 0 {
		return 1, nil
	}
	return s.nextVersion, nil
}

// AtomicNextDeployVersion atomically allocates the next deployment version.
// Production callers must route through this method (see RACE-002). The mock
// tracks calls on a separate counter so regression tests can assert the
// rollback engine uses the atomic path rather than the legacy one.
func (s *mockStore) AtomicNextDeployVersion(_ context.Context, _ string) (int, error) {
	s.atomicCalls++
	if s.nextVersionErr != nil {
		return 0, s.nextVersionErr
	}
	if s.nextVersion == 0 {
		return 1, nil
	}
	return s.nextVersion, nil
}

// AppStore methods
func (s *mockStore) CreateApp(_ context.Context, _ *core.Application) error { return nil }

func (s *mockStore) GetApp(_ context.Context, id string) (*core.Application, error) {
	if s.getAppErr != nil {
		return nil, s.getAppErr
	}
	if app, ok := s.apps[id]; ok {
		return app, nil
	}
	return nil, fmt.Errorf("app not found: %s", id)
}

func (s *mockStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
}

func (s *mockStore) UpdateApp(_ context.Context, _ *core.Application) error { return nil }

func (s *mockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	if s.listAppsByTenantErr != nil {
		return nil, 0, s.listAppsByTenantErr
	}
	var apps []core.Application
	for _, a := range s.apps {
		apps = append(apps, *a)
	}
	return apps, len(apps), nil
}

func (s *mockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}

func (s *mockStore) UpdateAppStatus(_ context.Context, id, status string) error {
	s.appStatusUpdates = append(s.appStatusUpdates, statusUpdate{ID: id, Status: status})
	if s.updateStatusFn != nil {
		return s.updateStatusFn(nil, id, status)
	}
	return nil
}

func (s *mockStore) DeleteApp(_ context.Context, _ string) error { return nil }

// DomainStore methods
func (s *mockStore) CreateDomain(_ context.Context, d *core.Domain) error {
	if s.createDomainErr != nil {
		return s.createDomainErr
	}
	if d.ID == "" {
		d.ID = fmt.Sprintf("dom-%d", time.Now().UnixNano())
	}
	s.domains[d.FQDN] = d
	return nil
}

func (s *mockStore) GetDomain(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}

func (s *mockStore) GetDomainByFQDN(_ context.Context, fqdn string) (*core.Domain, error) {
	if s.getDomainByFQDNErr != nil {
		return nil, s.getDomainByFQDNErr
	}
	if d, ok := s.domains[fqdn]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("domain not found: %s", fqdn)
}

func (s *mockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}

func (s *mockStore) DeleteDomain(_ context.Context, _ string) error { return nil }

func (s *mockStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) { return 0, nil }

func (s *mockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) { return nil, nil }

// TenantStore methods
func (s *mockStore) CreateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (s *mockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error { return nil }
func (s *mockStore) DeleteTenant(_ context.Context, _ string) error       { return nil }

// UserStore methods
func (s *mockStore) CreateUser(_ context.Context, _ *core.User) error { return nil }
func (s *mockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) UpdateUser(_ context.Context, _ *core.User) error    { return nil }
func (s *mockStore) UpdatePassword(_ context.Context, _, _ string) error { return nil }
func (s *mockStore) UpdateLastLogin(_ context.Context, _ string) error   { return nil }
func (s *mockStore) CountUsers(_ context.Context) (int, error)           { return 0, nil }
func (s *mockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}
func (s *mockStore) UpdateTOTPEnabled(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

// ProjectStore methods
func (s *mockStore) CreateProject(_ context.Context, _ *core.Project) error { return nil }
func (s *mockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (s *mockStore) DeleteProject(_ context.Context, _ string) error { return nil }
func (s *mockStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "", nil
}

// RoleStore methods
func (s *mockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, fmt.Errorf("not implemented")
}
func (s *mockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) { return nil, nil }

// AuditStore methods
func (s *mockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (s *mockStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}

// SecretStore methods
func (s *mockStore) CreateSecret(_ context.Context, secret *core.Secret) error {
	if secret.ID == "" {
		secret.ID = core.GenerateID()
	}
	return nil
}
func (s *mockStore) CreateSecretVersion(_ context.Context, version *core.SecretVersion) error {
	if version.ID == "" {
		version.ID = core.GenerateID()
	}
	return nil
}
func (s *mockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (s *mockStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return nil, nil
}
func (s *mockStore) UpdateSecretVersionValue(_ context.Context, _, _ string) error {
	return nil
}
func (s *mockStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, core.ErrNotFound
}
func (s *mockStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, core.ErrNotFound
}

// InviteStore methods
func (s *mockStore) CreateInvite(_ context.Context, invite *core.Invitation) error {
	if invite.ID == "" {
		invite.ID = core.GenerateID()
	}
	return nil
}
func (s *mockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}
func (s *mockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	if s.listTenantsErr {
		return nil, 0, fmt.Errorf("list tenants error")
	}
	return s.allTenantsList, len(s.allTenantsList), nil
}

// Store methods
func (s *mockStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (s *mockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) {
	return nil, 0, nil
}
func (s *mockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (s *mockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (s *mockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error { return nil }

func (s *mockStore) Close() error                 { return nil }
func (s *mockStore) Ping(_ context.Context) error { return nil }

// Ensure mockStore satisfies core.Store at compile time.
var _ core.Store = (*mockStore)(nil)

// Suppress unused import warning for sql package.
var _ = sql.ErrNoRows
