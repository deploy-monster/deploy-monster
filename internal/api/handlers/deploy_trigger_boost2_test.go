package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestDeployTrigger_ImageApp_RuntimeError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})

	// Override CreateAndStart to return error via a custom mock
	// But mockContainerRuntime doesn't support this. Create a custom one.
	errRuntime := &errCreateRuntime{err: errors.New("docker error")}

	handler := NewDeployTriggerHandler(store, errRuntime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status=failed, got %q", store.updatedStatus["app1"])
	}
}

// errCreateRuntime is a mock runtime that fails CreateAndStart.
type errCreateRuntime struct {
	err error
}

func (e *errCreateRuntime) Ping() error { return nil }
func (e *errCreateRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", e.err
}
func (e *errCreateRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (e *errCreateRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (e *errCreateRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (e *errCreateRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (e *errCreateRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (e *errCreateRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (e *errCreateRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}
func (e *errCreateRuntime) ImagePull(_ context.Context, _ string) error           { return nil }
func (e *errCreateRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (e *errCreateRuntime) ImageRemove(_ context.Context, _ string) error         { return nil }
func (e *errCreateRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (e *errCreateRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) { return nil, nil }

func TestDeployTrigger_ImageApp_AtomicVersionError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.errGetNextDeployVersion = errors.New("version fail")

	handler := NewDeployTriggerHandler(store, nil, testCore().Events)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeployTrigger_ImageApp_CreateDeploymentError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.nextDeployVersion["app1"] = 3
	store.errCreateDeployment = errors.New("db fail")

	handler := NewDeployTriggerHandler(store, nil, testCore().Events)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeployTrigger_ImageApp_UpdateStatusError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.nextDeployVersion["app1"] = 3
	store.errUpdateAppStatus = errors.New("status fail")

	runtime := &mockContainerRuntime{}
	handler := NewDeployTriggerHandler(store, runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	// UpdateAppStatus errors are logged but not fatal — should still return 200
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
