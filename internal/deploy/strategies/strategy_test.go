package strategies

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockRuntime implements core.ContainerRuntime for testing.
type mockRuntime struct {
	createAndStartFn func(ctx context.Context, opts core.ContainerOpts) (string, error)
	stopFn           func(ctx context.Context, containerID string, timeoutSec int) error
	removeFn         func(ctx context.Context, containerID string, force bool) error
	restartFn        func(ctx context.Context, containerID string) error
	stopCalled       bool
	removeCalled     bool
	createCalled     bool
	lastOpts         core.ContainerOpts
}

func (m *mockRuntime) Ping() error { return nil }

func (m *mockRuntime) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	m.createCalled = true
	m.lastOpts = opts
	if m.createAndStartFn != nil {
		return m.createAndStartFn(ctx, opts)
	}
	return "container-123", nil
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
	if m.restartFn != nil {
		return m.restartFn(ctx, containerID)
	}
	return nil
}

func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}

func (m *mockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}

func (m *mockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}

func (m *mockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{
		Health:  "healthy",
		Running: true,
	}, nil
}

func (m *mockRuntime) ImagePull(_ context.Context, _ string) error { return nil }

func (m *mockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }

func (m *mockRuntime) ImageRemove(_ context.Context, _ string) error { return nil }

func (m *mockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) { return nil, nil }

func (m *mockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) { return nil, nil }

// mockStore implements core.Store for testing
type mockStore struct {
	domains []core.Domain
}

func (m *mockStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return m.domains, nil
}

func (m *mockStore) CreateApp(_ context.Context, _ *core.Application) error {
	return nil
}

func (m *mockStore) GetApp(_ context.Context, _ string) (*core.Application, error) {
	return nil, nil
}

func (m *mockStore) UpdateApp(_ context.Context, _ *core.Application) error {
	return nil
}

func (m *mockStore) DeleteApp(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}

func (m *mockStore) ListAppsByTenant(_ context.Context, _ string, _ int, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}

func (m *mockStore) ListAllApps(_ context.Context) ([]core.Application, error) {
	return nil, nil
}

func (m *mockStore) UpdateAppStatus(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, nil
}

func (m *mockStore) CreateDeployment(_ context.Context, _ *core.Deployment) error {
	return nil
}

func (m *mockStore) GetDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}

func (m *mockStore) UpdateDeployment(_ context.Context, _ *core.Deployment) error {
	return nil
}

func (m *mockStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}

func (m *mockStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}

func (m *mockStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) {
	return 1, nil
}

func (m *mockStore) CreateProject(_ context.Context, _ *core.Project) error {
	return nil
}

func (m *mockStore) GetProject(_ context.Context, _ string) (*core.Project, error) {
	return nil, nil
}

func (m *mockStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}

func (m *mockStore) DeleteProject(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) CreateTenantWithDefaults(_ context.Context, _ string, _ string) (string, error) {
	return "", nil
}

func (m *mockStore) CreateTenant(_ context.Context, _ *core.Tenant) error {
	return nil
}

func (m *mockStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}

func (m *mockStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}

func (m *mockStore) UpdateTenant(_ context.Context, _ *core.Tenant) error {
	return nil
}

func (m *mockStore) DeleteTenant(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) ListAllTenants(_ context.Context, _ int, _ int) ([]core.Tenant, int, error) {
	return nil, 0, nil
}

func (m *mockStore) CreateDomain(_ context.Context, _ *core.Domain) error {
	return nil
}

func (m *mockStore) GetDomain(_ context.Context, _ string) (*core.Domain, error) {
	return nil, nil
}

func (m *mockStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, nil
}

func (m *mockStore) UpdateDomain(_ context.Context, _ *core.Domain) error {
	return nil
}

func (m *mockStore) DeleteDomain(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockStore) ListAllDomains(_ context.Context) ([]core.Domain, error) {
	return nil, nil
}

func (m *mockStore) CreateSecret(_ context.Context, _ *core.Secret) error {
	return nil
}

func (m *mockStore) CreateSecretVersion(_ context.Context, _ *core.SecretVersion) error {
	return nil
}

func (m *mockStore) ListSecretsByTenant(_ context.Context, _ string) ([]core.Secret, error) {
	return nil, nil
}
func (m *mockStore) ListAllSecretVersions(_ context.Context) ([]core.SecretVersion, error) {
	return nil, nil
}
func (m *mockStore) UpdateSecretVersionValue(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) GetSecretByScopeAndName(_ context.Context, _, _ string) (*core.Secret, error) {
	return nil, nil
}

func (m *mockStore) GetLatestSecretVersion(_ context.Context, _ string) (*core.SecretVersion, error) {
	return nil, nil
}

func (m *mockStore) CreateUser(_ context.Context, _ *core.User) error {
	return nil
}

func (m *mockStore) GetUser(_ context.Context, _ string) (*core.User, error) {
	return nil, nil
}

func (m *mockStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) {
	return nil, nil
}

func (m *mockStore) UpdateUser(_ context.Context, _ *core.User) error {
	return nil
}

func (m *mockStore) UpdatePassword(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockStore) UpdateLastLogin(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) CountUsers(_ context.Context) (int, error) {
	return 0, nil
}

func (m *mockStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "", nil
}

func (m *mockStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, nil
}

func (m *mockStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) {
	return nil, nil
}

func (m *mockStore) GetRole(_ context.Context, _ string) (*core.Role, error) {
	return nil, nil
}

func (m *mockStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error {
	return nil
}

func (m *mockStore) ListAuditLogs(_ context.Context, _ string, _ int, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}

func (m *mockStore) CreateInvite(_ context.Context, _ *core.Invitation) error {
	return nil
}

func (m *mockStore) ListInvitesByTenant(_ context.Context, _ string) ([]core.Invitation, error) {
	return nil, nil
}

func (m *mockStore) CreateUsageRecord(_ context.Context, _ *core.UsageRecord) error { return nil }
func (m *mockStore) ListUsageRecordsByTenant(_ context.Context, _ string, _, _ int) ([]core.UsageRecord, int, error) { return nil, 0, nil }
func (m *mockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *mockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) { return nil, 0, nil }
func (m *mockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error { return nil }

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"recreate", "recreate"},
		{"rolling", "rolling"},
		{"", "recreate"},        // default
		{"unknown", "recreate"}, // fallback
	}

	for _, tt := range tests {
		s := New(tt.name)
		if s.Name() != tt.want {
			t.Errorf("New(%q).Name() = %q, want %q", tt.name, s.Name(), tt.want)
		}
	}
}

func TestRecreate_Name(t *testing.T) {
	r := &Recreate{}
	if got := r.Name(); got != "recreate" {
		t.Errorf("Recreate.Name() = %q, want %q", got, "recreate")
	}
}

func TestRolling_Name(t *testing.T) {
	r := &Rolling{}
	if got := r.Name(); got != "rolling" {
		t.Errorf("Rolling.Name() = %q, want %q", got, "rolling")
	}
}

func TestRecreate_Execute_NoOldContainer(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
		Store:          &mockStore{},
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Recreate.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should have been called")
	}
	if runtime.stopCalled {
		t.Error("Stop should not be called when no old container exists")
	}
	if runtime.removeCalled {
		t.Error("Remove should not be called when no old container exists")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRecreate_Execute_WithOldContainer(t *testing.T) {
	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 2,
		},
		NewImage:       "nginx:1.25",
		OldContainerID: "old-container-456",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Recreate.Execute returned error: %v", err)
	}

	if !runtime.stopCalled {
		t.Error("Stop should be called for old container")
	}
	if !runtime.removeCalled {
		t.Error("Remove should be called for old container")
	}
	if !runtime.createCalled {
		t.Error("CreateAndStart should be called for new container")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRecreate_Execute_CreateFails(t *testing.T) {
	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("image pull failed")
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "invalid-image",
		OldContainerID: "",
		Runtime:        runtime,
	}

	recreate := &Recreate{}
	err := recreate.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error when CreateAndStart fails")
	}
	if err.Error() != "start new container: image pull failed" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestRolling_Execute_NoOldContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rolling deploy test with sleep in short mode")
	}

	runtime := &mockRuntime{}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "rolling-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "",
		Runtime:        runtime,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err != nil {
		t.Fatalf("Rolling.Execute returned error: %v", err)
	}

	if !runtime.createCalled {
		t.Error("CreateAndStart should be called")
	}
	if runtime.stopCalled {
		t.Error("Stop should not be called when no old container exists")
	}
	if plan.Deployment.ContainerID != "container-123" {
		t.Errorf("ContainerID = %q, want %q", plan.Deployment.ContainerID, "container-123")
	}
}

func TestRolling_Execute_CreateFails(t *testing.T) {
	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("port conflict")
		},
	}
	plan := &DeployPlan{
		App: &core.Application{
			ID:       "app-1",
			Name:     "test-app",
			TenantID: "tenant-1",
		},
		Deployment: &core.Deployment{
			Version: 1,
		},
		NewImage:       "nginx:latest",
		OldContainerID: "old-container",
		Runtime:        runtime,
	}

	rolling := &Rolling{}
	err := rolling.Execute(context.Background(), plan)
	if err == nil {
		t.Fatal("expected error when CreateAndStart fails")
	}
	if err.Error() != "start new container: port conflict" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
	// Old container should not be stopped if new one failed to start
	if runtime.stopCalled {
		t.Error("old container should not be stopped when new container fails to start")
	}
}
